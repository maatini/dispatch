package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
	"dispatch/internal/loggy"
	"dispatch/internal/natsutil"
	"dispatch/internal/sender"
)

var resolverLog = loggy.GetLogger("Resolver")

// Resolver holds all dependencies for GraphQL resolvers.
type Resolver struct {
	senders       *sender.Store
	js            nats.JetStreamContext
	mailPublisher mailPublisher
}

type mailPublisher interface {
	PublishMsg(*nats.Msg, ...nats.PubOpt) (*nats.PubAck, error)
}

func NewResolver(senders *sender.Store, js nats.JetStreamContext) *Resolver {
	return &Resolver{senders: senders, js: js, mailPublisher: js}
}

// --- Query ---

type senderFilterArgs struct {
	AppTag *string
}

func (r *Resolver) Senders(ctx context.Context, args struct{ Filter *senderFilterArgs }) ([]*senderGQL, error) {
	all, err := r.senders.List()
	if err != nil {
		return nil, err
	}
	result := make([]*senderGQL, 0, len(all))
	for _, s := range all {
		if args.Filter != nil && args.Filter.AppTag != nil && s.AppTag != *args.Filter.AppTag {
			continue
		}
		result = append(result, toSenderGQL(s))
	}
	return result, nil
}

type mailFilterArgs struct {
	AppTag  *string
	Status  *string
	TraceID *string
}

type pagedMailArgs struct {
	Filter *mailFilterArgs
	Page   *int32
	Size   *int32
}

func (r *Resolver) Mails(ctx context.Context, args pagedMailArgs) (*pagedMailResponse, error) {
	records, err := r.readAuditStream(ctx)
	if err != nil {
		return nil, err
	}

	filtered := records[:0]
	for _, rec := range records {
		if matchesMailFilter(rec, args.Filter) {
			filtered = append(filtered, rec)
		}
	}

	page, size := pageSize(args.Page, args.Size)
	items, total := paginate(filtered, page, size)
	gqlItems := make([]*mailRecordGQL, len(items))
	for i, item := range items {
		gqlItems[i] = toMailRecordGQL(item)
	}
	return &pagedMailResponse{items: gqlItems, total: int32(total)}, nil
}

type pagedBounceArgs struct {
	Page *int32
	Size *int32
}

func (r *Resolver) Bounces(ctx context.Context, args pagedBounceArgs) (*pagedBounceResponse, error) {
	records, err := r.readBounceStream(ctx)
	if err != nil {
		return nil, err
	}
	page, size := pageSize(args.Page, args.Size)
	items, total := paginate(records, page, size)
	gqlItems := make([]*bounceRecordGQL, len(items))
	for i, item := range items {
		gqlItems[i] = toBounceRecordGQL(item)
	}
	return &pagedBounceResponse{items: gqlItems, total: int32(total)}, nil
}

func (r *Resolver) DeadLetters(ctx context.Context, args pagedBounceArgs) (*pagedDeadLetterResponse, error) {
	records, err := r.readDeadLetterStream(ctx)
	if err != nil {
		return nil, err
	}
	page, size := pageSize(args.Page, args.Size)
	items, total := paginate(records, page, size)
	gqlItems := make([]*deadLetterGQL, len(items))
	for i, item := range items {
		gqlItems[i] = toDeadLetterGQL(item)
	}
	return &pagedDeadLetterResponse{items: gqlItems, total: int32(total)}, nil
}

// --- Mutation ---

type senderInputArgs struct {
	AppTag         string
	Email          string
	Test           bool
	DailyQuota     int32
	AllowedDomains *string
}

func (r *Resolver) CreateSender(_ context.Context, args struct{ Input senderInputArgs }) (*senderGQL, error) {
	s := fromSenderInput(args.Input)
	if err := r.senders.Put(s); err != nil {
		return nil, err
	}
	return toSenderGQL(s), nil
}

func (r *Resolver) UpdateSender(_ context.Context, args struct {
	AppTag string
	Input  senderInputArgs
}) (*senderGQL, error) {
	s := fromSenderInput(args.Input)
	s.AppTag = args.AppTag
	if err := r.senders.Put(s); err != nil {
		return nil, err
	}
	return toSenderGQL(s), nil
}

