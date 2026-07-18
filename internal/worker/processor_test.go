package worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
	"dispatch/internal/msgraph"
	"dispatch/internal/testkit"
)

const (
	testSender    = "s@e.com"
	testRecipient = "r@e.com"
)

// --- stubs ---

type stubGraph struct{ err error }

func (s *stubGraph) SendEmail(_ context.Context, _ domain.MailRequestDO) error { return s.err }

// captureJS captures published NATS messages for test assertions.
type captureJS struct {
	records [][]byte
}

func (j *captureJS) Publish(_ string, data []byte, _ ...nats.PubOpt) (*nats.PubAck, error) {
	j.records = append(j.records, data)
	return &nats.PubAck{}, nil
}

func buildMsg(req domain.MailRequestDO) *nats.Msg {
	data, _ := json.Marshal(req)
	h := nats.Header{}
	h.Set("traceId", req.TraceID)
	return &nats.Msg{Data: data, Header: h}
}

func TestHandle_Success(t *testing.T) {
	js := &captureJS{}
	kv := testkit.NewMockKV()
	proc := &Processor{graph: &stubGraph{}, delivered: kv, js: js}

	req := domain.MailRequestDO{TraceID: "trace-1", AppTag: "app", Sender: testSender, Recipients: []string{testRecipient}}
	proc.Handle(context.Background(), buildMsg(req))

	if len(js.records) == 0 {
		t.Fatal("expected audit record published")
	}
	var audit domain.AuditRecord
	if err := json.Unmarshal(js.records[0], &audit); err != nil {
		t.Fatal(err)
	}
	if audit.Status != domain.StatusDelivered {
		t.Errorf("expected DELIVERED, got %s", audit.Status)
	}
	if _, err := kv.Get("trace-1"); err != nil {
		t.Error("expected traceId in delivered KV")
	}
}

func TestHandle_TransientError_NoAudit(t *testing.T) {
	js := &captureJS{}
	proc := &Processor{
		graph:     &stubGraph{err: &msgraph.GraphTransientError{StatusCode: 500}},
		delivered: testkit.NewMockKV(),
		js:        js,
	}
	req := domain.MailRequestDO{TraceID: "trace-2", AppTag: "app", Sender: testSender, Recipients: []string{testRecipient}}
	proc.Handle(context.Background(), buildMsg(req))

	if len(js.records) != 0 {
		t.Error("transient error must not write audit")
	}
}

func TestHandle_PermanentError_FAILEDAudit(t *testing.T) {
	js := &captureJS{}
	proc := &Processor{
		graph:     &stubGraph{err: &msgraph.GraphPermanentError{StatusCode: 400, Body: "bad"}},
		delivered: testkit.NewMockKV(),
		js:        js,
	}
	req := domain.MailRequestDO{TraceID: "trace-3", AppTag: "app", Sender: testSender, Recipients: []string{testRecipient}}
	proc.Handle(context.Background(), buildMsg(req))

	if len(js.records) == 0 {
		t.Fatal("expected FAILED audit record")
	}
	var audit domain.AuditRecord
	_ = json.Unmarshal(js.records[0], &audit)
	if audit.Status != domain.StatusFailed {
		t.Errorf("expected FAILED, got %s", audit.Status)
	}
}

func TestHandle_TestMode_NoGraphCall(t *testing.T) {
	js := &captureJS{}
	kv := testkit.NewMockKV()
	graphCalled := false
	proc := &Processor{
		graph:     &callCheckGraph{onCall: func() { graphCalled = true }},
		delivered: kv,
		js:        js,
	}

	req := domain.MailRequestDO{TraceID: "trace-4", Test: true, AppTag: "app", Sender: testSender, Recipients: []string{testRecipient}}
	proc.Handle(context.Background(), buildMsg(req))

	if graphCalled {
		t.Error("test mode must not call MS Graph")
	}
	if len(js.records) == 0 {
		t.Fatal("expected TEST_SUCCESS audit")
	}
	var audit domain.AuditRecord
	_ = json.Unmarshal(js.records[0], &audit)
	if audit.Status != domain.StatusTestSuccess {
		t.Errorf("expected TEST_SUCCESS, got %s", audit.Status)
	}
}

