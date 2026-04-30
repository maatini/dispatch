package sender

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
)

type mockKV struct {
	data    map[string][]byte
	getErr  error
	putErr  error
	delErr  error
	keysErr error
	keys    []string
}

func newMockKV() *mockKV { return &mockKV{data: make(map[string][]byte)} }

func (m *mockKV) Get(key string) (nats.KeyValueEntry, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	v, ok := m.data[key]
	if !ok {
		return nil, nats.ErrKeyNotFound
	}
	return &mockEntry{value: v}, nil
}

func (m *mockKV) Put(key string, value []byte) (uint64, error) {
	if m.putErr != nil {
		return 0, m.putErr
	}
	m.data[key] = value
	return 1, nil
}

func (m *mockKV) Create(key string, value []byte) (uint64, error) {
	m.data[key] = value
	return 1, nil
}

func (m *mockKV) Delete(key string, _ ...nats.DeleteOpt) error {
	if m.delErr != nil {
		return m.delErr
	}
	delete(m.data, key)
	return nil
}

func (m *mockKV) Keys(_ ...nats.WatchOpt) ([]string, error) {
	if m.keysErr != nil {
		return nil, m.keysErr
	}
	return m.keys, nil
}

type mockEntry struct{ value []byte }

func (e *mockEntry) Bucket() string             { return "" }
func (e *mockEntry) Key() string                { return "" }
func (e *mockEntry) Value() []byte              { return e.value }
func (e *mockEntry) Revision() uint64           { return 0 }
func (e *mockEntry) Delta() uint64              { return 0 }
func (e *mockEntry) Created() time.Time         { return time.Time{} }
func (e *mockEntry) Operation() nats.KeyValueOp { return nats.KeyValuePut }

func newStore(kv *mockKV, ttl time.Duration) *Store {
	return &Store{kv: kv, cache: make(map[string]cacheEntry), cacheTTL: ttl}
}

func mustMarshal(s domain.Sender) []byte {
	b, _ := json.Marshal(s)
	return b
}

func TestGet_CacheMiss_KVHit(t *testing.T) {
	kv := newMockKV()
	want := domain.Sender{AppTag: "app1", Email: "noreply@example.com", DailyQuota: 100}
	kv.data["app1"] = mustMarshal(want)

	got, err := newStore(kv, 10*time.Minute).Get("app1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Email != want.Email {
		t.Errorf("Email: want %s, got %s", want.Email, got.Email)
	}
}

func TestGet_CacheHit(t *testing.T) {
	kv := newMockKV()
	want := domain.Sender{AppTag: "app2", Email: "cached@example.com", DailyQuota: 50}
	kv.data["app2"] = mustMarshal(want)

	store := newStore(kv, 10*time.Minute)
	_, _ = store.Get("app2") // populate cache

	kv.getErr = errors.New("KV down") // would fail on a cache miss

	got, err := store.Get("app2")
	if err != nil {
		t.Fatalf("cache hit must not hit KV: %v", err)
	}
	if got.Email != want.Email {
		t.Errorf("Email from cache: want %s, got %s", want.Email, got.Email)
	}
}

func TestGet_CacheExpiry(t *testing.T) {
	kv := newMockKV()
	kv.data["app3"] = mustMarshal(domain.Sender{AppTag: "app3", Email: "old@example.com"})

	store := newStore(kv, -1*time.Millisecond) // TTL already past on first write
	_, _ = store.Get("app3")

	kv.data["app3"] = mustMarshal(domain.Sender{AppTag: "app3", Email: "fresh@example.com"})

	got, err := store.Get("app3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Email != "fresh@example.com" {
		t.Errorf("expected fresh value after expiry, got %s", got.Email)
	}
}

func TestGet_UnknownAppTag(t *testing.T) {
	_, err := newStore(newMockKV(), 10*time.Minute).Get("unknown")
	var ve *domain.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if ve.Code != domain.ErrUnknownAppTag {
		t.Errorf("expected ErrUnknownAppTag, got %s", ve.Code)
	}
}

