package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"codymail-go/internal/domain"
	"codymail-go/internal/msgraph"
)

// --- stubs ---

type stubGraph struct{ err error }

func (s *stubGraph) SendEmail(_ context.Context, _ domain.MailRequestDO) error { return s.err }

type stubKV struct {
	data map[string][]byte
}

func newStubKV() *stubKV { return &stubKV{data: make(map[string][]byte)} }

func (s *stubKV) Get(key string) (nats.KeyValueEntry, error) {
	v, ok := s.data[key]
	if !ok {
		return nil, nats.ErrKeyNotFound
	}
	return &stubKVEntry{value: v}, nil
}

func (s *stubKV) Put(key string, value []byte) (uint64, error) {
	s.data[key] = value
	return 1, nil
}

type stubKVEntry struct{ value []byte }

func (e *stubKVEntry) Bucket() string             { return "" }
func (e *stubKVEntry) Key() string                { return "" }
func (e *stubKVEntry) Value() []byte              { return e.value }
func (e *stubKVEntry) Revision() uint64           { return 0 }
func (e *stubKVEntry) Delta() uint64              { return 0 }
func (e *stubKVEntry) Created() time.Time         { return time.Time{} }
func (e *stubKVEntry) Operation() nats.KeyValueOp { return nats.KeyValuePut }

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
	kv := newStubKV()
	proc := &Processor{graph: &stubGraph{}, delivered: kv, js: js}

	req := domain.MailRequestDO{TraceID: "trace-1", AppTag: "app", Sender: "s@e.com", Recipients: []string{"r@e.com"}}
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
		delivered: newStubKV(),
		js:        js,
	}
	req := domain.MailRequestDO{TraceID: "trace-2", AppTag: "app", Sender: "s@e.com", Recipients: []string{"r@e.com"}}
	proc.Handle(context.Background(), buildMsg(req))

	if len(js.records) != 0 {
		t.Error("transient error must not write audit")
	}
}

func TestHandle_PermanentError_FAILEDAudit(t *testing.T) {
	js := &captureJS{}
	proc := &Processor{
		graph:     &stubGraph{err: &msgraph.GraphPermanentError{StatusCode: 400, Body: "bad"}},
		delivered: newStubKV(),
		js:        js,
	}
	req := domain.MailRequestDO{TraceID: "trace-3", AppTag: "app", Sender: "s@e.com", Recipients: []string{"r@e.com"}}
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
	kv := newStubKV()
	graphCalled := false
	proc := &Processor{
		graph:     &callCheckGraph{onCall: func() { graphCalled = true }},
		delivered: kv,
		js:        js,
	}

	req := domain.MailRequestDO{TraceID: "trace-4", Test: true, AppTag: "app", Sender: "s@e.com", Recipients: []string{"r@e.com"}}
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
	kv := newStubKV()
	kv.data["trace-5"] = []byte{1}

	graphCalled := false
	proc := &Processor{
		graph:     &callCheckGraph{onCall: func() { graphCalled = true }},
		delivered: kv,
		js:        js,
	}

	req := domain.MailRequestDO{TraceID: "trace-5", AppTag: "app", Sender: "s@e.com", Recipients: []string{"r@e.com"}}
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
	proc := &Processor{graph: &stubGraph{}, delivered: newStubKV(), js: js}

	msg := &nats.Msg{Data: []byte("not json"), Header: nats.Header{}}
	msg.Header.Set("traceId", "trace-6")
	proc.Handle(context.Background(), msg)

	if len(js.records) == 0 {
		t.Fatal("expected dead letter record")
	}
}

type callCheckGraph struct{ onCall func() }

func (g *callCheckGraph) SendEmail(_ context.Context, _ domain.MailRequestDO) error {
	g.onCall()
	return errors.New("should not be called")
}
