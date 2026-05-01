package gateway

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
)

// AttachmentStore uploads attachment bytes to NATS Object Store and returns
// updated AttachmentDOs with ObjectKey set and Content cleared.
type AttachmentStore struct {
	store nats.ObjectStore
}

func NewAttachmentStore(store nats.ObjectStore) *AttachmentStore {
	return &AttachmentStore{store: store}
}

func (a *AttachmentStore) Upload(ctx context.Context, traceID string, attachments []domain.Attachment) ([]domain.AttachmentDO, error) {
	result := make([]domain.AttachmentDO, len(attachments))
	for i, att := range attachments {
		key := traceID + "/" + strconv.Itoa(i)
		r := base64.NewDecoder(base64.StdEncoding, strings.NewReader(att.Content))
		_, err := a.store.Put(&nats.ObjectMeta{Name: key}, r)
		if err != nil {
			return nil, fmt.Errorf("object store put %s: %w", key, err)
		}
		result[i] = domain.AttachmentDO{
			Name:        att.Name,
			ContentType: att.MimeType,
			ObjectKey:   key,
		}
	}
	return result, nil
}