func TestHandle_DuplicateDelivery_NoGraphCall(t *testing.T) {
	js := &captureJS{}
	kv := testkit.NewMockKV()
	kv.Data["trace-5"] = []byte{1}

	graphCalled := false
	proc := &Processor{
		graph:     &callCheckGraph{onCall: func() { graphCalled = true }},
		delivered: kv,
		js:        js,
	}

	req := domain.MailRequestDO{TraceID: "trace-5", AppTag: "app", Sender: testSender, Recipients: []string{testRecipient}}
	proc.Handle(context.Background(), buildMsg(req))

	if graphCalled {
		t.Error("duplicate must not call MS Graph")
	}
	if len(js.records) != 0 {
		t.Error("duplicate must not write audit")
	}
}

func TestHandle_InvalidJSON_DeadLetter(t *testing.T) {
	js := &captureJS{}
	proc := &Processor{graph: &stubGraph{}, delivered: testkit.NewMockKV(), js: js}

	msg := &nats.Msg{Data: []byte("not json"), Header: nats.Header{}}
	msg.Header.Set("traceId", "trace-6")
	proc.Handle(context.Background(), msg)

	if len(js.records) == 0 {
		t.Fatal("expected dead letter record")
	}
}

func TestHandle_MissingTraceID_GoesToDeadLetter(t *testing.T) {
	js := &captureJS{}
	kv := testkit.NewMockKV()
	graphCalled := false
	proc := &Processor{
		graph:     &callCheckGraph{onCall: func() { graphCalled = true }},
		delivered: kv,
		js:        js,
	}

	req := domain.MailRequestDO{AppTag: "app", Sender: testSender, Recipients: []string{testRecipient}}
	data, _ := json.Marshal(req)
	proc.Handle(context.Background(), &nats.Msg{Data: data, Header: nats.Header{}})

	if graphCalled {
		t.Error("missing traceId must not call MS Graph")
	}
	if len(kv.Data) != 0 {
		t.Errorf("delivered KV must stay empty, got %d keys", len(kv.Data))
	}
	if len(js.records) == 0 {
		t.Fatal("expected dead letter record")
	}
	var dl domain.DeadLetter
	if err := json.Unmarshal(js.records[0], &dl); err != nil {
		t.Fatalf("expected dead letter payload, got %v", err)
	}
	if dl.Error == "" {
		t.Error("dead letter must carry a reason")
	}
}

func TestHandle_DedupUsesPayloadTraceID(t *testing.T) {
	js := &captureJS{}
	kv := testkit.NewMockKV()
	proc := &Processor{graph: &stubGraph{}, delivered: kv, js: js}

	req := domain.MailRequestDO{TraceID: "payload-trace", AppTag: "app", Sender: testSender, Recipients: []string{testRecipient}}
	data, _ := json.Marshal(req)
	h := nats.Header{}
	h.Set("traceId", "header-trace")
	proc.Handle(context.Background(), &nats.Msg{Data: data, Header: h})

	if _, err := kv.Get("payload-trace"); err != nil {
		t.Error("dedup entry must use payload traceId")
	}
	if _, err := kv.Get("header-trace"); err == nil {
		t.Error("dedup entry must not use header traceId when payload traceId is set")
	}
}

type callCheckGraph struct{ onCall func() }

func (g *callCheckGraph) SendEmail(_ context.Context, _ domain.MailRequestDO) error {
	g.onCall()
	return errors.New("should not be called")
}

// stubAttFetcher is the attachment fetcher stub.
type stubAttFetcher struct {
	fetchErr  error
	cleanedUp bool
}

func (s *stubAttFetcher) Fetch(atts []domain.AttachmentDO) ([]domain.AttachmentDO, error) {
	return atts, s.fetchErr
}

