package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/pranav/location-tracker/models"
	"github.com/rs/zerolog/log"
	"github.com/segmentio/kafka-go"
)



const dlqTopic = "gps.updates.dlq"

// DLQMessage wraps the original event with failure metadata.
// This gives you everything you need to debug or replay the event later.
type DLQMessage struct {
	OriginalEvent models.LocationEvent `json:"original_event"`
	FailureReason string               `json:"failure_reason"`
	FailedAt      time.Time            `json:"failed_at"`
	RetryCount    int                  `json:"retry_count"`
}

// DLQWriter wraps a kafka.Writer pointed at the DLQ topic.
type DLQWriter struct {
	writer *kafka.Writer
}

// NewDLQWriter creates a Kafka writer for the dead letter topic.
func NewDLQWriter(broker string) *DLQWriter {
	return &DLQWriter{
		writer: &kafka.Writer{
			Addr:         kafka.TCP(broker),
			Topic:        dlqTopic,
			Balancer:     &kafka.Hash{},
			BatchTimeout: 10 * time.Millisecond,
			// Don't retry DLQ writes — if the DLQ itself is unavailable,
			// log it and move on rather than blocking the consumer loop.
			MaxAttempts: 1,
		},
	}
}

// Send publishes a failed event to the DLQ with failure metadata.
// It is best-effort: if the DLQ write fails, we log and continue.
// This prevents a DLQ failure from cascading into a consumer deadlock.
func (d *DLQWriter) Send(ctx context.Context, event models.LocationEvent, reason string, retries int) {
	msg := DLQMessage{
		OriginalEvent: event,
		FailureReason: reason,
		FailedAt:      time.Now().UTC(),
		RetryCount:    retries,
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		log.Error().Err(err).Str("driver_id", event.DriverID).Msg("DLQ marshal failed, event lost")
		return
	}

	err = d.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(event.DriverID),
		Value: payload,
	})
	if err != nil {
		log.Error().Err(err).Str("driver_id", event.DriverID).Msg("DLQ write failed, event lost")
		return
	}

	log.Info().Str("driver_id", event.DriverID).Str("reason", reason).Msg("Queued failed event to DLQ")
}

// Close shuts down the DLQ writer.
func (d *DLQWriter) Close() error {
	return d.writer.Close()
}
