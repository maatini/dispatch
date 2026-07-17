//go:build integration

package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"dispatch/internal/config"
	"dispatch/internal/domain"
	"dispatch/internal/natsutil"
	"dispatch/internal/quota"
	"dispatch/internal/sender"
	"dispatch/internal/spam"
)

func integrationNATS(t *testing.T) (*nats.Conn, nats.JetStreamContext) {
	t.Helper()
	url := os.Getenv("NATS_URL")
	if url == "" {
		url = "nats://localhost:4222"
	}
	nc, err := nats.Connect(url, nats.Timeout(2*time.Second))
	if err != nil {
		t.Skipf("NATS not reachable at %s: %v", url, err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		t.Fatalf("JetStream context: %v", err)
	}
	t.Cleanup(nc.Close)
	if err := natsutil.ProvisionStreams(js); err != nil {
		t.Fatalf("provision streams: %v", err)
	}
	if err := natsutil.ProvisionKVBuckets(js, time.Hour); err != nil {
		t.Fatalf("provision KV buckets: %v", err)
	}
	return nc, js
}

func integrationHandler(t *testing.T, nc *nats.Conn, js nats.JetStreamContext) (*Handler, nats.ObjectStore) {
	t.Helper()
	objStore, err := natsutil.ProvisionObjectStore(js)
	if err != nil {
		t.Fatalf("provision object store: %v", err)
	}
	sendersKV, err := js.KeyValue(natsutil.BucketSenders)
	if err != nil {
		t.Fatalf("senders KV: %v", err)
	}
	quotaKV, err := js.KeyValue(natsutil.BucketQuota)
	if err != nil {
		t.Fatalf("quota KV: %v", err)
	}
	spamKV, err := js.KeyValue(natsutil.BucketSpam)
	if err != nil {
		t.Fatalf("spam KV: %v", err)
	}
	cfg := config.Config{
		MaxBodySize:          10_000_000,
		MimeWhitelist:        []string{"application/pdf"},
		MaxTotalAttachmentMB: 20,
		NatsPublishTimeout:   5 * time.Second,
	}
	h := NewHandler(cfg,
		sender.New(sendersKV, time.Minute),
		quota.NewChecker(quotaKV),
		spam.NewChecker(spamKV),
		NewNatsPublisher(js, 5*time.Second),
		NewAttachmentStore(objStore),
		nc.Status,
	)
	return h, objStore
}

func putIntegrationSender(t *testing.T, js nats.JetStreamContext, s domain.Sender) {
	t.Helper()
	kv, err := js.KeyValue(natsutil.BucketSenders)
	if err != nil {
		t.Fatalf("senders KV: %v", err)
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kv.Put(s.AppTag, data); err != nil {
		t.Fatalf("put sender: %v", err)
	}
}

func postMail(t *testing.T, h *Handler, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/dispatch/api/v1/mail/send", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Router().ServeHTTP(rr, req)
	return rr
}

func TestIntegration_GatewayHappyPath(t *testing.T) {
	nc, js := integrationNATS(t)
	h, objStore := integrationHandler(t, nc, js)

	appTag := "itest-" + uuid.NewString()[:8]
	putIntegrationSender(t, js, domain.Sender{AppTag: appTag, Email: "noreply@example.com", DailyQuota: 100})

	var baseSeq uint64
	if info, err := js.StreamInfo(natsutil.StreamMails); err == nil {
		baseSeq = info.State.LastSeq
	}

	rr := postMail(t, h, map[string]any{
		"appTag":      appTag,
		"recipients":  []string{"user@example.com"},
		"subject":     "integration happy path",
		"bodyContent": "hello",
		"attachments": []map[string]any{
			{"name": "file.pdf", "mimeType": "application/pdf", "content": "dGVzdA=="},
		},
	})
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		TraceID string `json:"traceId"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.TraceID == "" {
		t.Fatal("response must contain traceId")
	}

	var raw *nats.RawStreamMsg
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		info, err := js.StreamInfo(natsutil.StreamMails)
		if err == nil && info.State.LastSeq > baseSeq {
			if m, err := js.GetMsg(natsutil.StreamMails, info.State.LastSeq); err == nil {
				raw = m
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if raw == nil {
		t.Fatal("no message appeared in DISPATCH_MAILS within 5s")
	}
	if got := raw.Header.Get("traceId"); got != resp.TraceID {
		t.Errorf("stream message traceId header: want %s, got %s", resp.TraceID, got)
	}
	var payload domain.MailRequestDO
	if err := json.Unmarshal(raw.Data, &payload); err != nil {
		t.Fatalf("unmarshal stream payload: %v", err)
	}
	if payload.TraceID != resp.TraceID {
		t.Errorf("payload traceId: want %s, got %s", resp.TraceID, payload.TraceID)
	}
	if len(payload.Attachments) != 1 || payload.Attachments[0].ObjectKey == "" {
		t.Fatalf("payload must reference one attachment object, got %+v", payload.Attachments)
	}
	if _, err := objStore.GetInfo(payload.Attachments[0].ObjectKey); err != nil {
		t.Errorf("attachment object %q not found in object store: %v", payload.Attachments[0].ObjectKey, err)
	}
}

func TestIntegration_GatewayQuotaFailClosed(t *testing.T) {
	nc, js := integrationNATS(t)
	h, _ := integrationHandler(t, nc, js)

	appTag := "itest-" + uuid.NewString()[:8]
	putIntegrationSender(t, js, domain.Sender{AppTag: appTag, Email: "noreply@example.com", DailyQuota: 1})

	rr1 := postMail(t, h, map[string]any{
		"appTag":     appTag,
		"recipients": []string{"first@example.com"},
		"subject":    "first",
	})
	if rr1.Code != http.StatusAccepted {
		t.Fatalf("first request: expected 202, got %d: %s", rr1.Code, rr1.Body.String())
	}

	rr2 := postMail(t, h, map[string]any{
		"appTag":     appTag,
		"recipients": []string{"second@example.com"},
		"subject":    "second",
	})
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: expected 429, got %d: %s", rr2.Code, rr2.Body.String())
	}
	if got := rr2.Header().Get("X-RateLimit-Limit"); got != "1" {
		t.Errorf("X-RateLimit-Limit: want 1, got %s", got)
	}
	if rr2.Header().Get("X-RateLimit-Remaining") == "" {
		t.Error("X-RateLimit-Remaining header must be set")
	}
}