func (s *stubAttFetcher) Cleanup(_ []domain.AttachmentDO) { s.cleanedUp = true }

// failJS simulates a NATS publish failure.
type failJS struct{}

func (j *failJS) Publish(_ string, _ []byte, _ ...nats.PubOpt) (*nats.PubAck, error) {
	return nil, errors.New("publish failed")
}

func TestHandle_AuditPublishError_DoesNotPanic(t *testing.T) {
	proc := &Processor{graph: &stubGraph{}, delivered: testkit.NewMockKV(), js: &failJS{}}
	req := domain.MailRequestDO{TraceID: "t-pub", AppTag: "app", Sender: testSender, Recipients: []string{testRecipient}}
	// must complete without panic; audit publish error is only logged
	proc.Handle(context.Background(), buildMsg(req))
}

func TestHandle_DeadLetterPublishError_DoesNotPanic(t *testing.T) {
	proc := &Processor{graph: &stubGraph{}, delivered: testkit.NewMockKV(), js: &failJS{}}
	msg := &nats.Msg{Data: []byte("not json"), Header: nats.Header{}}
	msg.Header.Set("traceId", "t-dl")
	proc.Handle(context.Background(), msg)
}

func TestHandle_AttachmentFetchError_NoAck(t *testing.T) {
	att := &stubAttFetcher{fetchErr: errors.New("object store down")}
	proc := &Processor{graph: &stubGraph{}, delivered: testkit.NewMockKV(), js: &captureJS{}, attStore: att}

	req := domain.MailRequestDO{
		TraceID:     "trace-att",
		Attachments: []domain.AttachmentDO{{Name: "f.pdf"}},
	}
	acked := false
	msg := buildMsg(req)
	msg.Subject = "test"
	// We cannot intercept Ack on *nats.Msg directly in unit tests, so we verify
	// the audit stream has no records (indicating no ack path was taken).
	js := &captureJS{}
	proc.js = js
	proc.Handle(context.Background(), msg)

	if len(js.records) != 0 {
		t.Errorf("attachment fetch error must not write audit; got %d records", len(js.records))
	}
	_ = acked
}

func TestHandle_AttachmentCleanupAfterDelivery(t *testing.T) {
	att := &stubAttFetcher{}
	js := &captureJS{}
	kv := testkit.NewMockKV()
	proc := &Processor{graph: &stubGraph{}, delivered: kv, js: js, attStore: att}

	req := domain.MailRequestDO{
		TraceID:     "trace-clean",
		Attachments: []domain.AttachmentDO{{Name: "f.pdf"}},
		Sender:      testSender,
		Recipients:  []string{testRecipient},
	}
	proc.Handle(context.Background(), buildMsg(req))

	if !att.cleanedUp {
		t.Error("attachment cleanup must be called after successful delivery")
	}
}

func TestHandle_AttachmentCleanupAfterPermanentError(t *testing.T) {
	att := &stubAttFetcher{}
	js := &captureJS{}
	proc := &Processor{
		graph:     &stubGraph{err: &msgraph.GraphPermanentError{StatusCode: 400, Body: "bad"}},
		delivered: testkit.NewMockKV(),
		js:        js,
		attStore:  att,
	}

	req := domain.MailRequestDO{
		TraceID:     "trace-perm",
		Attachments: []domain.AttachmentDO{{Name: "f.pdf"}},
		Sender:      testSender,
		Recipients:  []string{testRecipient},
	}
	proc.Handle(context.Background(), buildMsg(req))

	if !att.cleanedUp {
		t.Error("attachment cleanup must be called after permanent graph error")
	}
}

