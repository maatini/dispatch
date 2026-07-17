package worker

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
)

// --- fake Object Store ---

type fakeObjectStore struct {
	objects map[string][]byte
	getErr  map[string]error
}

func newFakeObjectStore() *fakeObjectStore {
	return &fakeObjectStore{objects: make(map[string][]byte), getErr: make(map[string]error)}
}

func (f *fakeObjectStore) Get(name string, _ ...nats.GetObjectOpt) (nats.ObjectResult, error) {
	if err, ok := f.getErr[name]; ok {
		return nil, err
	}
	data, ok := f.objects[name]
	if !ok {
		return nil, errors.New("object not found")
	}
	return &fakeObjectResult{r: io.NopCloser(bytes.NewReader(data))}, nil
}

func (f *fakeObjectStore) Delete(name string) error { delete(f.objects, name); return nil }

func (f *fakeObjectStore) Put(_ *nats.ObjectMeta, _ io.Reader, _ ...nats.ObjectOpt) (*nats.ObjectInfo, error) {
	return nil, nil
}

func (f *fakeObjectStore) PutBytes(name string, data []byte, _ ...nats.ObjectOpt) (*nats.ObjectInfo, error) {
	f.objects[name] = data
	return nil, nil
}

func (f *fakeObjectStore) GetBytes(_ string, _ ...nats.GetObjectOpt) ([]byte, error) {
	return nil, nil
}

func (f *fakeObjectStore) PutString(_ string, _ string, _ ...nats.ObjectOpt) (*nats.ObjectInfo, error) {
	return nil, nil
}

func (f *fakeObjectStore) GetString(_ string, _ ...nats.GetObjectOpt) (string, error) {
	return "", nil
}

func (f *fakeObjectStore) PutFile(_ string, _ ...nats.ObjectOpt) (*nats.ObjectInfo, error) {
	return nil, nil
}

func (f *fakeObjectStore) GetFile(_, _ string, _ ...nats.GetObjectOpt) error { return nil }

func (f *fakeObjectStore) GetInfo(_ string, _ ...nats.GetObjectInfoOpt) (*nats.ObjectInfo, error) {
	return nil, nil
}

func (f *fakeObjectStore) UpdateMeta(_ string, _ *nats.ObjectMeta) error { return nil }

func (f *fakeObjectStore) AddLink(_ string, _ *nats.ObjectInfo) (*nats.ObjectInfo, error) {
	return nil, nil
}

func (f *fakeObjectStore) AddBucketLink(_ string, _ nats.ObjectStore) (*nats.ObjectInfo, error) {
	return nil, nil
}

func (f *fakeObjectStore) Seal() error { return nil }

func (f *fakeObjectStore) Watch(_ ...nats.WatchOpt) (nats.ObjectWatcher, error) { return nil, nil }

func (f *fakeObjectStore) List(_ ...nats.ListObjectsOpt) ([]*nats.ObjectInfo, error) {
	return nil, nil
}

func (f *fakeObjectStore) Status() (nats.ObjectStoreStatus, error) {
	return nil, nil
}

type fakeObjectResult struct {
	r io.ReadCloser
}

func (fr *fakeObjectResult) Read(p []byte) (int, error) { return fr.r.Read(p) }
func (fr *fakeObjectResult) Close() error               { return fr.r.Close() }
func (fr *fakeObjectResult) Bucket() string             { return "" }
func (fr *fakeObjectResult) Name() string               { return "" }
func (fr *fakeObjectResult) Description() string        { return "" }
func (fr *fakeObjectResult) Size() uint64               { return 0 }
func (fr *fakeObjectResult) ModTime() time.Time         { return time.Time{} }
func (fr *fakeObjectResult) NUID() string               { return "" }
func (fr *fakeObjectResult) Digest() string             { return "" }
func (fr *fakeObjectResult) MaxChunkSize() uint32       { return 0 }
func (fr *fakeObjectResult) MetaData() []*nats.KeyValue { return nil }
func (fr *fakeObjectResult) Links() []*nats.ObjectInfo  { return nil }
func (fr *fakeObjectResult) Options() []nats.ObjectOpt  { return nil }
func (fr *fakeObjectResult) MimeType() string           { return "" }
func (fr *fakeObjectResult) Error() error               { return nil }
func (fr *fakeObjectResult) Info() (*nats.ObjectInfo, error) {
	return nil, nil
}

func TestFetch_PassthroughEmptyObjectKey(t *testing.T) {
	store := NewAttachmentStore(newFakeObjectStore())
	atts := []domain.AttachmentDO{{Name: "f.txt", ContentType: "text/plain", ObjectKey: ""}}
	result, err := store.Fetch(atts)
	if err != nil {
		t.Fatalf("empty ObjectKey must passthrough: %v", err)
	}
	if result[0].Name != "f.txt" {
		t.Errorf("name must be preserved")
	}
}

func TestFetch_Success(t *testing.T) {
	os := newFakeObjectStore()
	os.objects["key1"] = []byte("hello world")

	store := NewAttachmentStore(os)
	atts := []domain.AttachmentDO{{Name: "f.pdf", ContentType: "application/pdf", ObjectKey: "key1"}}
	result, err := store.Fetch(atts)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !bytes.Equal(result[0].Content, []byte("hello world")) {
		t.Errorf("fetched content mismatch")
	}
}

func TestFetch_GetError(t *testing.T) {
	os := newFakeObjectStore()
	os.getErr["key1"] = errors.New("object store down")

	store := NewAttachmentStore(os)
	atts := []domain.AttachmentDO{{Name: "f.pdf", ObjectKey: "key1"}}
	_, err := store.Fetch(atts)
	if err == nil {
		t.Fatal("expected Get error to propagate")
	}
}

func TestFetch_Mixed(t *testing.T) {
	os := newFakeObjectStore()
	os.objects["key2"] = []byte("data")

	store := NewAttachmentStore(os)
	atts := []domain.AttachmentDO{
		{Name: "no-key", ObjectKey: ""},
		{Name: "has-key", ObjectKey: "key2"},
	}
	result, err := store.Fetch(atts)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if result[0].Name != "no-key" {
		t.Errorf("passthrough name mismatch")
	}
	if !bytes.Equal(result[1].Content, []byte("data")) {
		t.Errorf("fetched content mismatch")
	}
}

func TestCleanup_SkipsEmptyObjectKey(t *testing.T) {
	os := newFakeObjectStore()
	store := NewAttachmentStore(os)
	store.Cleanup([]domain.AttachmentDO{{Name: "f.pdf", ObjectKey: ""}})
}

func TestCleanup_DeletesExistingKey(t *testing.T) {
	os := newFakeObjectStore()
	os.objects["key-to-delete"] = []byte("data")

	store := NewAttachmentStore(os)
	store.Cleanup([]domain.AttachmentDO{{Name: "f.pdf", ObjectKey: "key-to-delete"}})

	if _, ok := os.objects["key-to-delete"]; ok {
		t.Error("Cleanup must delete object from store")
	}
}

func TestCleanup_NonExistentKey_DoesNotPanic(t *testing.T) {
	os := newFakeObjectStore()
	store := NewAttachmentStore(os)
	store.Cleanup([]domain.AttachmentDO{{Name: "f.pdf", ObjectKey: "non-existent"}})
}
