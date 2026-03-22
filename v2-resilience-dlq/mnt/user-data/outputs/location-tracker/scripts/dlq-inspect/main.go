package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/segmentio/kafka-go"
)

// Run this to inspect failed events sitting in the DLQ:
//   go run ./scripts/dlq-inspect/main.go
//
// It reads from the beginning of gps.updates.dlq and prints
// every failed event with its failure reason and timestamp.
// Use this to debug why events failed and decide whether to replay them.

const dlqTopic = "gps.updates.dlq"

type DLQMessage struct {
	OriginalEvent map[string]interface{} `json:"original_event"`
	FailureReason string                 `json:"failure_reason"`
	FailedAt      time.Time              `json:"failed_at"`
	RetryCount    int                    `json:"retry_count"`
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	broker := envOr("KAFKA_BROKER", "localhost:9092")

	// Read from the very beginning — offset -2 means earliest
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:   []string{broker},
		Topic:     dlqTopic,
		Partition: 0,
		MinBytes:  1,
		MaxBytes:  10e6,
		MaxWait:   2 * time.Second,
	})
	defer reader.Close()

	// Start from the very first message
	if err := reader.SetOffset(kafka.FirstOffset); err != nil {
		log.Fatalf("set offset: %v", err)
	}

	fmt.Printf("\n═══════════════════════════════════════════\n")
	fmt.Printf("  Dead Letter Queue Inspector\n")
	fmt.Printf("  Topic: %s | Broker: %s\n", dlqTopic, broker)
	fmt.Printf("═══════════════════════════════════════════\n\n")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count := 0
	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if count == 0 {
				fmt.Println("  No failed events in DLQ. Queue is empty.")
			}
			break
		}

		var dlqMsg DLQMessage
		if err := json.Unmarshal(msg.Value, &dlqMsg); err != nil {
			fmt.Printf("  [raw] offset=%d key=%s value=%s\n",
				msg.Offset, string(msg.Key), string(msg.Value))
			count++
			continue
		}

		count++
		driverID := "unknown"
		if id, ok := dlqMsg.OriginalEvent["driver_id"].(string); ok {
			driverID = id
		}

		fmt.Printf("  ─── Event #%d ───────────────────────────\n", count)
		fmt.Printf("  Driver    : %s\n", driverID)
		fmt.Printf("  Failed at : %s\n", dlqMsg.FailedAt.Format("2006-01-02 15:04:05 UTC"))
		fmt.Printf("  Retries   : %d\n", dlqMsg.RetryCount)
		fmt.Printf("  Reason    : %s\n", dlqMsg.FailureReason)

		if lat, ok := dlqMsg.OriginalEvent["lat"].(float64); ok {
			lng, _ := dlqMsg.OriginalEvent["lng"].(float64)
			fmt.Printf("  Location  : lat=%.5f lng=%.5f\n", lat, lng)
		}
		fmt.Println()
	}

	fmt.Printf("═══════════════════════════════════════════\n")
	fmt.Printf("  Total failed events: %d\n", count)
	fmt.Printf("═══════════════════════════════════════════\n\n")

	if count > 0 {
		fmt.Println("  To replay: restart the consumer after fixing the root cause.")
		fmt.Println("  Kafka retains DLQ messages for 7 days by default.")
	}
}
