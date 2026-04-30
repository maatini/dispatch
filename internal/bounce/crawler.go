package bounce

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/nats-io/nats.go"

	"codymail-go/internal/domain"
	"codymail-go/internal/natsutil"
)

var traceIDRegex = regexp.MustCompile(`X-CodyMail-TraceId:\s*([0-9a-f-]{36})`)

type graphClient interface {
	GetUnreadMessages(ctx context.Context, mailbox string) ([]NDRMessage, error)
	MarkAsRead(ctx context.Context, mailbox, messageID string) error
}

// NDRMessage represents a non-delivery report from MS Graph.
type NDRMessage struct {
	ID      string
	Body    string
	Subject string
}

// Crawler reads NDR messages from a bounce mailbox and writes BounceRecords to NATS.
type Crawler struct {
	graph   graphClient
	js      nats.JetStreamContext
	mailbox string
}

func NewCrawler(graph graphClient, js nats.JetStreamContext, mailbox string) *Crawler {
	return &Crawler{graph: graph, js: js, mailbox: mailbox}
}

func (c *Crawler) Run(ctx context.Context) error {
	msgs, err := c.graph.GetUnreadMessages(ctx, c.mailbox)
	if err != nil {
		return fmt.Errorf("get unread messages: %w", err)
	}

	slog.InfoContext(ctx, "bounce crawler: found messages", slog.Int("count", len(msgs)))

	for _, msg := range msgs {
		if err := c.process(ctx, msg); err != nil {
			slog.WarnContext(ctx, "bounce process failed",
				slog.String("messageId", msg.ID),
				slog.String("error", err.Error()),
			)
		}
		if err := c.graph.MarkAsRead(ctx, c.mailbox, msg.ID); err != nil {
			slog.WarnContext(ctx, "mark as read failed",
				slog.String("messageId", msg.ID),
				slog.String("error", err.Error()),
			)
		}
	}
	return nil
}

func (c *Crawler) process(ctx context.Context, msg NDRMessage) error {
	traceID := extractTraceID(msg.Body)

	record := domain.BounceRecord{
		OriginalTraceID:  traceID,
		BouncedAt:        time.Now().UTC(),
		BounceReason:     msg.Subject,
		BouncedRecipient: "",
		ProcessedAt:      time.Now().UTC(),
	}

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal bounce record: %w", err)
	}

	if _, err := c.js.Publish(natsutil.SubjectBounce, data); err != nil {
		return fmt.Errorf("publish bounce record: %w", err)
	}

	slog.InfoContext(ctx, "bounce recorded",
		slog.String("traceId", traceID),
	)
	return nil
}

func extractTraceID(body string) string {
	m := traceIDRegex.FindStringSubmatch(body)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}
