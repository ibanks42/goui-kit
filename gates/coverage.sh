#!/bin/bash
# coverage.sh [--accept]: the coverage gate, two verdicts.
#   GATE coverage all       api/coverage.json (written by the generator; format in
#                           tools/status.py) is consistent: every function has a status, every
#                           row adds up, and the table in docs/STATUS.md is the current one
#   GATE coverage ratchet   nothing went backwards against gates/coverage-baseline.json: no
#                           component lost, none with less bound + bridged or less verified,
#                           none with more failed (gates/coverage-ratchet.py)
# --accept records the present state as the new baseline (commit it with its reason).
# The gallery's own ratchet (a page that was byte-identical must stay so) is in gallery.sh.
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
if [ "${1:-}" = --accept ]; then exec python3 "$here/coverage-ratchet.py" --accept; fi
fail=0
python3 "$repo/tools/status.py" --check || fail=1
python3 "$here/coverage-ratchet.py" || fail=1
exit $fail
