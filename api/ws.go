package main

import (
	"context"
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

	// 1. Initial Position
	if last, err := db.GetLatestLocation(ctx, rdb, driverID); err == nil && last != nil {
		if err := conn.WriteJSON(last); err != nil {
			log.Warn().Err(err).Msg("ws initial write failed")
			return
		}
	}

	// 2. Subscribe to Redis updates
	pubsub := db.SubscribeToDriver(ctx, rdb, driverID)
	defer pubsub.Close()

	ch := pubsub.Channel()

	// 3. Monitor for disconnects
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				cancel()
				return
			}
		}
	}()

	// 4. Main Loop
	for {
		select {
		case msg := <-ch:
			if err := conn.WriteMessage(websocket.TextMessage, []byte(msg.Payload)); err != nil {
				log.Warn().Err(err).Msg("ws write error")
				return
			}
		case <-ctx.Done():
			log.Info().Str("driver_id", driverID).Msg("WebSocket disconnected")
			return
		}
	}
}

