package worker

import (
	"fmt"
	"io"
	"log/slog"

	"github.com/nats-io/nats.go"

	"codymail-go/internal/domain"
)

// AttachmentStore fetches and deletes attachment objects from NATS Object Store.
type AttachmentStore struct {
	store nats.ObjectStore
}

func NewAttachmentStore(store nats.ObjectStore) *AttachmentStore {
	return &AttachmentStore{store: store}
}

// Fetch downloads bytes for each attachment that has an ObjectKey and sets Content.
func (a *AttachmentStore) Fetch(attachments []domain.AttachmentDO) ([]domain.AttachmentDO, error) {
	result := make([]domain.AttachmentDO, len(attachments))
	for i, att := range attachments {
		if att.ObjectKey == "" {
			result[i] = att
			continue
		}
		obj, err := a.store.Get(att.ObjectKey)
		if err != nil {
			return nil, fmt.Errorf("object store get %s: %w", att.ObjectKey, err)
		}
		data, err := io.ReadAll(obj)
		_ = obj.Close()
		if err != nil {
			return nil, fmt.Errorf("object store read %s: %w", att.ObjectKey, err)
		}
		result[i] = domain.AttachmentDO{
			Name:        att.Name,
			ContentType: att.ContentType,
			ObjectKey:   att.ObjectKey,
			Content:     data,
		}
	}
	return result, nil
}

// Cleanup deletes all object keys. Errors are logged but do not block ACK;
// the bucket TTL handles any orphaned objects.
func (a *AttachmentStore) Cleanup(attachments []domain.AttachmentDO) {
	for _, att := range attachments {
		if att.ObjectKey == "" {
			continue
		}
		if err := a.store.Delete(att.ObjectKey); err != nil {
			slog.Warn("object store delete failed",
				slog.String("key", att.ObjectKey),
				slog.String("error", err.Error()),
			)
		}
	}
}
