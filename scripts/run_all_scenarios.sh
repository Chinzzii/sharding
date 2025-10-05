#!/usr/bin/env bash
set -euo pipefail

# run_all_scenarios.sh
# Runs the set of scenarios (each scenario runs single -> sharded via run_scenario.sh)
# Requires: ./scripts/run_scenario.sh in the same directory and executable.
# Builds demo-server and loadgen binaries once and reuses them for all scenarios.

RUNNER="./scripts/run_scenario.sh"
if [[ ! -x "$RUNNER" ]]; then
  echo "ERROR: $RUNNER not found or not executable. Place run_scenario.sh in ./scripts and 'chmod +x' it."
  exit 1
fi

# Cooldown between runs (seconds)
COOLDOWN=${COOLDOWN:-15}

# Port used by demo-server (must match what run_scenario.sh uses or pass with --port)
PORT=${PORT:-8080}

# Paths for binaries
BIN_DIR="./bin"
GO_DEMO_BIN="$BIN_DIR/demo-server"
GO_LOADGEN_BIN="$BIN_DIR/loadgen"

# --- Build binaries once (so run_scenario.sh does not rebuild each time) ---
echo "Building binaries once for all scenarios..."
mkdir -p "$BIN_DIR"
go build -o "$GO_DEMO_BIN" ./cmd/demo-server
go build -o "$GO_LOADGEN_BIN" ./cmd/loadgen

echo "Running all scenarios (each scenario runs single -> sharded)"
echo

# Each line: name|ops|concurrency|writeRatio|shards
SCENARIOS=(
  "baseline_light|1000|10|0.5|3"           # 1 Baseline - light load
  "typical_webapp|10000|50|0.3|3"          # 2 Typical web-app (mostly reads)
  "read_heavy_cachelike|10000|100|0.1|3"   # 3 Read-heavy high concurrency
  "write_heavy_ingest|10000|50|0.9|3"      # 4 Write-heavy ingestion
  "bursty_spike|2000|200|0.5|3"            # 5 Bursty spike (short high concurrency)
  "sustained_stress|50000|200|0.5|3"       # 6 Sustained high-load (long stress)
  "many_clients|10000|500|0.5|3"           # 7 High concurrency many clients
  "long_run_highvol|100000|100|0.6|3"      # 8 Long-run high-volume throughput
  "shard_scaling_base|20000|100|0.6|3"     # 9 Shard-scaling (will also run shards=5 after this)
)

# Helper to run a single scenario via run_scenario.sh
# runidx is optional (defaults to 1)
run_one() {
  local name=$1
  local ops=$2
  local conc=$3
  local wr=$4
  local shards=$5
  local runidx=${6:-1}   # default to 1 if not provided

  local scenename="${name}_run${runidx}"
  echo "------------------------------------------------------------"
  echo "Scenario: ${scenename}  (ops=${ops} concurrency=${conc} writeRatio=${wr} shards=${shards})"
  echo "Running: ${RUNNER} --ops ${ops} --concurrency ${conc} --writeRatio ${wr} --shards ${shards} --scenario ${scenename} --port ${PORT}"
  echo

  # Run and let run_scenario.sh handle server lifecycle and CSV appending
  bash "$RUNNER" --ops "${ops}" --concurrency "${conc}" --writeRatio "${wr}" --shards "${shards}" --scenario "${scenename}" --port "${PORT}"

  echo "Completed ${scenename}"
  echo "Sleeping ${COOLDOWN}s before next run..."
  sleep "${COOLDOWN}"
  echo
}

# Main loop over scenarios
for entry in "${SCENARIOS[@]}"; do
  IFS='|' read -r name ops conc wr shards <<< "$entry"
  # run the scenario (single -> sharded with shards value), default runidx=1
  run_one "$name" "$ops" "$conc" "$wr" "$shards"

  # Special-case for shard scaling: after running shards=3, also run shards=5
  if [[ "$name" == "shard_scaling_base" ]]; then
    run_one "${name}_shards5" "$ops" "$conc" "$wr" 5
  fi
done

echo "All scenarios complete. Results are in results.csv (or the CSV file run_scenario.sh uses)."
