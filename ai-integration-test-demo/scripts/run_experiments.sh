#!/usr/bin/env bash
#
# BST-Agent Batch Experiment Runner
#
# Runs all experiment groups multiple times for statistical validity.
# Usage:
#   ./scripts/run_experiments.sh --api-key <KEY> [--runs 3] [--model glm-5.1] [--groups "dual step-only ..."]
#   ./scripts/run_experiments.sh --api-key codex [--runs 3] [--model gpt-5.4]   # Codex CLI provider
#
# Results are saved to results/formal/<model>/<group>-run<N>.log
# A summary CSV is generated at results/formal/<model>/summary.csv

set -euo pipefail

# ── Defaults ──────────────────────────────────────────────────
RUNS=3
MODEL="glm-5.1"
BASE_URL="https://open.bigmodel.cn/api/anthropic"
API_KEY=""
PORT=5400
PROJECT_DIR="."
EXP_GROUPS="dual step-only code-batch batch-only code-only l0 l1"
RETRY_MAX=3
RETRY_DELAY=120  # seconds to wait on 429 before retry
DRY_RUN=false

# ── Parse args ────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --api-key)    API_KEY="$2";    shift 2 ;;
    --runs)       RUNS="$2";       shift 2 ;;
    --model)      MODEL="$2";      shift 2 ;;
    --base-url)   BASE_URL="$2";   shift 2 ;;
    --port)       PORT="$2";       shift 2 ;;
    --groups)     EXP_GROUPS="$2";     shift 2 ;;
    --retry-delay) RETRY_DELAY="$2"; shift 2 ;;
    --dry-run)    DRY_RUN=true;    shift ;;
    -h|--help)
      echo "Usage: $0 --api-key <KEY> [--runs N] [--model MODEL] [--groups \"g1 g2 ...\"]"
      echo ""
      echo "Options:"
      echo "  --api-key KEY       LLM API key (required)"
      echo "  --runs N            Runs per group (default: 3)"
      echo "  --model MODEL       Model name (default: glm-5.1)"
      echo "  --base-url URL      API base URL"
      echo "  --port PORT         Server port (default: 5400)"
      echo "  --groups \"g1 g2\"    Space-separated groups to run"
      echo "                      Available: dual step-only code-batch batch-only code-only l0 l1"
      echo "  --retry-delay SECS  Wait time on 429 error (default: 120)"
      echo "  --dry-run           Print what would be run without executing"
      exit 0 ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

if [[ -z "$API_KEY" ]]; then
  echo "ERROR: --api-key is required (use 'codex' for Codex CLI provider)"
  exit 2
fi

# Auto-configure for Codex CLI provider
if [[ "$API_KEY" == "codex" ]]; then
  if [[ "$MODEL" == "glm-5.1" ]]; then
    MODEL="gpt-5.4"
  fi
  BASE_URL=""
  echo "NOTE: Using Codex CLI provider (model=$MODEL)"
fi

# ── Setup ─────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEMO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN="$DEMO_DIR/bin/server"
RESULTS_DIR="$DEMO_DIR/results/formal/$MODEL"

mkdir -p "$RESULTS_DIR"

# Build
echo "=== Building server ==="
cd "$DEMO_DIR"
go build -o "$BIN" ./cmd/server
echo "Build OK: $BIN"

# ── Helper: map group name to scenario/make-target ────────────
get_scenario() {
  case "$1" in
    dual)        echo "dual" ;;
    step-only)   echo "step-only" ;;
    code-batch)  echo "code-batch" ;;
    batch-only)  echo "batch-only" ;;
    code-only)   echo "code-only" ;;
    l0)          echo "l0" ;;
    l1)          echo "l1" ;;
    *) echo "$1" ;;
  esac
}

