package admin

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
	"dispatch/internal/sender"
	"dispatch/internal/testkit"
)

func strPtr(s string) *string { return &s }
func ptr(i int32) *int32      { return &i }

type capturePublisher struct {
	msgs []*nats.Msg
	err  error
}

func (c *capturePublisher) PublishMsg(msg *nats.Msg, _ ...nats.PubOpt) (*nats.PubAck, error) {
	if c.err != nil {
		return nil, c.err
	}
	c.msgs = append(c.msgs, msg)
	return &nats.PubAck{}, nil
}

func TestMatchesMailFilter_NilFilter(t *testing.T) {
	rec := domain.AuditRecord{AppTag: "app", Status: "DELIVERED", TraceID: "t1"}
	if !matchesMailFilter(rec, nil) {
		t.Error("nil filter must match everything")
	}
}

func TestMatchesMailFilter_AppTag(t *testing.T) {
	rec := domain.AuditRecord{AppTag: "app1", TraceID: "t1"}
	if !matchesMailFilter(rec, &mailFilterArgs{AppTag: strPtr("app1")}) {
		t.Error("matching AppTag must return true")
	}
	if matchesMailFilter(rec, &mailFilterArgs{AppTag: strPtr("app2")}) {
		t.Error("non-matching AppTag must return false")
	}
}

func TestMatchesMailFilter_Status(t *testing.T) {
	rec := domain.AuditRecord{Status: "DELIVERED", TraceID: "t1"}
	if !matchesMailFilter(rec, &mailFilterArgs{Status: strPtr("DELIVERED")}) {
		t.Error("matching Status must return true")
	}
	if matchesMailFilter(rec, &mailFilterArgs{Status: strPtr("FAILED")}) {
		t.Error("non-matching Status must return false")
	}
}

func TestMatchesMailFilter_TraceID(t *testing.T) {
	rec := domain.AuditRecord{TraceID: "x"}
	if !matchesMailFilter(rec, &mailFilterArgs{TraceID: strPtr("x")}) {
		t.Error("matching TraceID must return true")
	}
	if matchesMailFilter(rec, &mailFilterArgs{TraceID: strPtr("y")}) {
		t.Error("non-matching TraceID must return false")
	}
}

func TestMatchesMailFilter_Combined(t *testing.T) {
	rec := domain.AuditRecord{AppTag: "app1", Status: "DELIVERED", TraceID: "t1"}
	f := &mailFilterArgs{AppTag: strPtr("app1"), Status: strPtr("DELIVERED"), TraceID: strPtr("t1")}
	if !matchesMailFilter(rec, f) {
		t.Error("combined matching filter must return true")
	}
	f.Status = strPtr("FAILED")
	if matchesMailFilter(rec, f) {
		t.Error("combined mismatched filter must return false")
	}
}

func TestPageSize_Defaults(t *testing.T) {
	p, s := pageSize(nil, nil)
	if p != 0 {
		t.Errorf("default page: want 0, got %d", p)
	}
	if s != 50 {
		t.Errorf("default size: want 50, got %d", s)
	}
}

func TestPageSize_ZeroAndNegative(t *testing.T) {
	p, s := pageSize(ptr(0), ptr(0))
	if p != 0 {
		t.Errorf("page 0: want 0, got %d", p)
	}
	if s != 50 {
		t.Errorf("size 0 must default to 50, got %d", s)
	}
	p2, s2 := pageSize(ptr(1), ptr(-1))
	if p2 != 1 {
		t.Errorf("page 1: want 1, got %d", p2)
	}
	if s2 != 50 {
		t.Errorf("negative size must default to 50, got %d", s2)
	}
}

func TestPageSize_ValidValues(t *testing.T) {
	p, s := pageSize(ptr(2), ptr(10))
	if p != 2 {
		t.Errorf("page: want 2, got %d", p)
	}
	if s != 10 {
		t.Errorf("size: want 10, got %d", s)
	}
}

