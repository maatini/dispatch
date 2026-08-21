package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"

	"dispatch/internal/loggy"
	"dispatch/internal/metrics"
	"dispatch/internal/natsutil"
)

var consumerLog = loggy.GetLogger("Consumer")

const (
	// fetchBatch is 1 so AckWait/InProgress apply only to the message currently
	// in Handle. A larger batch would start AckWait on unprocessed messages.
	fetchBatch      = 1
	fetchErrBackoff = 500 * time.Millisecond
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

	consumerLog.Info("mail worker consumer started")

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		msgs, err := sub.Fetch(fetchBatch, nats.Context(ctx))
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			consumerLog.Warn("fetch error", loggy.Kv("error", err.Error()))
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(fetchErrBackoff):
			}
			continue
		}

		if len(msgs) > 0 {
			if md, mdErr := msgs[0].Metadata(); mdErr == nil {
				metrics.SetWorkerQueuePending(md.NumPending)
			}
		}
		for _, msg := range msgs {
			c.processor.Handle(ctx, msg)
		}
	}
}
