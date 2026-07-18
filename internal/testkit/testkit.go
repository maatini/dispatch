// Package testkit provides shared in-memory NATS KV mocks for unit tests.
package testkit

import (
	"time"

	"github.com/nats-io/nats.go"
)

// MockKV is an in-memory implementation of the NATS KV interfaces used in tests.
// Error fields inject failures; revision tracking enables CAS (check-and-set) tests.
type MockKV struct {
	Data      map[string][]byte
	Revisions map[string]uint64

	GetErr    error
	PutErr    error
	CreateErr error
	UpdateErr error
	DeleteErr error
	KeysErr   error

	// KeysList, if non-nil, is returned by Keys instead of the Data keys.
	KeysList []string
}

func NewMockKV() *MockKV {
	return &MockKV{
		Data:      make(map[string][]byte),
		Revisions: make(map[string]uint64),
	}
}

func (m *MockKV) Get(key string) (nats.KeyValueEntry, error) {
	if m.GetErr != nil {
		return nil, m.GetErr
	}
	v, ok := m.Data[key]
	if !ok {
		return nil, nats.ErrKeyNotFound
	}
	return &entry{value: v, revision: m.Revisions[key]}, nil
}

func (m *MockKV) Put(key string, value []byte) (uint64, error) {
	if m.PutErr != nil {
		return 0, m.PutErr
	}
	m.Revisions[key]++
	m.Data[key] = value
	return m.Revisions[key], nil
}

func (m *MockKV) Create(key string, value []byte) (uint64, error) {
	if m.CreateErr != nil {
		return 0, m.CreateErr
	}
	if _, ok := m.Data[key]; ok {
		return 0, nats.ErrKeyExists
	}
	m.Data[key] = value
	m.Revisions[key] = 1
	return 1, nil
}

func (m *MockKV) Update(key string, value []byte, last uint64) (uint64, error) {
	if m.UpdateErr != nil {
		return 0, m.UpdateErr
	}
	if m.Revisions[key] != last {
		return 0, &WrongSeqError{}
	}
	m.Data[key] = value
	m.Revisions[key] = last + 1
	return last + 1, nil
}

func (m *MockKV) Delete(key string, _ ...nats.DeleteOpt) error {
	if m.DeleteErr != nil {
		return m.DeleteErr
	}
	delete(m.Data, key)
	delete(m.Revisions, key)
	return nil
}

func (m *MockKV) Keys(_ ...nats.WatchOpt) ([]string, error) {
	if m.KeysErr != nil {
		return nil, m.KeysErr
	}
	if m.KeysList != nil {
		return m.KeysList, nil
	}
	keys := make([]string, 0, len(m.Data))
	for k := range m.Data {
		keys = append(keys, k)
	}
	return keys, nil
}

// WrongSeqError implements nats.JetStreamError to simulate a CAS conflict.
type WrongSeqError struct{}

func (e *WrongSeqError) APIError() *nats.APIError { return &nats.APIError{Code: 400} }
func (e *WrongSeqError) Error() string            { return "wrong last sequence" }

type entry struct {
	value    []byte
	revision uint64
}

func (e *entry) Bucket() string             { return "" }
func (e *entry) Key() string                { return "" }
func (e *entry) Value() []byte              { return e.value }
func (e *entry) Revision() uint64           { return e.revision }
func (e *entry) Delta() uint64              { return 0 }
func (e *entry) Created() time.Time         { return time.Time{} }
func (e *entry) Operation() nats.KeyValueOp { return nats.KeyValuePut }