func TestHandle_TestMode_AttachmentCleanup(t *testing.T) {
	att := &stubAttFetcher{}
	js := &captureJS{}
	kv := testkit.NewMockKV()
	proc := &Processor{
		graph:     &callCheckGraph{onCall: func() { /* test mode skips MS Graph; this stub must not be invoked */ }},
		delivered: kv,
		js:        js,
		attStore:  att,
	}

	req := domain.MailRequestDO{
		TraceID:     "trace-test",
		Test:        true,
		Attachments: []domain.AttachmentDO{{Name: "f.pdf"}},
		Sender:      testSender,
		Recipients:  []string{testRecipient},
	}
	proc.Handle(context.Background(), buildMsg(req))

	if !att.cleanedUp {
		t.Error("attachment cleanup must be called after test-mode delivery")
	}
}

func TestHandle_DedupGetError_NotKeyNotFound_FailClosed(t *testing.T) {
	js := &captureJS{}
	kv := testkit.NewMockKV()
	kv.GetErr = errors.New("KV connection lost")

	graphCalled := false
	proc := &Processor{
		graph:     &callCheckGraph{onCall: func() { graphCalled = true }},
		delivered: kv,
		js:        js,
	}

	req := domain.MailRequestDO{TraceID: "trace-failclosed", AppTag: "app", Sender: testSender, Recipients: []string{testRecipient}}
	proc.Handle(context.Background(), buildMsg(req))

	if graphCalled {
		t.Error("dedup KV error (not ErrKeyNotFound) must be fail-closed: no Graph call")
	}
	if len(js.records) != 0 {
		t.Error("dedup KV error (not ErrKeyNotFound) must not write audit")
	}
}

func TestHandle_DedupPutError_NoAckNoCleanup(t *testing.T) {
	js := &captureJS{}
	kv := testkit.NewMockKV()
	kv.PutErr = errors.New("KV full")
	att := &stubAttFetcher{}

	proc := &Processor{
		graph:     &stubGraph{},
		delivered: kv,
		js:        js,
		attStore:  att,
	}

	req := domain.MailRequestDO{
		TraceID:     "trace-putfail",
		AppTag:      "app",
		Sender:      testSender,
		Recipients:  []string{testRecipient},
		Attachments: []domain.AttachmentDO{{Name: "f.pdf"}},
	}
	proc.Handle(context.Background(), buildMsg(req))

	// Audit may still be written (observability); Ack must not happen → no cleanup
	if len(js.records) == 0 {
		t.Fatal("audit must be written even when delivered.Put fails")
	}
	var audit domain.AuditRecord
	_ = json.Unmarshal(js.records[0], &audit)
	if audit.Status != domain.StatusDelivered {
		t.Errorf("expected DELIVERED audit, got %s", audit.Status)
	}
	if att.cleanedUp {
		t.Error("delivered.Put failure must not Ack or cleanup attachments (fail-closed redelivery)")
	}
	if len(kv.Data) != 0 {
		t.Error("delivered KV must stay empty when Put fails")
	}
}

func TestHandle_WriteAuditMarshalError_DoesNotPanic(t *testing.T) {
	// writeAudit marshals domain.AuditRecord; a nil recipient list should be fine
	js := &captureJS{}
	kv := testkit.NewMockKV()
	proc := &Processor{graph: &stubGraph{}, delivered: kv, js: js}

	req := domain.MailRequestDO{TraceID: "trace-audit-marshal", AppTag: "app", Sender: testSender, Recipients: nil}
	proc.Handle(context.Background(), buildMsg(req))

	if len(js.records) == 0 {
		t.Fatal("audit must be published")
	}
}

