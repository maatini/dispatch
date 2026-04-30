package gateway

import (
	"bytes"
	"context"
	"fmt"
	"strconv"

	"github.com/nats-io/nats.go"

	"codymail-go/internal/domain"
)

// AttachmentStore uploads attachment bytes to NATS Object Store and returns
// updated AttachmentDOs with ObjectKey set and Content cleared.
type AttachmentStore struct {
	store nats.ObjectStore
}

func NewAttachmentStore(store nats.ObjectStore) *AttachmentStore {
	return &AttachmentStore{store: store}
}

func (a *AttachmentStore) Upload(ctx context.Context, traceID string, attachments []domain.AttachmentDO) ([]domain.AttachmentDO, error) {
	result := make([]domain.AttachmentDO, len(attachments))
	for i, att := range attachments {
		key := traceID + "/" + strconv.Itoa(i)
		_, err := a.store.Put(&nats.ObjectMeta{Name: key}, bytes.NewReader(att.Content))
		if err != nil {
			return nil, fmt.Errorf("object store put %s: %w", key, err)
		}
		result[i] = domain.AttachmentDO{
			Name:        att.Name,
			ContentType: att.ContentType,
			ObjectKey:   key,
		}
	}
	return result, nil
}
