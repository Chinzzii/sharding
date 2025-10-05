#!/usr/bin/env bash
set -euo pipefail

# Defaults (change as needed)
OPS=10000
CONCURRENCY=50
WRITERATIO=0.5
SHARDS=3
PORT=8080
CSV_FILE="./results/results.csv"
SLEEP_AFTER_SERVER_START=5   # seconds to wait for server to bind

# Paths for binaries
BIN_DIR="./bin"
GO_DEMO_BIN="$BIN_DIR/demo-server"
GO_LOADGEN_BIN="$BIN_DIR/loadgen"

usage() {
  cat <<EOF
Usage: $0 [--ops N] [--concurrency N] [--writeRatio F] [--shards N] [--port N] [--csv file] [--scenario name]

Runs two experiments for the same knobs:
  1) single mode
  2) sharded mode with --shards=<SHARDS>

Appends result lines to the CSV file (creates header if missing).
EOF
  exit 1
}

# simple args parse
while [[ $# -gt 0 ]]; do
  case "$1" in
    --ops) OPS="$2"; shift 2 ;;
    --concurrency) CONCURRENCY="$2"; shift 2 ;;
    --writeRatio) WRITERATIO="$2"; shift 2 ;;
    --shards) SHARDS="$2"; shift 2 ;;
    --port) PORT="$2"; shift 2 ;;
    --csv) CSV_FILE="$2"; shift 2 ;;
    --scenario) SCENARIO_NAME="$2"; shift 2 ;;
    -h|--help) usage ;;
    *) echo "Unknown arg: $1"; usage ;;
  esac
done

SCENARIO_NAME=${SCENARIO_NAME:-"s${SHARDS}_o${OPS}_c${CONCURRENCY}_w${WRITERATIO}"}

# ensure parent dirs exist
mkdir -p "$(dirname "$CSV_FILE")"
mkdir -p logs "$BIN_DIR"
# ensure CSV header exists
if [[ ! -f "$CSV_FILE" ]]; then
  echo "scenario_name,mode,shards,ops,concurrency,writeRatio,throughput_ops_s,writes,reads,errs,avgLatency_ms,timestamp,notes" > "$CSV_FILE"
fi

# --- Build binaries once ---
echo "Building binaries..."
go build -o "$GO_DEMO_BIN" ./cmd/demo-server
go build -o "$GO_LOADGEN_BIN" ./cmd/loadgen

start_server() {
  local mode=$1
  local shards=$2
  local rundir="temp/${SCENARIO_NAME}/${mode}"
  mkdir -p "$rundir"

  local logfile="logs/${SCENARIO_NAME}_${mode}.log"
  echo "Starting demo-server mode=$mode shards=$shards dbdir=$rundir (logs -> $logfile)..." >&2

  DEMO_DB_DIR="$rundir" nohup "$GO_DEMO_BIN" --mode="$mode" --shards="$shards" --port="$PORT" >"$logfile" 2>&1 &
  local pid=$!
  sleep "$SLEEP_AFTER_SERVER_START"
  echo "$pid|$rundir"
}

stop_server() {
  local pid=$1
  echo "Stopping server pid=$pid..."

  if kill "$pid" 2>/dev/null; then
    for _ in {1..10}; do
      if ! kill -0 "$pid" 2>/dev/null; then
        echo "Server pid=$pid stopped gracefully."
        sleep 2
        return 0
      fi
      sleep 1
    done

    echo "Server pid=$pid did not stop gracefully. Force killing..."
    kill -9 "$pid" 2>/dev/null || true
  fi
  sleep 5
}

run_loadgen_capture() {
  echo "Running loadgen: ops=$OPS concurrency=$CONCURRENCY writeRatio=$WRITERATIO -> connecting to http://localhost:$PORT"
  "$GO_LOADGEN_BIN" --target "http://localhost:$PORT" --ops "$OPS" --concurrency "$CONCURRENCY" --writeRatio "$WRITERATIO" 2>&1
}

parse_loadgen_line() {
  local line="$1"
  if [[ "$line" =~ throughput=([0-9.]+)\ ops/s\ writes=([0-9]+)\ reads=([0-9]+)\ errs=([0-9]+)\ avgLatency=([0-9.]+)ms ]]; then
    echo "${BASH_REMATCH[1]},${BASH_REMATCH[2]},${BASH_REMATCH[3]},${BASH_REMATCH[4]},${BASH_REMATCH[5]}"
  else
    local t w r e a
    t=$(echo "$line" | sed -n 's/.*throughput=\([0-9.]*\).*/\1/p' || true)
    w=$(echo "$line" | sed -n 's/.*writes=\([0-9]*\).*/\1/p' || true)
    r=$(echo "$line" | sed -n 's/.*reads=\([0-9]*\).*/\1/p' || true)
    e=$(echo "$line" | sed -n 's/.*errs=\([0-9]*\).*/\1/p' || true)
    a=$(echo "$line" | sed -n 's/.*avgLatency=\([0-9.]*\)ms.*/\1/p' || true)
    echo "$t,$w,$r,$e,$a"
  fi
}

for MODE in single sharded; do
  if [[ "$MODE" == "single" ]]; then
    SHARD_ARG=1
  else
    SHARD_ARG="$SHARDS"
  fi

  PID_RUNDIR=$(start_server "$MODE" "$SHARD_ARG")
  PID="${PID_RUNDIR%%|*}"
  RUNDIR="${PID_RUNDIR##*|}"

  LG_OUT=$(run_loadgen_capture) || LG_OUT="$LG_OUT"$'\n'"(loadgen exited nonzero)"

  LAST_LINE=$(echo "$LG_OUT" | awk 'NF{line=$0} END{print line}')
  if [[ -z "$LAST_LINE" ]]; then
    echo "No loadgen output captured!"
    stop_server "$PID"
    exit 1
  fi

  PARSED=$(parse_loadgen_line "$LAST_LINE")
  IFS=',' read -r THROUGHPUT WRITES READS ERRS AVGLAT <<< "$PARSED"

  TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  echo "${SCENARIO_NAME},${MODE},${SHARD_ARG},${OPS},${CONCURRENCY},${WRITERATIO},${THROUGHPUT},${WRITES},${READS},${ERRS},${AVGLAT},${TIMESTAMP}," >> "$CSV_FILE"

  echo "=== Summary: mode=${MODE} shards=${SHARD_ARG} ==="
  echo "throughput(ops/s): ${THROUGHPUT}"
  echo "writes: ${WRITES}  reads: ${READS}  errs: ${ERRS}"
  echo "avgLatency_ms: ${AVGLAT}"
  echo "log (loadgen last line):"
  echo "$LAST_LINE"
  echo

  stop_server "$PID"
  sleep 5
done

echo "Results appended to $CSV_FILE (scenario=${SCENARIO_NAME})."
