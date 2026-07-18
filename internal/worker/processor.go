package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
	"dispatch/internal/loggy"
	"dispatch/internal/msgraph"
	"dispatch/internal/natsutil"
	"dispatch/internal/pii"
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

type attachmentFetcher interface {
	Fetch(attachments []domain.AttachmentDO) ([]domain.AttachmentDO, error)
	Cleanup(attachments []domain.AttachmentDO)
}

var procLog = loggy.GetLogger("Processor")

// Processor handles NATS messages: deserialize → dedup → send → audit.
type Processor struct {
	graph     emailSender
	delivered deliveredStore
	js        jsPublisher
	attStore  attachmentFetcher
}

func NewProcessor(graph emailSender, delivered nats.KeyValue, js jsPublisher, attStore attachmentFetcher) *Processor {
	return &Processor{graph: graph, delivered: delivered, js: js, attStore: attStore}
}

var errMissingTraceID = errors.New("missing traceId")

func (p *Processor) Handle(ctx context.Context, msg *nats.Msg) {
	var req domain.MailRequestDO
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		procLog.Error("dead letter: JSON parse failed", err)
		p.writeDeadLetter(ctx, msg.Data, err)
		_ = msg.Ack()
		return
	}

	traceID := req.TraceID
	if traceID == "" {
		traceID = msg.Header.Get("traceId")
	}
	if traceID == "" {
		procLog.Error("dead letter: missing traceId", errMissingTraceID)
		p.writeDeadLetter(ctx, msg.Data, errMissingTraceID)
		_ = msg.Ack()
		return
	}
	log := procLog.With(loggy.Kv("traceId", traceID))

	// idempotent dedup — fail-closed: ErrKeyNotFound = nicht zugestellt (weiter verarbeiten),
	// jeder andere Fehler = transient (return ohne Ack → JetStream-Redelivery)
	_, kvErr := p.delivered.Get(traceID)
	if kvErr == nil {
		log.Info("duplicate delivery detected, acking and skipping")
		_ = msg.Ack()
		return
	}
	if !errors.Is(kvErr, nats.ErrKeyNotFound) {
		log.Warn("delivered KV lookup failed, not acking", loggy.Kv("error", kvErr.Error()))
		return // no ack → JetStream redelivers
	}

	// fetch attachment bytes from Object Store before sending
	if len(req.Attachments) > 0 {
		fetched, err := p.attStore.Fetch(req.Attachments)
		if err != nil {
			log.Error("attachment fetch failed, not acking", err)
			return // no ack → JetStream redelivers
		}
		req.Attachments = fetched
	}

	if req.Test {
		p.processTestMode(ctx, req, traceID, msg, log)
		return
	}
	p.processSend(ctx, req, traceID, msg, log)
}

func (p *Processor) processTestMode(ctx context.Context, req domain.MailRequestDO, traceID string, msg *nats.Msg, log *loggy.Loggy) {
	log.Info("test mode: skipping MS Graph call")
	p.writeAudit(ctx, req, domain.StatusTestSuccess, "")
	if _, err := p.delivered.Put(traceID, []byte{1}); err != nil {
		// fail-closed: no Ack → redelivery; double-send is worse than redelivery
		log.Warn("delivered KV put failed, not acking", loggy.Kv("error", err.Error()))
		return
	}
	_ = msg.Ack()
	if len(req.Attachments) > 0 {
		p.attStore.Cleanup(req.Attachments)
	}
}

func (p *Processor) processSend(ctx context.Context, req domain.MailRequestDO, traceID string, msg *nats.Msg, log *loggy.Loggy) {
	if err := p.graph.SendEmail(ctx, req); err != nil {
		var transient *msgraph.GraphTransientError
		if errors.As(err, &transient) {
			log.Warnc(ctx, loggy.CategoryAPIExternalFailure, "transient graph error, not acking",
				loggy.Kv("sender", pii.MaskEmail(req.Sender)),
				loggy.Kv("error", err.Error()),
			)
			// no ack → JetStream redelivers; keep objects in store for next attempt
			return
		}
		log.Errorc(ctx, loggy.CategoryAPIClientError, "permanent graph error, acking with FAILED", err,
			loggy.Kv("sender", pii.MaskEmail(req.Sender)),
		)
		p.writeAudit(ctx, req, domain.StatusFailed, err.Error())
		_ = msg.Ack()
		if len(req.Attachments) > 0 {
			p.attStore.Cleanup(req.Attachments)
		}
		return
	}
	log.Infoc(ctx, loggy.CategoryBusinessLogic, "mail delivered",
		loggy.Kv("appTag", req.AppTag),
		loggy.Kv("sender", pii.MaskEmail(req.Sender)),
	)
	p.writeAudit(ctx, req, domain.StatusDelivered, "")
	if _, err := p.delivered.Put(traceID, []byte{1}); err != nil {
		// fail-closed: no Ack → redelivery; double-send is worse than redelivery
		log.Warn("delivered KV put failed, not acking", loggy.Kv("error", err.Error()))
		return
	}
	_ = msg.Ack()
	if len(req.Attachments) > 0 {
		p.attStore.Cleanup(req.Attachments)
	}
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
		procLog.Error("marshal audit record", err)
		return
	}
	if _, err := p.js.Publish(natsutil.SubjectAudit, data); err != nil {
		procLog.Error("publish audit record", err)
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
		procLog.Error("marshal dead letter", err)
		return
	}
	if _, err := p.js.Publish(natsutil.SubjectDeadLetter, data); err != nil {
		procLog.Error("publish dead letter", err)
	}
}
