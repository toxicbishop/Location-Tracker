package main

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/pranav/location-tracker/db"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all for demo
	},
}

// GET /driver/{id}/ws
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Parse driver_id from path
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		http.Error(w, "driver_id required", http.StatusBadRequest)
		return
	}
	driverID := parts[2]

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	activeWebSocketConns.Inc()
	defer activeWebSocketConns.Dec()

	// 1. Initial Position: Send the latest cached location immediately
	// so the rider doesn't see a blank map while waiting for the next update.
	if last, err := db.GetLatestLocation(ctx, rdb, driverID); err == nil && last != nil {
		if err := conn.WriteJSON(last); err != nil {
			log.Printf("ws initial write failed: %v", err)
			return
		}
	}

	// 2. Subscribe to Redis updates for this driver
	pubsub := db.SubscribeToDriver(ctx, rdb, driverID)
	defer pubsub.Close()

	ch := pubsub.Channel()

	// 3. Monitor for disconnects in a separate goroutine
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				cancel() // kill the subscription loop
				return
			}
		}
	}()

	log.Printf("WebSocket connected: driver=%s", driverID)

	// 4. Main Loop: Push updates from Redis to WebSocket
	for {
		select {
		case msg := <-ch:
			// Redis message payload is the JSON string
			if err := conn.WriteMessage(websocket.TextMessage, []byte(msg.Payload)); err != nil {
				log.Printf("ws write error: %v", err)
				return
			}
		case <-ctx.Done():
			log.Printf("WebSocket disconnected: driver=%s", driverID)
			return
		}
	}
}
