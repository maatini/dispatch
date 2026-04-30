package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
	"dispatch/internal/natsutil"
)

// NatsPublisher publishes MailRequestDO messages to NATS JetStream.
type NatsPublisher struct {
	js      nats.JetStreamContext
	timeout time.Duration
}

func NewNatsPublisher(js nats.JetStreamContext, timeout time.Duration) *NatsPublisher {
	return &NatsPublisher{js: js, timeout: timeout}
}

func (p *NatsPublisher) Publish(ctx context.Context, msg *domain.MailRequestDO) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal MailRequestDO: %w", err)
	}

	natMsg := nats.NewMsg(natsutil.SubjectMails)
	natMsg.Header.Set("traceId", msg.TraceID)
	natMsg.Header.Set("appTag", msg.AppTag)
	natMsg.Data = data

	pubCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	_, err = p.js.PublishMsg(natMsg, nats.Context(pubCtx))
	if err != nil {
		return &domain.NatsPublishError{Cause: err}
	}
	return nil
}