func TestPaginate_Window(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}
	result, total := paginate(items, 0, 2)
	if total != 5 {
		t.Errorf("total: want 5, got %d", total)
	}
	if len(result) != 2 {
		t.Errorf("page 0 size 2: want 2 items, got %d", len(result))
	}
	if result[0] != "a" || result[1] != "b" {
		t.Errorf("page 0 items mismatch")
	}
}

func TestPaginate_StartBeyondTotal(t *testing.T) {
	items := []string{"a", "b", "c"}
	result, total := paginate(items, 3, 2)
	if total != 3 {
		t.Errorf("total: want 3, got %d", total)
	}
	if result != nil {
		t.Error("start >= total must return nil")
	}
}

func TestPaginate_LastPartialPage(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}
	result, total := paginate(items, 2, 2)
	if total != 5 {
		t.Errorf("total mismatch")
	}
	if len(result) != 1 {
		t.Errorf("last page: want 1 item, got %d", len(result))
	}
}

func TestToSenderGQL_AllowedDomainsEmptyIsNil(t *testing.T) {
	s := domain.Sender{AppTag: "app", Email: "e", AllowedDomains: ""}
	g := toSenderGQL(s)
	if g.AllowedDomains() != nil {
		t.Error("empty AllowedDomains must map to nil")
	}
}

func TestToSenderGQL_AllowedDomainsSet(t *testing.T) {
	s := domain.Sender{AppTag: "app", Email: "e", AllowedDomains: "example.com"}
	g := toSenderGQL(s)
	if g.AllowedDomains() == nil || *g.AllowedDomains() != "example.com" {
		t.Error("AllowedDomains must be set")
	}
}

func TestFromSenderInput_AllowedDomainsNilToEmpty(t *testing.T) {
	s := fromSenderInput(senderInputArgs{AppTag: "app", DailyQuota: 10})
	if s.AllowedDomains != "" {
		t.Errorf("nil AllowedDomains must become empty string, got %q", s.AllowedDomains)
	}
}

func TestToMailRecordGQL_SubjectOnlyWhenSet(t *testing.T) {
	g := toMailRecordGQL(domain.AuditRecord{TraceID: "t", Subject: ""})
	if g.Subject() != nil {
		t.Error("empty Subject must be nil")
	}
	g2 := toMailRecordGQL(domain.AuditRecord{TraceID: "t", Subject: "Hello"})
	if g2.Subject() == nil || *g2.Subject() != "Hello" {
		t.Error("Subject must be set")
	}
}

func TestToMailRecordGQL_ErrorOnlyWhenSet(t *testing.T) {
	g := toMailRecordGQL(domain.AuditRecord{TraceID: "t"})
	if g.Error() != nil {
		t.Error("empty Error must be nil")
	}
	g2 := toMailRecordGQL(domain.AuditRecord{TraceID: "t", Error: "failed"})
	if g2.Error() == nil || *g2.Error() != "failed" {
		t.Error("Error must be set")
	}
}

func TestToBounceRecordGQL(t *testing.T) {
	now := time.Now()
	g := toBounceRecordGQL(domain.BounceRecord{
		OriginalTraceID: "t1", BounceReason: "blocked",
		BouncedAt: now, ProcessedAt: now.Add(time.Hour),
	})
	if g.OriginalTraceId() != "t1" {
		t.Error("traceId mismatch")
	}
}

func TestToDeadLetterGQL(t *testing.T) {
	now := time.Now()
	g := toDeadLetterGQL(domain.DeadLetter{Payload: "{}", Error: "e", Timestamp: now})
	if g.Payload() != "{}" {
		t.Error("Payload mismatch")
	}
}

