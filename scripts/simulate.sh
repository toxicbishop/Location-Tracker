#!/bin/bash
# simulate.sh — sends fake GPS events to the producer API
# Usage: ./scripts/simulate.sh
# Simulates 3 drivers moving around Bengaluru

PRODUCER_URL="http://localhost:8080/location"

DRIVERS=(
  "550e8400-e29b-41d4-a716-446655440001"
  "550e8400-e29b-41d4-a716-446655440002"
  "550e8400-e29b-41d4-a716-446655440003"
)

# Bengaluru center coords with small random drift
BASE_LAT=12.9716
BASE_LNG=77.5946

echo "Starting GPS simulation — press Ctrl+C to stop"

while true; do
  for DRIVER in "${DRIVERS[@]}"; do
    # Add small random offset to simulate movement
    LAT=$(echo "$BASE_LAT + ($RANDOM % 100 - 50) / 10000.0" | bc -l)
    LNG=$(echo "$BASE_LNG + ($RANDOM % 100 - 50) / 10000.0" | bc -l)
    SPEED=$(echo "$RANDOM % 80 + 10" | bc)

    PAYLOAD=$(printf '{"driver_id":"%s","lat":%s,"lng":%s,"speed":%s}' \
      "$DRIVER" "$LAT" "$LNG" "$SPEED")

    STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
      -X POST "$PRODUCER_URL" \
      -H "Content-Type: application/json" \
      -d "$PAYLOAD")

    echo "[$STATUS] driver=${DRIVER:0:8}... lat=$LAT lng=$LNG speed=$SPEED"
  done

  sleep 2  # send every 2 seconds per driver
done
