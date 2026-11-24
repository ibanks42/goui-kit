#!/bin/bash
# Upstream API coverage gate (`gates/coverage.sh [--accept]`).
#
# What it gates:
#   1. Keeps the generated coverage table in `docs/STATUS.md` in sync with `api/coverage.json`
#      via `tools/status.py --check`.
#   2. Prevents coverage regressions against `gates/coverage-baseline.json` via
#      `gates/coverage-ratchet.py` (per-component and per-group bound+bridged, verified, failed).
#
# Usage:
#   Pass `--accept` after intentional coverage changes to rewrite `gates/coverage-baseline.json`.
# What PASS means (exit 0): both `tools/status.py --check` and `gates/coverage-ratchet.py` exit 0.

here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
if [ "${1:-}" = --accept ]; then exec python3 "$here/coverage-ratchet.py" --accept; fi
fail=0
python3 "$repo/tools/status.py" --check || fail=1
python3 "$here/coverage-ratchet.py" || fail=1
exit $fail
