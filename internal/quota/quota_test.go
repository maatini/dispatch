package quota

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
	"dispatch/internal/testkit"
)

func TestCheck_UnderLimit(t *testing.T) {
	checker := &Checker{kv: testkit.NewMockKV()}
	if err := checker.Check("tenant1", 100, 5); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestCheck_Unlimited(t *testing.T) {
	checker := &Checker{kv: testkit.NewMockKV()}
	if err := checker.Check("tenant1", 0, 9999); err != nil {
		t.Fatalf("unlimited quota must always pass: %v", err)
	}
	if err := checker.Check("tenant1", -1, 9999); err != nil {
		t.Fatalf("unlimited quota (-1) must always pass: %v", err)
	}
}

func TestCheck_ExceedsLimit(t *testing.T) {
	checker := &Checker{kv: testkit.NewMockKV()}
	_ = checker.Check("tenant1", 10, 8)
	err := checker.Check("tenant1", 10, 5)
	var quotaErr *domain.QuotaError
	if !errors.As(err, &quotaErr) {
		t.Fatalf("expected QuotaError, got %T: %v", err, err)
	}
	if quotaErr.Current != 8 || quotaErr.Requested != 5 || quotaErr.Limit != 10 {
		t.Errorf("unexpected QuotaError values: %+v", quotaErr)
	}
}

func TestCheck_Accumulates(t *testing.T) {
	checker := &Checker{kv: testkit.NewMockKV()}
	for i := range 5 {
		if err := checker.Check("tenant1", 100, 10); err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
	}
	err := checker.Check("tenant1", 100, 60)
	var quotaErr *domain.QuotaError
	if !errors.As(err, &quotaErr) {
		t.Fatalf("expected QuotaError after accumulation, got %T: %v", err, err)
	}
}

func TestCheck_ExpiredEntriesIgnored(t *testing.T) {
	kv := testkit.NewMockKV()
	// pre-populate with a 25h-old entry
	old := state{Entries: []entry{{TS: time.Now().Add(-25 * time.Hour).Unix(), Count: 99}}}
	data, _ := json.Marshal(old)
	kv.Data["tenant1"] = data
	kv.Revisions["tenant1"] = 1

	checker := &Checker{kv: kv}
	if err := checker.Check("tenant1", 10, 5); err != nil {
		t.Fatalf("old entries should be ignored, got: %v", err)
	}
}

func TestCheck_KVErrorFailClosed(t *testing.T) {
	kv := testkit.NewMockKV()
	kv.GetErr = errors.New("mock KV error")
	checker := &Checker{kv: kv}
	err := checker.Check("tenant1", 10, 1)
	var stateErr *domain.QuotaStateError
	if !errors.As(err, &stateErr) {
		t.Fatalf("expected QuotaStateError on KV failure, got %T: %v", err, err)
	}
}

// alwaysCASConflictKV always returns a JetStream CAS conflict on write,
// forcing Check() to exhaust all maxCASRetries.
type alwaysCASConflictKV struct{}

func (m *alwaysCASConflictKV) Get(_ string) (nats.KeyValueEntry, error) {
	return nil, nats.ErrKeyNotFound
}

func (m *alwaysCASConflictKV) Create(_ string, _ []byte) (uint64, error) {
	return 0, &testkit.WrongSeqError{}
}

func (m *alwaysCASConflictKV) Update(_ string, _ []byte, _ uint64) (uint64, error) {
	return 0, &testkit.WrongSeqError{}
}

func TestCheck_CASRetryExhausted(t *testing.T) {
	checker := &Checker{kv: &alwaysCASConflictKV{}}
	err := checker.Check("tenant1", 10, 1)
	var stateErr *domain.QuotaStateError
	if !errors.As(err, &stateErr) {
		t.Fatalf("expected QuotaStateError after CAS exhaustion, got %T: %v", err, err)
	}
}

func TestCheck_CASRetryPauses(t *testing.T) {
	var pauses int
	checker := &Checker{
		kv:         &alwaysCASConflictKV{},
		retryPause: func(int) { pauses++ },
	}
	_ = checker.Check("tenant1", 10, 1)
	if pauses != maxCASRetries-1 {
		t.Errorf("CAS retries must pause between attempts: want %d pauses, got %d", maxCASRetries-1, pauses)
	}
}

func TestNewChecker_WiresDefaultPause(t *testing.T) {
	c := NewChecker(nil)
	if c.retryPause == nil {
		t.Fatal("NewChecker must set defaultCASPause so production CAS retries back off")
	}
}

func TestCheck_ExactLimitAllowed(t *testing.T) {
	checker := &Checker{kv: testkit.NewMockKV()}
	if err := checker.Check("tenant1", 10, 10); err != nil {
		t.Fatalf("requested == limit must be allowed (sum+requested > limit), got: %v", err)
	}
	err := checker.Check("tenant1", 10, 1)
	var quotaErr *domain.QuotaError
	if !errors.As(err, &quotaErr) {
		t.Fatalf("one over a filled exact limit must be QuotaError, got %T: %v", err, err)
	}
	if quotaErr.Current != 10 || quotaErr.Requested != 1 || quotaErr.Limit != 10 {
		t.Errorf("unexpected QuotaError values: %+v", quotaErr)
	}
}

func TestCheck_SumEqualsLimitAllowed(t *testing.T) {
	checker := &Checker{kv: testkit.NewMockKV()}
	if err := checker.Check("tenant1", 10, 6); err != nil {
		t.Fatalf("first chunk: %v", err)
	}
	if err := checker.Check("tenant1", 10, 4); err != nil {
		t.Fatalf("sum+requested == limit must be allowed, got: %v", err)
	}
}

func TestCheck_EntryAtCutoffExcluded(t *testing.T) {
	kv := testkit.NewMockKV()
	// Stay away from a Unix-second rollover so planted TS equals Check's cutoff.
	for time.Now().Nanosecond() > 700_000_000 {
		time.Sleep(20 * time.Millisecond)
	}
	cutoff := time.Now().Add(-24 * time.Hour).Unix()
	old := state{Entries: []entry{{TS: cutoff, Count: 99}}}
	data, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	kv.Data["tenant1"] = data
	kv.Revisions["tenant1"] = 1

	checker := &Checker{kv: kv}
	if err := checker.Check("tenant1", 10, 5); err != nil {
		t.Fatalf("entry at exact 24h cutoff must be excluded (TS > cutoff), got: %v", err)
	}
}

func TestCheck_EntryInsideWindowCounted(t *testing.T) {
	kv := testkit.NewMockKV()
	recent := state{Entries: []entry{{TS: time.Now().Add(-23 * time.Hour).Unix(), Count: 8}}}
	data, err := json.Marshal(recent)
	if err != nil {
		t.Fatal(err)
	}
	kv.Data["tenant1"] = data
	kv.Revisions["tenant1"] = 1

	checker := &Checker{kv: kv}
	err = checker.Check("tenant1", 10, 5)
	var quotaErr *domain.QuotaError
	if !errors.As(err, &quotaErr) {
		t.Fatalf("23h-old usage must still count, got %T: %v", err, err)
	}
	if quotaErr.Current != 8 {
		t.Errorf("current: want 8, got %d", quotaErr.Current)
	}
}

func TestCheck_UnmarshalFailClosed(t *testing.T) {
	kv := testkit.NewMockKV()
	kv.Data["tenant1"] = []byte("{")
	kv.Revisions["tenant1"] = 1
	checker := &Checker{kv: kv}
	err := checker.Check("tenant1", 10, 1)
	var stateErr *domain.QuotaStateError
	if !errors.As(err, &stateErr) {
		t.Fatalf("corrupt quota JSON must be QuotaStateError, got %T: %v", err, err)
	}
}

func TestCheck_KVWriteFailClosed(t *testing.T) {
	kv := testkit.NewMockKV()
	kv.CreateErr = errors.New("connection reset")
	checker := &Checker{kv: kv}
	err := checker.Check("tenant1", 10, 1)
	var stateErr *domain.QuotaStateError
	if !errors.As(err, &stateErr) {
		t.Fatalf("non-JetStream write error must be QuotaStateError, got %T: %v", err, err)
	}
}

// oneShotCASConflictKV fails the first Create with a JetStream CAS conflict, then succeeds.
type oneShotCASConflictKV struct {
	inner *testkit.MockKV
	fails int
}

func (m *oneShotCASConflictKV) Get(key string) (nats.KeyValueEntry, error) {
	return m.inner.Get(key)
}

func (m *oneShotCASConflictKV) Create(key string, value []byte) (uint64, error) {
	if m.fails == 0 {
		m.fails++
		return 0, &testkit.WrongSeqError{}
	}
	return m.inner.Create(key, value)
}

func (m *oneShotCASConflictKV) Update(key string, value []byte, last uint64) (uint64, error) {
	return m.inner.Update(key, value, last)
}

func TestCheck_CASRetryThenSuccess(t *testing.T) {
	kv := &oneShotCASConflictKV{inner: testkit.NewMockKV()}
	checker := &Checker{kv: kv}
	if err := checker.Check("tenant1", 10, 1); err != nil {
		t.Fatalf("single CAS conflict must retry and succeed, got: %v", err)
	}
	if kv.fails != 1 {
		t.Errorf("expected exactly one CAS failure, got %d", kv.fails)
	}
}
