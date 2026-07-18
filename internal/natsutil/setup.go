package natsutil

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	StreamMails      = "DISPATCH_MAILS"
	StreamAudit      = "DISPATCH_AUDIT"
	StreamDeadLetter = "DISPATCH_DEAD_LETTERS"
	StreamBounces    = "DISPATCH_BOUNCES"

	SubjectMails      = "cody.mailing.job.request.mails"
	SubjectAudit      = "cody.mailing.audit"
	SubjectDeadLetter = "cody.mailing.deadletter"
	SubjectBounce     = "cody.mailing.bounce"

	BucketSenders     = "senders"
	BucketQuota       = "quota"
	BucketSpam        = "spam"
	BucketDelivered   = "delivered"
	BucketAttachments = "attachments"

	ConsumerMailWorker = "mail-worker"
)

// Connect establishes a NATS connection and returns the JetStream context.
func Connect(url string) (*nats.Conn, nats.JetStreamContext, error) {
	nc, err := nats.Connect(url,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(10),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("NATS connect %s: %w", url, err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("JetStream context: %w", err)
	}
	return nc, js, nil
}

// Setup provisions the streams and KV buckets shared by all services.
func Setup(js nats.JetStreamContext, spamTTL time.Duration) error {
	if err := ProvisionStreams(js); err != nil {
		return fmt.Errorf("provision streams: %w", err)
	}
	if err := ProvisionKVBuckets(js, spamTTL); err != nil {
		return fmt.Errorf("provision KV buckets: %w", err)
	}
	return nil
}

// ProvisionStreams ensures all required JetStream streams exist.
func ProvisionStreams(js nats.JetStreamContext) error {
	streams := []nats.StreamConfig{
		{
			Name:      StreamMails,
			Subjects:  []string{SubjectMails},
			Storage:   nats.FileStorage,
			Retention: nats.WorkQueuePolicy,
			MaxAge:    72 * time.Hour,
		},
		{
			Name:     StreamAudit,
			Subjects: []string{SubjectAudit},
			Storage:  nats.FileStorage,
			MaxAge:   30 * 24 * time.Hour,
		},
		{
			Name:     StreamDeadLetter,
			Subjects: []string{SubjectDeadLetter},
			Storage:  nats.FileStorage,
			MaxAge:   30 * 24 * time.Hour,
		},
		{
			Name:     StreamBounces,
			Subjects: []string{SubjectBounce},
			Storage:  nats.FileStorage,
			MaxAge:   30 * 24 * time.Hour,
		},
	}
	for _, cfg := range streams {
		if err := upsertStream(js, cfg); err != nil {
			return err
		}
	}
	return nil
}

func upsertStream(js nats.JetStreamContext, cfg nats.StreamConfig) error {
	_, err := js.StreamInfo(cfg.Name)
	if err == nats.ErrStreamNotFound {
		_, err = js.AddStream(&cfg)
		return err
	}
	if err != nil {
		return fmt.Errorf("stream info %s: %w", cfg.Name, err)
	}
	_, err = js.UpdateStream(&cfg)
	return err
}

// ProvisionKVBuckets ensures all required KV buckets exist.
func ProvisionKVBuckets(js nats.JetStreamContext, spamTTL time.Duration) error {
	buckets := []nats.KeyValueConfig{
		{Bucket: BucketSenders, Storage: nats.FileStorage},
		{Bucket: BucketQuota, Storage: nats.FileStorage, TTL: 25 * time.Hour},
		{Bucket: BucketSpam, Storage: nats.FileStorage, TTL: spamTTL},
		{Bucket: BucketDelivered, Storage: nats.FileStorage, TTL: 7 * 24 * time.Hour},
	}
	for _, cfg := range buckets {
		if err := upsertKV(js, cfg); err != nil {
			return err
		}
	}
	return nil
}

func upsertKV(js nats.JetStreamContext, cfg nats.KeyValueConfig) error {
	_, err := js.KeyValue(cfg.Bucket)
	if err == nats.ErrBucketNotFound {
		_, err = js.CreateKeyValue(&cfg)
		return err
	}
	return err
}

// ProvisionObjectStore ensures the attachment Object Store exists.
// TTL matches stream retention so orphaned objects are cleaned up automatically.
func ProvisionObjectStore(js nats.JetStreamContext) (nats.ObjectStore, error) {
	store, err := js.ObjectStore(BucketAttachments)
	if err == nats.ErrStreamNotFound {
		store, err = js.CreateObjectStore(&nats.ObjectStoreConfig{
			Bucket:  BucketAttachments,
			Storage: nats.FileStorage,
			TTL:     72 * time.Hour,
		})
	}
	if err != nil {
		return nil, fmt.Errorf("object store %s: %w", BucketAttachments, err)
	}
	return store, nil
}

// ProvisionWorkerConsumer ensures the durable pull consumer for the mail worker exists.
func ProvisionWorkerConsumer(js nats.JetStreamContext) error {
	_, err := js.ConsumerInfo(StreamMails, ConsumerMailWorker)
	if err == nats.ErrConsumerNotFound {
		_, err = js.AddConsumer(StreamMails, &nats.ConsumerConfig{
			Durable:       ConsumerMailWorker,
			AckPolicy:     nats.AckExplicitPolicy,
			MaxDeliver:    -1,
			AckWait:       30 * time.Second,
			FilterSubject: SubjectMails,
		})
		return err
	}
	return err
}
