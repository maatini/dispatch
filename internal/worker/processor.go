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

const minInProgressInterval = 10 * time.Second

// Processor handles NATS messages: deserialize → dedup → max-deliver gate → send → audit.
type Processor struct {
	graph           emailSender
	delivered       deliveredStore
	js              jsPublisher
	attStore        attachmentFetcher
	maxDeliver      int
	inProgressEvery time.Duration
	// test hooks (nil in production): allow unit tests to inject delivery count / term without JetStream reply subjects.
	deliveryCountFn func(msg *nats.Msg) (uint64, bool)
	termFn          func(msg *nats.Msg) error
}

// NewProcessor builds a Processor. maxDeliver and ackWait drive the MaxDeliver gate and
// InProgress heartbeat interval (ackWait/3, minimum 10s).
func NewProcessor(graph emailSender, delivered nats.KeyValue, js jsPublisher, attStore attachmentFetcher, maxDeliver int, ackWait time.Duration) *Processor {
	return &Processor{
		graph:           graph,
		delivered:       delivered,
		js:              js,
		attStore:        attStore,
		maxDeliver:      maxDeliver,
		inProgressEvery: inProgressInterval(ackWait),
	}
}

// inProgressInterval returns how often to call msg.InProgress: AckWait/3, min 10s.
func inProgressInterval(ackWait time.Duration) time.Duration {
	every := ackWait / 3
	if every < minInProgressInterval {
		return minInProgressInterval
	}
	return every
}

// shouldTerminateMaxDeliver reports whether NumDelivered has reached the configured limit.
// Pure helper for unit tests and the Handle gate.
func shouldTerminateMaxDeliver(numDelivered uint64, maxDeliver int) bool {
	if maxDeliver < 1 {
		return false
	}
	return numDelivered >= uint64(maxDeliver)
}

// deliveryCount returns JetStream NumDelivered when metadata is available.
// On plain *nats.Msg (unit tests) Metadata fails → ok=false and the MaxDeliver gate is skipped.
func deliveryCount(msg *nats.Msg) (uint64, bool) {
	md, err := msg.Metadata()
	if err != nil {
		return 0, false
	}
	return md.NumDelivered, true
}

// startInProgress periodically signals JetStream that work is still in progress so long
// Graph/attachment handling does not redeliver under AckWait. Failures are warn-only.
// Returns a stop func that must be deferred.
func startInProgress(msg *nats.Msg, every time.Duration) (stop func()) {
	if every <= 0 {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := msg.InProgress(); err != nil {
					procLog.Warn("InProgress signal failed", loggy.Kv("error", err.Error()))
				}
			}
		}
	}()
	return func() { close(done) }
}

// terminalStop stops redelivery: prefer Term, fall back to Ack.
func terminalStop(msg *nats.Msg) {
	if err := msg.Term(); err != nil {
		_ = msg.Ack()
	}
}

var errMissingTraceID = errors.New("missing traceId")

func (p *Processor) Handle(ctx context.Context, msg *nats.Msg) {
	stop := startInProgress(msg, p.inProgressEvery)
	defer stop()

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
	// Dedup runs BEFORE MaxDeliver so a successful Put on a prior attempt still Ack-skips
	// without FAILED/DLQ even when NumDelivered is high.
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

	// MaxDeliver gate: terminal stop + DLQ when delivery count is exhausted (no Graph).
	countFn := p.deliveryCountFn
	if countFn == nil {
		countFn = deliveryCount
	}
	if n, ok := countFn(msg); ok && shouldTerminateMaxDeliver(n, p.maxDeliver) {
		cause := fmt.Errorf("max deliver exceeded: %d", n)
		log.Error("max deliver exhausted, dead-lettering", cause,
			loggy.Kv("numDelivered", n),
			loggy.Kv("maxDeliver", p.maxDeliver),
		)
		p.writeDeadLetter(ctx, msg.Data, cause)
		p.writeAudit(ctx, req, domain.StatusFailed, cause.Error())
		if len(req.Attachments) > 0 && p.attStore != nil {
			p.attStore.Cleanup(req.Attachments)
		}
		if p.termFn != nil {
			_ = p.termFn(msg)
		} else {
			terminalStop(msg)
		}
		return
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
