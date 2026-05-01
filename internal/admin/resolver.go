package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
	"dispatch/internal/natsutil"
	"dispatch/internal/sender"
)

// Resolver holds all dependencies for GraphQL resolvers.
type Resolver struct {
	senders *sender.Store
	js      nats.JetStreamContext
}

func NewResolver(senders *sender.Store, js nats.JetStreamContext) *Resolver {
	return &Resolver{senders: senders, js: js}
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
	return &pagedMailResponse{Items: gqlItems, Total: int32(total)}, nil
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
	return &pagedBounceResponse{Items: gqlItems, Total: int32(total)}, nil
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
	return &pagedDeadLetterResponse{Items: gqlItems, Total: int32(total)}, nil
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
	if _, err := r.js.Publish(natsutil.SubjectMails, []byte(args.Payload)); err != nil {
		return false, fmt.Errorf("reprocess: %w", err)
	}
	slog.InfoContext(ctx, "dead letter reprocessed")
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

func (r *Resolver) readAuditStream(_ context.Context) ([]domain.AuditRecord, error) {
	return readStream[domain.AuditRecord](r.js, natsutil.StreamAudit)
}

func (r *Resolver) readBounceStream(_ context.Context) ([]domain.BounceRecord, error) {
	return readStream[domain.BounceRecord](r.js, natsutil.StreamBounces)
}

func (r *Resolver) readDeadLetterStream(_ context.Context) ([]domain.DeadLetter, error) {
	return readStream[domain.DeadLetter](r.js, natsutil.StreamDeadLetter)
}

func readStream[T any](js nats.JetStreamContext, stream string) ([]T, error) {
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
	for {
		msg, err := sub.NextMsg(0)
		if err != nil {
			break
		}
		var item T
		if jsonErr := json.Unmarshal(msg.Data, &item); jsonErr == nil {
			results = append(results, item)
		}
		if uint64(len(results)) >= info.State.Msgs {
			break
		}
	}
	return results, nil
}

// --- GQL types ---

type senderGQL struct {
	AppTag         string
	Email          string
	Test           bool
	DailyQuota     int32
	AllowedDomains *string
}

type mailRecordGQL struct {
	TraceID    string
	AppTag     string
	Status     string
	Sender     string
	Subject    *string
	Recipients []string
	Error      *string
	Timestamp  string
}

type bounceRecordGQL struct {
	OriginalTraceID  string
	BouncedAt        string
	BounceReason     string
	BouncedRecipient string
	ProcessedAt      string
}

type deadLetterGQL struct {
	Payload   string
	Error     string
	Timestamp string
}

type pagedMailResponse struct {
	Items []*mailRecordGQL
	Total int32
}

type pagedBounceResponse struct {
	Items []*bounceRecordGQL
	Total int32
}

type pagedDeadLetterResponse struct {
	Items []*deadLetterGQL
	Total int32
}

func toSenderGQL(s domain.Sender) *senderGQL {
	g := &senderGQL{
		AppTag:     s.AppTag,
		Email:      s.Email,
		Test:       s.Test,
		DailyQuota: int32(s.DailyQuota),
	}
	if s.AllowedDomains != "" {
		g.AllowedDomains = &s.AllowedDomains
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
		TraceID:    r.TraceID,
		AppTag:     r.AppTag,
		Status:     r.Status,
		Sender:     r.Sender,
		Recipients: r.Recipients,
		Timestamp:  r.Timestamp.String(),
	}
	if r.Subject != "" {
		g.Subject = &r.Subject
	}
	if r.Error != "" {
		g.Error = &r.Error
	}
	return g
}

func toBounceRecordGQL(r domain.BounceRecord) *bounceRecordGQL {
	return &bounceRecordGQL{
		OriginalTraceID:  r.OriginalTraceID,
		BouncedAt:        r.BouncedAt.String(),
		BounceReason:     r.BounceReason,
		BouncedRecipient: r.BouncedRecipient,
		ProcessedAt:      r.ProcessedAt.String(),
	}
}

func toDeadLetterGQL(dl domain.DeadLetter) *deadLetterGQL {
	return &deadLetterGQL{
		Payload:   dl.Payload,
		Error:     dl.Error,
		Timestamp: dl.Timestamp.String(),
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
