package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	"codymail-go/internal/domain"
	"codymail-go/internal/msgraph"
	"codymail-go/internal/natsutil"
	"codymail-go/internal/pii"
)

type emailSender interface {
	SendEmail(ctx context.Context, req domain.MailRequestDO) error
}

type deliveredStore interface {
	Get(key string) (nats.KeyValueEntry, error)
	Put(key string, value []byte) (uint64, error)
}

type jsPublisher interface {
	Publish(subj string, data []byte, opts ...nats.PubOpt) (*nats.PubAck, error)
}

// Processor handles NATS messages: deserialize → dedup → send → audit.
type Processor struct {
	graph     emailSender
	delivered deliveredStore
	js        jsPublisher
}

func NewProcessor(graph emailSender, delivered nats.KeyValue, js jsPublisher) *Processor {
	return &Processor{graph: graph, delivered: delivered, js: js}
}

func (p *Processor) Handle(ctx context.Context, msg *nats.Msg) {
	traceID := msg.Header.Get("traceId")
	if traceID == "" {
		traceID = "unknown"
	}
	log := slog.With(slog.String("traceId", traceID))

	var req domain.MailRequestDO
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.ErrorContext(ctx, "dead letter: JSON parse failed", slog.String("error", err.Error()))
		p.writeDeadLetter(ctx, msg.Data, err)
		_ = msg.Ack()
		return
	}

	// idempotent dedup
	if _, err := p.delivered.Get(traceID); err == nil {
		log.InfoContext(ctx, "duplicate delivery detected, acking and skipping")
		_ = msg.Ack()
		return
	}

	if req.Test {
		log.InfoContext(ctx, "test mode: skipping MS Graph call")
		p.writeAudit(ctx, req, domain.StatusTestSuccess, "")
		if _, err := p.delivered.Put(traceID, []byte{1}); err != nil {
			log.WarnContext(ctx, "delivered KV put failed", slog.String("error", err.Error()))
		}
		_ = msg.Ack()
		return
	}

	if err := p.graph.SendEmail(ctx, req); err != nil {
		var transient *msgraph.GraphTransientError
		if errors.As(err, &transient) {
			log.WarnContext(ctx, "transient graph error, not acking",
				slog.String("sender", pii.MaskEmail(req.Sender)),
				slog.String("error", err.Error()),
			)
			// no ack → JetStream redelivers
			return
		}

		log.ErrorContext(ctx, "permanent graph error, acking with FAILED",
			slog.String("sender", pii.MaskEmail(req.Sender)),
			slog.String("error", err.Error()),
		)
		p.writeAudit(ctx, req, domain.StatusFailed, err.Error())
		_ = msg.Ack()
		return
	}

	log.InfoContext(ctx, "mail delivered",
		slog.String("appTag", req.AppTag),
		slog.String("sender", pii.MaskEmail(req.Sender)),
	)
	p.writeAudit(ctx, req, domain.StatusDelivered, "")
	if _, err := p.delivered.Put(traceID, []byte{1}); err != nil {
		log.WarnContext(ctx, "delivered KV put failed", slog.String("error", err.Error()))
	}
	_ = msg.Ack()
}

func (p *Processor) writeAudit(ctx context.Context, req domain.MailRequestDO, status, errMsg string) {
	record := domain.AuditRecord{
		TraceID:    req.TraceID,
		AppTag:     req.AppTag,
		Status:     status,
		Sender:     req.Sender,
		Subject:    req.Subject,
		Recipients: req.Recipients,
		Error:      errMsg,
		Timestamp:  time.Now().UTC(),
	}
	data, err := json.Marshal(record)
	if err != nil {
		slog.ErrorContext(ctx, "marshal audit record", slog.String("error", err.Error()))
		return
	}
	if _, err := p.js.Publish(natsutil.SubjectAudit, data); err != nil {
		slog.ErrorContext(ctx, "publish audit record", slog.String("error", err.Error()))
	}
}

func (p *Processor) writeDeadLetter(ctx context.Context, payload []byte, cause error) {
	dl := domain.DeadLetter{
		Payload:   string(payload),
		Error:     fmt.Sprintf("%v", cause),
		Timestamp: time.Now().UTC(),
	}
	data, err := json.Marshal(dl)
	if err != nil {
		slog.ErrorContext(ctx, "marshal dead letter", slog.String("error", err.Error()))
		return
	}
	if _, err := p.js.Publish(natsutil.SubjectDeadLetter, data); err != nil {
		slog.ErrorContext(ctx, "publish dead letter", slog.String("error", err.Error()))
	}
}
