package bounce

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/nats-io/nats.go"

	"dispatch/internal/domain"
	"dispatch/internal/loggy"
	"dispatch/internal/natsutil"
)

var crawlerLog = loggy.GetLogger("Crawler")

var traceIDRegex = regexp.MustCompile(`X-Dispatch-TraceId:\s*([0-9a-f-]{36})`)

type graphClient interface {
	GetUnreadMessages(ctx context.Context, mailbox string) ([]NDRMessage, error)
	MarkAsRead(ctx context.Context, mailbox, messageID string) error
}

type jsPublisher interface {
	Publish(subj string, data []byte, opts ...nats.PubOpt) (*nats.PubAck, error)
}

// NDRMessage represents a non-delivery report from MS Graph.
type NDRMessage struct {
	ID         string
	Body       string
	Subject    string
	Recipient  string
	ReceivedAt time.Time
}

// Crawler reads NDR messages from a bounce mailbox and writes BounceRecords to NATS.
type Crawler struct {
	graph   graphClient
	js      jsPublisher
	mailbox string
}

func NewCrawler(graph graphClient, js jsPublisher, mailbox string) *Crawler {
	return &Crawler{graph: graph, js: js, mailbox: mailbox}
}

func (c *Crawler) Run(ctx context.Context) error {
	msgs, err := c.graph.GetUnreadMessages(ctx, c.mailbox)
	if err != nil {
		return fmt.Errorf("get unread messages: %w", err)
	}

	crawlerLog.Info("bounce crawler: found messages", loggy.Kv("count", len(msgs)))

	for _, msg := range msgs {
		if err := c.process(ctx, msg); err != nil {
			crawlerLog.Warn("bounce process failed",
				loggy.Kv("messageId", msg.ID),
				loggy.Kv("error", err.Error()),
			)
			continue
		}
		if err := c.graph.MarkAsRead(ctx, c.mailbox, msg.ID); err != nil {
			crawlerLog.Warn("mark as read failed",
				loggy.Kv("messageId", msg.ID),
				loggy.Kv("error", err.Error()),
			)
		}
	}
	return nil
}

func (c *Crawler) process(ctx context.Context, msg NDRMessage) error {
	traceID := extractTraceID(msg.Body)

	bouncedAt := msg.ReceivedAt
	if bouncedAt.IsZero() {
		bouncedAt = time.Now().UTC()
	}

	record := domain.BounceRecord{
		OriginalTraceID:  traceID,
		BouncedAt:        bouncedAt,
		BounceReason:     msg.Subject,
		BouncedRecipient: msg.Recipient,
		ProcessedAt:      time.Now().UTC(),
	}

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal bounce record: %w", err)
	}

	if _, err := c.js.Publish(natsutil.SubjectBounce, data); err != nil {
		return fmt.Errorf("publish bounce record: %w", err)
	}

	crawlerLog.Info("bounce recorded", loggy.Kv("traceId", traceID))
	return nil
}

func extractTraceID(body string) string {
	m := traceIDRegex.FindStringSubmatch(body)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}