func TestHandle_TestMode_DeliveredPutError_NoAckNoCleanup(t *testing.T) {
	js := &captureJS{}
	kv := testkit.NewMockKV()
	kv.PutErr = errors.New("KV down")
	att := &stubAttFetcher{}
	proc := &Processor{graph: &stubGraph{}, delivered: kv, js: js, attStore: att}

	req := domain.MailRequestDO{
		TraceID:     "trace-test-put",
		Test:        true,
		AppTag:      "app",
		Sender:      testSender,
		Recipients:  []string{testRecipient},
		Attachments: []domain.AttachmentDO{{Name: "f.pdf"}},
	}
	proc.Handle(context.Background(), buildMsg(req))

	if len(js.records) == 0 {
		t.Fatal("TEST_SUCCESS audit must be written even when KV put fails")
	}
	var audit domain.AuditRecord
	_ = json.Unmarshal(js.records[0], &audit)
	if audit.Status != domain.StatusTestSuccess {
		t.Errorf("expected TEST_SUCCESS, got %s", audit.Status)
	}
	if att.cleanedUp {
		t.Error("test-mode delivered.Put failure must not Ack or cleanup attachments")
	}
}

func TestShouldTerminateMaxDeliver(t *testing.T) {
	cases := []struct {
		name         string
		numDelivered uint64
		maxDeliver   int
		want         bool
	}{
		{"below limit", 7, 8, false},
		{"at limit", 8, 8, true},
		{"above limit", 9, 8, true},
		{"first delivery never terminates at max 8", 1, 8, false},
		{"maxDeliver zero disables gate", 100, 0, false},
		{"maxDeliver negative disables gate", 100, -1, false},
		{"maxDeliver one terminates on first", 1, 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldTerminateMaxDeliver(tc.numDelivered, tc.maxDeliver)
			if got != tc.want {
				t.Errorf("shouldTerminateMaxDeliver(%d, %d)=%v, want %v",
					tc.numDelivered, tc.maxDeliver, got, tc.want)
			}
		})
	}
}

func TestInProgressInterval(t *testing.T) {
	if got := inProgressInterval(5 * time.Minute); got != 100*time.Second {
		t.Errorf("5m/3: want 100s, got %v", got)
	}
	// floor at 10s
	if got := inProgressInterval(15 * time.Second); got != minInProgressInterval {
		t.Errorf("15s/3=5s floored: want %v, got %v", minInProgressInterval, got)
	}
	if got := inProgressInterval(60 * time.Second); got != 20*time.Second {
		t.Errorf("60s/3: want 20s, got %v", got)
	}
}

func TestHandle_MaxDeliverExhausted_DLQNoGraph(t *testing.T) {
	js := &captureJS{}
	kv := testkit.NewMockKV()
	graphCalled := false
	termCalled := false
	att := &stubAttFetcher{}

	proc := &Processor{
		graph:      &callCheckGraph{onCall: func() { graphCalled = true }},
		delivered:  kv,
		js:         js,
		attStore:   att,
		maxDeliver: 3,
		deliveryCountFn: func(_ *nats.Msg) (uint64, bool) {
			return 3, true
		},
		termFn: func(_ *nats.Msg) error {
			termCalled = true
			return nil
		},
	}

	req := domain.MailRequestDO{
		TraceID:     "trace-maxd",
		AppTag:      "app",
		Sender:      testSender,
		Recipients:  []string{testRecipient},
		Attachments: []domain.AttachmentDO{{Name: "f.pdf", ObjectKey: "k1"}},
	}
	proc.Handle(context.Background(), buildMsg(req))

	if graphCalled {
		t.Error("MaxDeliver exhaustion must not call MS Graph")
	}
	if !termCalled {
		t.Error("MaxDeliver exhaustion must call Term (terminal stop)")
	}
	if !att.cleanedUp {
		t.Error("MaxDeliver exhaustion should best-effort cleanup attachment keys")
	}
	if len(js.records) < 2 {
		t.Fatalf("expected dead letter + FAILED audit, got %d records", len(js.records))
	}
	// records: dead letter and audit (order: DLQ then FAILED audit)
	var foundDL, foundFailed bool
	for _, rec := range js.records {
		var dl domain.DeadLetter
		if err := json.Unmarshal(rec, &dl); err == nil && dl.Error != "" && dl.Payload != "" {
			if !containsSubstring(dl.Error, "max deliver exceeded") {
				t.Errorf("dead letter error must mention max deliver, got %q", dl.Error)
			}
			foundDL = true
			continue
		}
		var audit domain.AuditRecord
		if err := json.Unmarshal(rec, &audit); err == nil && audit.Status != "" {
			if audit.Status != domain.StatusFailed {
				t.Errorf("audit status: want FAILED, got %s", audit.Status)
			}
			if !containsSubstring(audit.Error, "max deliver exceeded") {
				t.Errorf("audit error must mention max deliver, got %q", audit.Error)
			}
			foundFailed = true
		}
	}
	if !foundDL {
		t.Error("expected dead letter with max deliver error")
	}
	if !foundFailed {
		t.Error("expected FAILED audit on max deliver exhaustion")
	}
	if len(kv.Data) != 0 {
		t.Error("delivered KV must stay empty on max-deliver path")
	}
}

