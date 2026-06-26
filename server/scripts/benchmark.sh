#!/bin/bash
# benchmark.sh — measures producer throughput (events/sec)
# Uses parallel curl workers to simulate concurrent drivers
#
# Usage:
#   ./scripts/benchmark.sh              # default: 500 requests, 20 workers
#   ./scripts/benchmark.sh 1000 50      # 1000 requests, 50 workers

TOTAL=${1:-500}
WORKERS=${2:-20}
URL="http://localhost:8080/location"

# Fixed set of driver UUIDs (one per worker keeps partition ordering clean)
DRIVERS=(
  "550e8400-e29b-41d4-a716-446655440001"
  "550e8400-e29b-41d4-a716-446655440002"
  "550e8400-e29b-41d4-a716-446655440003"
  "550e8400-e29b-41d4-a716-446655440004"
  "550e8400-e29b-41d4-a716-446655440005"
  "550e8400-e29b-41d4-a716-446655440006"
  "550e8400-e29b-41d4-a716-446655440007"
  "550e8400-e29b-41d4-a716-446655440008"
  "550e8400-e29b-41d4-a716-446655440009"
  "550e8400-e29b-41d4-a716-446655440010"
  "550e8400-e29b-41d4-a716-446655440011"
  "550e8400-e29b-41d4-a716-446655440012"
  "550e8400-e29b-41d4-a716-446655440013"
  "550e8400-e29b-41d4-a716-446655440014"
  "550e8400-e29b-41d4-a716-446655440015"
  "550e8400-e29b-41d4-a716-446655440016"
  "550e8400-e29b-41d4-a716-446655440017"
  "550e8400-e29b-41d4-a716-446655440018"
  "550e8400-e29b-41d4-a716-446655440019"
  "550e8400-e29b-41d4-a716-446655440020"
)

echo "============================================"
echo "  Location Tracker — Throughput Benchmark"
echo "============================================"
echo "  Target : $URL"
echo "  Total  : $TOTAL requests"
echo "  Workers: $WORKERS parallel"
echo "--------------------------------------------"

# Check producer is up
if ! curl -sf http://localhost:8080/health > /dev/null; then
  echo "ERROR: Producer is not running at $URL"
  exit 1
fi

PER_WORKER=$(( TOTAL / WORKERS ))
SUCCESS=0
FAIL=0
TMPDIR=$(mktemp -d)

worker() {
  local worker_id=$1
  local count=$2
  local driver="${DRIVERS[$((worker_id % ${#DRIVERS[@]}))]}"
  local ok=0
  local fail=0

  for (( i=0; i<count; i++ )); do
    LAT=$(echo "12.9716 + ($RANDOM % 200 - 100) / 10000.0" | bc -l)
    LNG=$(echo "77.5946 + ($RANDOM % 200 - 100) / 10000.0" | bc -l)
    SPEED=$(echo "$RANDOM % 80 + 10" | bc)

    STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$URL" \
      -H "Content-Type: application/json" \
      -d "{\"driver_id\":\"$driver\",\"lat\":$LAT,\"lng\":$LNG,\"speed\":$SPEED}" \
      --max-time 5)

    if [ "$STATUS" = "202" ]; then
      ((ok++))
    else
      ((fail++))
    fi
  done

  echo "$ok $fail" > "$TMPDIR/worker_$worker_id"
}

# Start all workers in background
START_TIME=$(date +%s%3N)

for (( w=0; w<WORKERS; w++ )); do
  worker "$w" "$PER_WORKER" &
done

# Progress indicator
echo -n "  Running "
while jobs -r | grep -q worker 2>/dev/null; do
  echo -n "."
  sleep 0.5
done
wait
echo " done"

END_TIME=$(date +%s%3N)
ELAPSED_MS=$(( END_TIME - START_TIME ))
ELAPSED_S=$(echo "scale=2; $ELAPSED_MS / 1000" | bc)

# Aggregate results
for f in "$TMPDIR"/worker_*; do
  read ok fail < "$f"
  SUCCESS=$(( SUCCESS + ok ))
  FAIL=$(( FAIL + fail ))
done
rm -rf "$TMPDIR"

ACTUAL_TOTAL=$(( SUCCESS + FAIL ))
if [ "$ELAPSED_MS" -gt 0 ]; then
  RPS=$(echo "scale=1; $SUCCESS * 1000 / $ELAPSED_MS" | bc)
else
  RPS="N/A"
fi

echo ""
echo "============================================"
echo "  Results"
echo "============================================"
printf "  %-20s %s\n" "Total sent:"      "$ACTUAL_TOTAL"
printf "  %-20s %s\n" "Successful (202):" "$SUCCESS"
printf "  %-20s %s\n" "Failed:"          "$FAIL"
printf "  %-20s %s s\n" "Elapsed:"       "$ELAPSED_S"
printf "  %-20s %s req/s\n" "Throughput:"  "$RPS"
echo "--------------------------------------------"

# Redis check — how many unique drivers have a cached location?
if command -v redis-cli &> /dev/null; then
  CACHED=$(redis-cli keys "driver:latest:*" 2>/dev/null | wc -l | tr -d ' ')
  printf "  %-20s %s\n" "Redis cached drivers:" "$CACHED"
fi

echo "============================================"