func TestGet_KVError(t *testing.T) {
	kv := newMockKV()
	kv.getErr = errors.New("nats down")
	_, err := newStore(kv, 10*time.Minute).Get("app1")
	if err == nil {
		t.Fatal("expected error on KV failure")
	}
}

func TestPut_WritesAndInvalidatesCache(t *testing.T) {
	kv := newMockKV()
	kv.data["app4"] = mustMarshal(domain.Sender{AppTag: "app4", Email: "old@example.com"})

	store := newStore(kv, 10*time.Minute)
	_, _ = store.Get("app4") // populate cache

	if err := store.Put(domain.Sender{AppTag: "app4", Email: "new@example.com"}); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got, err := store.Get("app4")
	if err != nil {
		t.Fatalf("Get after Put: %v", err)
	}
	if got.Email != "new@example.com" {
		t.Errorf("expected new email after Put, got %s", got.Email)
	}
}

func TestPut_KVError(t *testing.T) {
	kv := newMockKV()
	kv.putErr = errors.New("nats down")
	err := newStore(kv, 10*time.Minute).Put(domain.Sender{AppTag: "app5", Email: "x@example.com"})
	if err == nil {
		t.Fatal("expected error on KV put failure")
	}
}

func TestDelete_RemovesFromCacheAndKV(t *testing.T) {
	kv := newMockKV()
	kv.data["app6"] = mustMarshal(domain.Sender{AppTag: "app6", Email: "del@example.com"})

	store := newStore(kv, 10*time.Minute)
	_, _ = store.Get("app6") // populate cache

	if err := store.Delete("app6"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := store.Get("app6")
	var ve *domain.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError after delete, got %T: %v", err, err)
	}
}

func TestDelete_KVError(t *testing.T) {
	kv := newMockKV()
	kv.delErr = errors.New("delete failed")
	if err := newStore(kv, 10*time.Minute).Delete("any"); err == nil {
		t.Fatal("expected error on KV delete failure")
	}
}

func TestList_ReturnsSenders(t *testing.T) {
	kv := newMockKV()
	kv.keys = []string{"a", "b"}
	kv.data["a"] = mustMarshal(domain.Sender{AppTag: "a", Email: "a@example.com"})
	kv.data["b"] = mustMarshal(domain.Sender{AppTag: "b", Email: "b@example.com"})

	list, err := newStore(kv, 10*time.Minute).List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 senders, got %d", len(list))
	}
}

func TestList_Empty(t *testing.T) {
	kv := newMockKV()
	kv.keysErr = nats.ErrNoKeysFound

	list, err := newStore(kv, 10*time.Minute).List()
	if err != nil {
		t.Fatalf("empty list must not error: %v", err)
	}
	if list != nil {
		t.Errorf("expected nil, got %v", list)
	}
}

func TestList_KVError(t *testing.T) {
	kv := newMockKV()
	kv.keysErr = errors.New("nats down")
	_, err := newStore(kv, 10*time.Minute).List()
	if err == nil {
		t.Fatal("expected error on KV keys failure")
	}
}

func TestInvalidateCache(t *testing.T) {
	kv := newMockKV()
	kv.data["app7"] = mustMarshal(domain.Sender{AppTag: "app7", Email: "inv@example.com"})

	store := newStore(kv, 10*time.Minute)
	_, _ = store.Get("app7")

	store.InvalidateCache("app7")
	kv.data["app7"] = mustMarshal(domain.Sender{AppTag: "app7", Email: "fresh@example.com"})

	got, err := store.Get("app7")
	if err != nil {
		t.Fatalf("Get after InvalidateCache: %v", err)
	}
	if got.Email != "fresh@example.com" {
		t.Errorf("expected fresh value after invalidation, got %s", got.Email)
	}
}