func (r *Resolver) DeleteSender(_ context.Context, args struct{ AppTag string }) (bool, error) {
	if err := r.senders.Delete(args.AppTag); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Resolver) ReprocessDeadLetter(ctx context.Context, args struct{ Payload string }) (bool, error) {
	var req domain.MailRequestDO
	if err := json.Unmarshal([]byte(args.Payload), &req); err != nil {
		return false, fmt.Errorf("invalid dead letter payload: %w", err)
	}

	traceID := req.TraceID
	if traceID == "" {
		traceID = uuid.New().String()
	}

	natMsg := nats.NewMsg(natsutil.SubjectMails)
	natMsg.Header.Set("traceId", traceID)
	natMsg.Header.Set("appTag", req.AppTag)
	natMsg.Data = []byte(args.Payload)

	if _, err := r.mailPublisher.PublishMsg(natMsg); err != nil {
		return false, fmt.Errorf("reprocess: %w", err)
	}
	resolverLog.Info("dead letter reprocessed", loggy.Kv("traceId", traceID))
	return true, nil
}

func matchesMailFilter(rec domain.AuditRecord, f *mailFilterArgs) bool {
	if f == nil {
		return true
	}
	if f.AppTag != nil && rec.AppTag != *f.AppTag {
		return false
	}
	if f.Status != nil && rec.Status != *f.Status {
		return false
	}
	if f.TraceID != nil && rec.TraceID != *f.TraceID {
		return false
	}
	return true
}

// --- stream helpers ---

func (r *Resolver) readAuditStream(ctx context.Context) ([]domain.AuditRecord, error) {
	return readStream[domain.AuditRecord](ctx, r.js, natsutil.StreamAudit)
}

func (r *Resolver) readBounceStream(ctx context.Context) ([]domain.BounceRecord, error) {
	return readStream[domain.BounceRecord](ctx, r.js, natsutil.StreamBounces)
}

func (r *Resolver) readDeadLetterStream(ctx context.Context) ([]domain.DeadLetter, error) {
	return readStream[domain.DeadLetter](ctx, r.js, natsutil.StreamDeadLetter)
}

