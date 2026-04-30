package worker

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"

	"dispatch/internal/natsutil"
)

// Consumer pulls messages from NATS JetStream and dispatches them to Processor.
type Consumer struct {
	js        nats.JetStreamContext
	processor *Processor
}

func NewConsumer(js nats.JetStreamContext, processor *Processor) *Consumer {
	return &Consumer{js: js, processor: processor}
}

func (c *Consumer) Run(ctx context.Context) error {
	sub, err := c.js.PullSubscribe(
		natsutil.SubjectMails,
		natsutil.ConsumerMailWorker,
		nats.Bind(natsutil.StreamMails, natsutil.ConsumerMailWorker),
	)
	if err != nil {
		return fmt.Errorf("pull subscribe: %w", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	slog.InfoContext(ctx, "mail worker consumer started")

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		msgs, err := sub.Fetch(10, nats.Context(ctx))
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			// transient fetch error — log and retry
			slog.WarnContext(ctx, "fetch error", slog.String("error", err.Error()))
			continue
		}

		for _, msg := range msgs {
			c.processor.Handle(ctx, msg)
		}
	}
}