func TestHandle_MaxDeliver_DedupWinsBeforeGate(t *testing.T) {
	// Already delivered at high NumDelivered → Ack-skip, no FAILED/DLQ, no Graph
	js := &captureJS{}
	kv := testkit.NewMockKV()
	kv.Data["trace-dedup-maxd"] = []byte{1}
	graphCalled := false
	termCalled := false

	proc := &Processor{
		graph:      &callCheckGraph{onCall: func() { graphCalled = true }},
		delivered:  kv,
		js:         js,
		maxDeliver: 3,
		deliveryCountFn: func(_ *nats.Msg) (uint64, bool) {
			return 99, true
		},
		termFn: func(_ *nats.Msg) error {
			termCalled = true
			return nil
		},
	}

	req := domain.MailRequestDO{TraceID: "trace-dedup-maxd", AppTag: "app", Sender: testSender, Recipients: []string{testRecipient}}
	proc.Handle(context.Background(), buildMsg(req))

	if graphCalled {
		t.Error("dedup hit must not call Graph even at high NumDelivered")
	}
	if termCalled {
		t.Error("dedup hit must not Term (Ack-skip path)")
	}
	if len(js.records) != 0 {
		t.Error("dedup hit must not write FAILED audit or DLQ")
	}
}

func TestHandle_MaxDeliver_BelowLimit_ProceedsToGraph(t *testing.T) {
	js := &captureJS{}
	kv := testkit.NewMockKV()
	graphCalled := false

	proc := &Processor{
		graph:      &callCheckGraph{onCall: func() { graphCalled = true }},
		delivered:  kv,
		js:         js,
		maxDeliver: 8,
		deliveryCountFn: func(_ *nats.Msg) (uint64, bool) {
			return 7, true // N-1: still allowed
		},
	}

	// callCheckGraph returns error — still proves Graph was reached (gate did not fire)
	req := domain.MailRequestDO{TraceID: "trace-below", AppTag: "app", Sender: testSender, Recipients: []string{testRecipient}}
	proc.Handle(context.Background(), buildMsg(req))

	if !graphCalled {
		t.Error("NumDelivered < MaxDeliver must proceed to Graph")
	}
}

func TestHandle_MaxDeliver_MetadataUnavailable_SkipsGate(t *testing.T) {
	// Plain unit-test msgs: no JetStream metadata → gate skipped (fail-soft)
	js := &captureJS{}
	kv := testkit.NewMockKV()
	graphCalled := false
	proc := &Processor{
		graph:      &callCheckGraph{onCall: func() { graphCalled = true }},
		delivered:  kv,
		js:         js,
		maxDeliver: 1, // would terminate if count available
		// deliveryCountFn nil → deliveryCount on plain msg → ok=false
	}

	req := domain.MailRequestDO{TraceID: "trace-nometa", AppTag: "app", Sender: testSender, Recipients: []string{testRecipient}}
	proc.Handle(context.Background(), buildMsg(req))

	if !graphCalled {
		t.Error("when metadata unavailable, MaxDeliver gate must skip so unit tests still exercise Graph path")
	}
}

func containsSubstring(s, sub string) bool {
	return strings.Contains(s, sub)
}