func readStream[T any](ctx context.Context, js nats.JetStreamContext, stream string) ([]T, error) {
	info, err := js.StreamInfo(stream)
	if err != nil {
		return nil, fmt.Errorf("stream info %s: %w", stream, err)
	}
	if info.State.Msgs == 0 {
		return nil, nil
	}

	sub, err := js.SubscribeSync("", nats.BindStream(stream), nats.DeliverAll())
	if err != nil {
		return nil, fmt.Errorf("subscribe %s: %w", stream, err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	var results []T
	consumed := uint64(0)
	for consumed < info.State.Msgs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		msg, err := sub.NextMsg(5 * time.Second)
		if err != nil {
			break
		}
		consumed++
		var item T
		if jsonErr := json.Unmarshal(msg.Data, &item); jsonErr == nil {
			results = append(results, item)
		}
	}
	return results, nil
}

// --- GQL types ---

type senderGQL struct {
	appTag         string
	email          string
	test           bool
	dailyQuota     int32
	allowedDomains *string
}

func (s *senderGQL) AppTag() string          { return s.appTag }
func (s *senderGQL) Email() string           { return s.email }
func (s *senderGQL) Test() bool              { return s.test }
func (s *senderGQL) DailyQuota() int32       { return s.dailyQuota }
func (s *senderGQL) AllowedDomains() *string { return s.allowedDomains }

type mailRecordGQL struct {
	traceID    string
	appTag     string
	status     string
	sender     string
	subject    *string
	recipients []string
	erro       *string
	timestamp  string
}

func (m *mailRecordGQL) TraceId() string      { return m.traceID }
func (m *mailRecordGQL) AppTag() string       { return m.appTag }
func (m *mailRecordGQL) Status() string       { return m.status }
func (m *mailRecordGQL) Sender() string       { return m.sender }
func (m *mailRecordGQL) Subject() *string     { return m.subject }
func (m *mailRecordGQL) Recipients() []string { return m.recipients }
func (m *mailRecordGQL) Error() *string       { return m.erro }
func (m *mailRecordGQL) Timestamp() string    { return m.timestamp }

type bounceRecordGQL struct {
	originalTraceID  string
	bouncedAt        string
	bounceReason     string
	bouncedRecipient string
	processedAt      string
}

func (b *bounceRecordGQL) OriginalTraceId() string  { return b.originalTraceID }
func (b *bounceRecordGQL) BouncedAt() string        { return b.bouncedAt }
func (b *bounceRecordGQL) BounceReason() string     { return b.bounceReason }
func (b *bounceRecordGQL) BouncedRecipient() string { return b.bouncedRecipient }
func (b *bounceRecordGQL) ProcessedAt() string      { return b.processedAt }

type deadLetterGQL struct {
	payload   string
	erro      string
	timestamp string
}

func (d *deadLetterGQL) Payload() string   { return d.payload }
func (d *deadLetterGQL) Error() string     { return d.erro }
func (d *deadLetterGQL) Timestamp() string { return d.timestamp }

type pagedMailResponse struct {
	items []*mailRecordGQL
	total int32
}

func (p *pagedMailResponse) Items() []*mailRecordGQL { return p.items }
func (p *pagedMailResponse) Total() int32            { return p.total }

type pagedBounceResponse struct {
	items []*bounceRecordGQL
	total int32
}

func (p *pagedBounceResponse) Items() []*bounceRecordGQL { return p.items }
func (p *pagedBounceResponse) Total() int32              { return p.total }

type pagedDeadLetterResponse struct {
	items []*deadLetterGQL
	total int32
}

func (p *pagedDeadLetterResponse) Items() []*deadLetterGQL { return p.items }
func (p *pagedDeadLetterResponse) Total() int32            { return p.total }

func toSenderGQL(s domain.Sender) *senderGQL {
	g := &senderGQL{
		appTag:     s.AppTag,
		email:      s.Email,
		test:       s.Test,
		dailyQuota: int32(s.DailyQuota),
	}
	if s.AllowedDomains != "" {
		g.allowedDomains = &s.AllowedDomains
	}
	return g
}

func fromSenderInput(input senderInputArgs) domain.Sender {
	s := domain.Sender{
		AppTag:     input.AppTag,
		Email:      input.Email,
		Test:       input.Test,
		DailyQuota: int(input.DailyQuota),
	}
	if input.AllowedDomains != nil {
		s.AllowedDomains = *input.AllowedDomains
	}
	return s
}

func toMailRecordGQL(r domain.AuditRecord) *mailRecordGQL {
	g := &mailRecordGQL{
		traceID:    r.TraceID,
		appTag:     r.AppTag,
		status:     r.Status,
		sender:     r.Sender,
		recipients: r.Recipients,
		timestamp:  r.Timestamp.String(),
	}
	if r.Subject != "" {
		g.subject = &r.Subject
	}
	if r.Error != "" {
		g.erro = &r.Error
	}
	return g
}

func toBounceRecordGQL(r domain.BounceRecord) *bounceRecordGQL {
	return &bounceRecordGQL{
		originalTraceID:  r.OriginalTraceID,
		bouncedAt:        r.BouncedAt.String(),
		bounceReason:     r.BounceReason,
		bouncedRecipient: r.BouncedRecipient,
		processedAt:      r.ProcessedAt.String(),
	}
}

func toDeadLetterGQL(dl domain.DeadLetter) *deadLetterGQL {
	return &deadLetterGQL{
		payload:   dl.Payload,
		erro:      dl.Error,
		timestamp: dl.Timestamp.String(),
	}
}

func pageSize(page, size *int32) (int, int) {
	p, s := 0, 50
	if page != nil && *page > 0 {
		p = int(*page)
	}
	if size != nil && *size > 0 {
		s = int(*size)
	}
	return p, s
}

func paginate[T any](items []T, page, size int) ([]T, int) {
	total := len(items)
	start := page * size
	if start >= total {
		return nil, total
	}
	end := start + size
	if end > total {
		end = total
	}
	return items[start:end], total
}
