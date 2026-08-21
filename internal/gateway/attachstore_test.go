package gateway

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
)

type fakeObjectStore struct {
	objects map[string][]byte
	putErr  error
}

func newFakeObjectStore() *fakeObjectStore {
	return &fakeObjectStore{objects: make(map[string][]byte)}
}

func (f *fakeObjectStore) Get(name string, _ ...nats.GetObjectOpt) (nats.ObjectResult, error) {
	data, ok := f.objects[name]
	if !ok {
		return nil, errors.New("object not found")
	}
	return &fakeObjectResult{r: io.NopCloser(bytes.NewReader(data))}, nil
}

func (f *fakeObjectStore) Delete(name string) error { delete(f.objects, name); return nil }

func (f *fakeObjectStore) Put(meta *nats.ObjectMeta, r io.Reader, _ ...nats.ObjectOpt) (*nats.ObjectInfo, error) {
	if f.putErr != nil {
		return nil, f.putErr
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	f.objects[meta.Name] = data
	return &nats.ObjectInfo{ObjectMeta: *meta, Size: uint64(len(data))}, nil
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

func TestNewAttachmentStore(t *testing.T) {
	store := NewAttachmentStore(newFakeObjectStore())
	if store == nil || store.store == nil {
		t.Fatal("NewAttachmentStore must wrap the Object Store")
	}
}

func TestUpload_Empty(t *testing.T) {
	store := NewAttachmentStore(newFakeObjectStore())
	got, err := store.Upload(context.Background(), "trace-empty", nil)
	if err != nil {
		t.Fatalf("empty upload: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want 0 attachments, got %d", len(got))
	}
}

func TestUpload_StoresDecodedBytesAndClearsContent(t *testing.T) {
	fake := newFakeObjectStore()
	store := NewAttachmentStore(fake)
	raw := []byte("hello-pdf")
	atts := []domain.Attachment{{
		Name:     "a.pdf",
		MimeType: "application/pdf",
		Content:  base64.StdEncoding.EncodeToString(raw),
	}}
	got, err := store.Upload(context.Background(), "trace-1", atts)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 DO, got %d", len(got))
	}
	if got[0].ObjectKey != "trace-1/0" {
		t.Errorf("ObjectKey: want trace-1/0, got %s", got[0].ObjectKey)
	}
	if got[0].Name != "a.pdf" || got[0].ContentType != "application/pdf" {
		t.Errorf("metadata: %+v", got[0])
	}
	if len(got[0].Content) != 0 {
		t.Error("Content must be cleared after Object Store put")
	}
	if !bytes.Equal(fake.objects["trace-1/0"], raw) {
		t.Errorf("stored bytes: want %q, got %q", raw, fake.objects["trace-1/0"])
	}
}

func TestUpload_MultipleAttachments(t *testing.T) {
	fake := newFakeObjectStore()
	store := NewAttachmentStore(fake)
	atts := []domain.Attachment{
		{Name: "a.txt", MimeType: "text/plain", Content: base64.StdEncoding.EncodeToString([]byte("aa"))},
		{Name: "b.txt", MimeType: "text/plain", Content: base64.StdEncoding.EncodeToString([]byte("bb"))},
	}
	got, err := store.Upload(context.Background(), "t", atts)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}
	if got[0].ObjectKey != "t/0" || got[1].ObjectKey != "t/1" {
		t.Errorf("keys: %s, %s", got[0].ObjectKey, got[1].ObjectKey)
	}
	if !bytes.Equal(fake.objects["t/0"], []byte("aa")) || !bytes.Equal(fake.objects["t/1"], []byte("bb")) {
		t.Errorf("stored objects: %v", fake.objects)
	}
}

func TestUpload_PutError(t *testing.T) {
	fake := newFakeObjectStore()
	fake.putErr = errors.New("disk full")
	store := NewAttachmentStore(fake)
	atts := []domain.Attachment{{
		Name:     "a.pdf",
		MimeType: "application/pdf",
		Content:  base64.StdEncoding.EncodeToString([]byte("x")),
	}}
	_, err := store.Upload(context.Background(), "trace-err", atts)
	if err == nil {
		t.Fatal("expected put error")
	}
	if !strings.Contains(err.Error(), "object store put") {
		t.Errorf("error must wrap put failure, got %v", err)
	}
}