func TestReprocessDeadLetter_PreservesTraceIDHeader(t *testing.T) {
	pub := &capturePublisher{}
	r := &Resolver{mailPublisher: pub}

	payloads := []string{
		`{"traceId":"trace-aaa","appTag":"app1","sender":"a@example.com","subject":"s1","recipients":["r1@example.com"],"bodyContent":"b"}`,
		`{"traceId":"trace-bbb","appTag":"app2","sender":"a@example.com","subject":"s2","recipients":["r2@example.com"],"bodyContent":"b"}`,
	}
	for _, p := range payloads {
		if _, err := r.ReprocessDeadLetter(context.Background(), struct{ Payload string }{Payload: p}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if len(pub.msgs) != 2 {
		t.Fatalf("want 2 published messages, got %d", len(pub.msgs))
	}
	wantTrace := []string{"trace-aaa", "trace-bbb"}
	wantTag := []string{"app1", "app2"}
	for i, msg := range pub.msgs {
		if got := msg.Header.Get("traceId"); got != wantTrace[i] {
			t.Errorf("msg %d traceId header: want %s, got %s", i, wantTrace[i], got)
		}
		if got := msg.Header.Get("appTag"); got != wantTag[i] {
			t.Errorf("msg %d appTag header: want %s, got %s", i, wantTag[i], got)
		}
		if string(msg.Data) != payloads[i] {
			t.Errorf("msg %d data mismatch", i)
		}
	}
}

func TestReprocessDeadLetter_MissingTraceID_GeneratesUUID(t *testing.T) {
	pub := &capturePublisher{}
	r := &Resolver{mailPublisher: pub}

	payload := `{"appTag":"app1","sender":"a@example.com","subject":"s","recipients":["r@example.com"],"bodyContent":"b"}`
	if _, err := r.ReprocessDeadLetter(context.Background(), struct{ Payload string }{Payload: payload}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.msgs) != 1 {
		t.Fatalf("want 1 published message, got %d", len(pub.msgs))
	}
	got := pub.msgs[0].Header.Get("traceId")
	if got == "" || got == "unknown" {
		t.Errorf("expected generated traceId, got %q", got)
	}
}

func TestReprocessDeadLetter_InvalidPayload_Fails(t *testing.T) {
	pub := &capturePublisher{}
	r := &Resolver{mailPublisher: pub}

	if _, err := r.ReprocessDeadLetter(context.Background(), struct{ Payload string }{Payload: `not-json`}); err == nil {
		t.Fatal("expected error for invalid payload")
	}
	if len(pub.msgs) != 0 {
		t.Errorf("no message must be published for invalid payload, got %d", len(pub.msgs))
	}
}

func TestReprocessDeadLetter_PublishError(t *testing.T) {
	pub := &capturePublisher{err: errors.New("nats down")}
	r := &Resolver{mailPublisher: pub}

	payload := `{"traceId":"t1","appTag":"app1"}`
	if _, err := r.ReprocessDeadLetter(context.Background(), struct{ Payload string }{Payload: payload}); err == nil {
		t.Fatal("expected publish error to propagate")
	}
}

// Sender CRUD via shared mock KV

func senderBytes(s domain.Sender) []byte {
	b, _ := json.Marshal(s)
	return b
}

func newTestResolver(kv *testkit.MockKV) *Resolver {
	return &Resolver{senders: sender.New(kv, sender.DefaultCacheTTL)}
}

func TestSenders_List(t *testing.T) {
	kv := testkit.NewMockKV()
	s1 := domain.Sender{AppTag: "app1", Email: "a1@e.com"}
	s2 := domain.Sender{AppTag: "app2", Email: "a2@e.com"}
	kv.Data["app1"] = senderBytes(s1)
	kv.Data["app2"] = senderBytes(s2)

	r := newTestResolver(kv)
	list, err := r.Senders(context.Background(), struct{ Filter *senderFilterArgs }{})
	if err != nil {
		t.Fatalf("Senders: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("want 2 senders, got %d", len(list))
	}
}

func TestSenders_ListWithAppTagFilter(t *testing.T) {
	kv := testkit.NewMockKV()
	kv.Data["app1"] = senderBytes(domain.Sender{AppTag: "app1", Email: "a1@e.com"})
	kv.Data["app2"] = senderBytes(domain.Sender{AppTag: "app2", Email: "a2@e.com"})

	r := newTestResolver(kv)
	list, err := r.Senders(context.Background(), struct{ Filter *senderFilterArgs }{
		Filter: &senderFilterArgs{AppTag: strPtr("app1")},
	})
	if err != nil {
		t.Fatalf("Senders: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("filtered: want 1, got %d", len(list))
	}
	if list[0].AppTag() != "app1" {
		t.Errorf("AppTag mismatch")
	}
}

func TestCreateSender_PutAndRoundtrip(t *testing.T) {
	kv := testkit.NewMockKV()
	r := newTestResolver(kv)

	g, err := r.CreateSender(context.Background(), struct{ Input senderInputArgs }{
		Input: senderInputArgs{AppTag: "new", Email: "n@e.com", DailyQuota: 100},
	})
	if err != nil {
		t.Fatalf("CreateSender: %v", err)
	}
	if g.AppTag() != "new" {
		t.Errorf("AppTag mismatch")
	}

	// verify stored in KV
	list, _ := r.Senders(context.Background(), struct{ Filter *senderFilterArgs }{})
	if len(list) != 1 {
		t.Errorf("after Create: want 1 sender, got %d", len(list))
	}
}

func TestUpdateSender_AppTagFromPathOverridesInput(t *testing.T) {
	kv := testkit.NewMockKV()
	kv.Data["app-x"] = senderBytes(domain.Sender{AppTag: "app-x", Email: "old@e.com"})

	r := newTestResolver(kv)
	g, err := r.UpdateSender(context.Background(), struct {
		AppTag string
		Input  senderInputArgs
	}{
		AppTag: "app-x",
		Input:  senderInputArgs{AppTag: "should-be-ignored", Email: "new@e.com", DailyQuota: 50},
	})
	if err != nil {
		t.Fatalf("UpdateSender: %v", err)
	}
	if g.AppTag() != "app-x" {
		t.Errorf("AppTag must come from path argument, got %s", g.AppTag())
	}
	if g.Email() != "new@e.com" {
		t.Errorf("Email mismatch")
	}
}

func TestDeleteSender_TrueAndRemovesEntry(t *testing.T) {
	kv := testkit.NewMockKV()
	kv.Data["to-delete"] = senderBytes(domain.Sender{AppTag: "to-delete", Email: "d@e.com"})

	r := newTestResolver(kv)
	ok, err := r.DeleteSender(context.Background(), struct{ AppTag string }{AppTag: "to-delete"})
	if err != nil {
		t.Fatalf("DeleteSender: %v", err)
	}
	if !ok {
		t.Error("DeleteSender must return true")
	}
	if _, exists := kv.Data["to-delete"]; exists {
		t.Error("entry must be removed from KV")
	}
}

func TestDeleteSender_KVError(t *testing.T) {
	kv := testkit.NewMockKV()
	kv.DeleteErr = errors.New("kv down")
	kv.Data["x"] = senderBytes(domain.Sender{AppTag: "x"})

	r := newTestResolver(kv)
	_, err := r.DeleteSender(context.Background(), struct{ AppTag string }{AppTag: "x"})
	if err == nil {
		t.Fatal("expected KV error to propagate")
	}
}

func TestCreateSender_KVError(t *testing.T) {
	kv := testkit.NewMockKV()
	kv.PutErr = errors.New("kv full")
	r := newTestResolver(kv)

	_, err := r.CreateSender(context.Background(), struct{ Input senderInputArgs }{
		Input: senderInputArgs{AppTag: "new", Email: "n@e.com", DailyQuota: 10},
	})
	if err == nil {
		t.Fatal("expected KV error to propagate")
	}
}