# ── Helper: run one experiment with retry ─────────────────────
run_one() {
  local group="$1"
  local run_num="$2"
  local scenario
  scenario="$(get_scenario "$group")"
  local log_file="$RESULTS_DIR/${group}-run${run_num}.log"
  local status_file="$RESULTS_DIR/${group}-run${run_num}.status"
  local meta_file="$RESULTS_DIR/${group}-run${run_num}.meta.json"

  # Skip if already completed
  if [[ -f "$status_file" ]] && grep -q "completed" "$status_file" 2>/dev/null; then
    echo "  [SKIP] $group run$run_num already completed"
    return 0
  fi

  local attempt=0
  while [[ $attempt -lt $RETRY_MAX ]]; do
    attempt=$((attempt + 1))
    echo "  [RUN]  $group run$run_num (attempt $attempt/$RETRY_MAX)"

    local start_time
    start_time="$(date +%s)"

    if [[ "$DRY_RUN" == "true" ]]; then
      echo "  [DRY]  Would run: $BIN -mode test -scenario $scenario -port $PORT -model $MODEL -base-url $BASE_URL"
      echo "completed" > "$status_file"
      return 0
    fi

    # Use a unique port per run to avoid conflicts if running in parallel
    local run_port=$((PORT + run_num))

    set +e
    API_KEY="$API_KEY" "$BIN" \
      -mode test \
      -scenario "$scenario" \
      -port "$run_port" \
      -model "$MODEL" \
      -base-url "$BASE_URL" \
      -project-dir "$PROJECT_DIR" \
      > "$log_file" 2>&1
    local exit_code=$?
    set -e

    local end_time
    end_time="$(date +%s)"
    local duration=$((end_time - start_time))

    # Write metadata
    cat > "$meta_file" <<METAEOF
{
  "group": "$group",
  "run": $run_num,
  "model": "$MODEL",
  "scenario": "$scenario",
  "start_time": "$(date -d @"$start_time" '+%Y-%m-%dT%H:%M:%S' 2>/dev/null || date -r "$start_time" '+%Y-%m-%dT%H:%M:%S' 2>/dev/null || echo "$start_time")",
  "duration_seconds": $duration,
  "exit_code": $exit_code,
  "attempt": $attempt
}
METAEOF

    if [[ $exit_code -eq 0 ]]; then
      echo "completed" > "$status_file"
      echo "  [OK]   $group run$run_num completed in ${duration}s"
      return 0
    fi

    # Check for 429 / rate limit
    if grep -qi "429\|rate.limit\|insufficient.balance\|too many requests" "$log_file" 2>/dev/null; then
      echo "  [429]  $group run$run_num hit rate limit (attempt $attempt)"
      if [[ $attempt -lt $RETRY_MAX ]]; then
        echo "  [WAIT] Sleeping ${RETRY_DELAY}s before retry..."
        sleep "$RETRY_DELAY"
      fi
    else
      echo "  [FAIL] $group run$run_num failed with exit code $exit_code (attempt $attempt)"
      if [[ $attempt -lt $RETRY_MAX ]]; then
        echo "  [WAIT] Sleeping 30s before retry..."
        sleep 30
      fi
    fi
  done

  echo "failed" > "$status_file"
  echo "  [FAIL] $group run$run_num exhausted $RETRY_MAX retries"
  return 1
}

# ── Main loop ─────────────────────────────────────────────────
echo ""
echo "=== Experiment Configuration ==="
echo "  Model:     $MODEL"
echo "  Base URL:  $BASE_URL"
echo "  Runs/group: $RUNS"
echo "  Groups:    $EXP_GROUPS"
echo "  Output:    $RESULTS_DIR/"
echo ""

total_groups=$(echo "$EXP_GROUPS" | wc -w)
total_runs=$((total_groups * RUNS))
completed=0
failed=0

echo "=== Starting $total_runs experiment runs ==="
echo ""

for group in $EXP_GROUPS; do
  echo "--- Group: $group ($RUNS runs) ---"
  for run_num in $(seq 1 "$RUNS"); do
    if run_one "$group" "$run_num"; then
      completed=$((completed + 1))
    else
      failed=$((failed + 1))
    fi
  done
  echo ""
done

# ── Generate summary ──────────────────────────────────────────
echo "=== Generating summary ==="

SUMMARY_FILE="$RESULTS_DIR/summary.csv"
echo "group,run,status,duration_seconds,exit_code" > "$SUMMARY_FILE"

for group in $EXP_GROUPS; do
  for run_num in $(seq 1 "$RUNS"); do
    local_status="unknown"
    local_duration="N/A"
    local_exit="N/A"
    meta="$RESULTS_DIR/${group}-run${run_num}.meta.json"
    status_f="$RESULTS_DIR/${group}-run${run_num}.status"

    if [[ -f "$status_f" ]]; then
      local_status="$(cat "$status_f")"
    fi
    if [[ -f "$meta" ]]; then
      local_duration="$(grep -o '"duration_seconds": [0-9]*' "$meta" | grep -o '[0-9]*' || echo "N/A")"
      local_exit="$(grep -o '"exit_code": [0-9]*' "$meta" | grep -o '[0-9]*' || echo "N/A")"
    fi
    echo "$group,$run_num,$local_status,$local_duration,$local_exit" >> "$SUMMARY_FILE"
  done
done

echo ""
echo "=========================================="
echo "  Experiment batch complete"
echo "  Completed: $completed / $total_runs"
echo "  Failed:    $failed / $total_runs"
echo "  Results:   $RESULTS_DIR/"
echo "  Summary:   $SUMMARY_FILE"
echo "=========================================="

# ── Run evaluation if all completed ──────────────────────────
if [[ $failed -eq 0 ]] && command -v python3 &>/dev/null; then
  echo ""
  echo "=== Running evaluation per group ==="
  for group in $EXP_GROUPS; do
    echo ""
    echo "--- Evaluation: $group ---"
    python3 "$SCRIPT_DIR/summarize_results.py" "$RESULTS_DIR" --pattern "${group}-run*.log" 2>/dev/null || true
  done
fi
