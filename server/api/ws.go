package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/pranav/location-tracker/db"
	"github.com/rs/zerolog/log"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all for demo
	},
}

// GET /ws/driver/{id}
//
// Migrated from Redis Pub/Sub to Redis Streams.
//
// Why Streams beat Pub/Sub here:
//   - Pub/Sub is fire-and-forget. If the API restarts while a rider is connected,
//     the subscription is gone and they miss every update until they reconnect.
//   - Streams persist. On reconnect the client can pass ?since=<stream-id> and the
//     handler replays all messages since that ID — zero data loss.
//   - XREAD BLOCK holds the connection open and delivers messages server-push,
//     so latency is identical to pub/sub for the happy path.
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Parse driver_id from path /ws/driver/{id}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		http.Error(w, "driver_id required", http.StatusBadRequest)
		return
	}
	driverID := parts[2]

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("ws upgrade error")
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	activeWebSocketConns.Inc()
	defer activeWebSocketConns.Dec()

	log.Info().Str("driver_id", driverID).Msg("WebSocket connected")

	// 1. Send the latest cached position immediately so the rider sees something.
	if last, err := db.GetLatestLocation(ctx, rdb, driverID); err == nil && last != nil {
		if err := conn.WriteJSON(last); err != nil {
			log.Warn().Err(err).Msg("ws initial write failed")
			return
		}
	}

	// 2. Determine where to start reading the stream.
	//    ?since=<stream-id> lets a reconnecting client replay missed messages.
	//    Default "$" means "only new messages from now" (live mode).
	lastID := r.URL.Query().Get("since")
	if lastID == "" {
		lastID = "$"
	}

	// 3. Monitor for client disconnects in a background goroutine.
	//    When the client closes the WebSocket, cancel the context so XREAD unblocks.
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				cancel()
				return
			}
		}
	}()

	// 4. Stream loop: XREAD BLOCK waits for new messages.
	//    On context cancellation (disconnect or API shutdown), XREAD returns an error
	//    and we exit cleanly — no goroutine leak.
	for {
		select {
		case <-ctx.Done():
			log.Info().Str("driver_id", driverID).Msg("WebSocket disconnected")
			return
		default:
		}

		events, newLastID, err := db.ReadLocationStream(ctx, rdb, driverID, lastID, 500)
		if err != nil {
			if ctx.Err() != nil {
				// Context cancelled — normal shutdown
				return
			}
			log.Warn().Err(err).Str("driver_id", driverID).Msg("stream read error")
			return
		}

		lastID = newLastID

		for _, event := range events {
			payload, _ := json.Marshal(event)
			if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				log.Warn().Err(err).Msg("ws write error")
				return
			}
		}
	}
}
