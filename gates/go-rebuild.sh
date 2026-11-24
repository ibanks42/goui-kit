#!/bin/bash
# Incremental Go rebuild-latency gate (`gates/go-rebuild.sh`).
# Gates: edit-compile turnaround for a small Go app importing go/gk.
# Method: copies examples/go/go.mod and examples/go/counter into a scratch directory, warms the Go
# build cache once, then times 3 one-line edits to counter/main.go in milliseconds and takes the
# median. Also measures one cold build with an empty GOCACHE for context.
# PASS: median incremental rebuild time is 5000 ms or less (`GATE rebuild go PASS`).

here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
. "$repo/go/env.sh"
lib=${GK_LIB_ROOT:-$repo/lib}/ship
scratch=${GK_SCRATCH:-${TMPDIR:-/tmp}}/go-rebuild.$$
rm -rf "$scratch"; mkdir -p "$scratch"
trap 'rm -rf "$scratch"' EXIT
cp -r "$repo/examples/go/go.mod" "$repo/examples/go/counter" "$scratch/"
sed -i "s#=> ../../go#=> $repo/go#" "$scratch/go.mod"
cd "$scratch"
export CGO_LDFLAGS="-L$lib"
# Warm up the package build cache before timing one-line edits.
go build ${GK_REBUILD_FLAGS:-} -o app ./counter || exit 1
times=()
for i in 1 2 3; do
  sed -i "s/fmt.Sprintf(\"count [^\"]*\"/fmt.Sprintf(\"count $i %d\"/" counter/main.go
  grep -q "\"count $i %d\"" counter/main.go || { echo "go-rebuild: the edit did not apply"; exit 1; }
  t0=$(date +%s%N)
  go build ${GK_REBUILD_FLAGS:-} -o app ./counter || exit 1
  t=$(( ($(date +%s%N) - t0) / 1000000 ))
  times+=("$t"); echo "rebuild $i: $t ms"
done
# Median of the three sorted timings (line 2).
med=$(printf '%s\n' "${times[@]}" | sort -n | sed -n 2p)
echo "binary $(stat -c %s app) bytes"
mkdir -p "$scratch/cold"
t0=$(date +%s%N)
GOCACHE=$scratch/cold go build ${GK_REBUILD_FLAGS:-} -o app ./counter || exit 1
cold=$(( ($(date +%s%N) - t0) / 1000000 ))
echo "cold build (empty cache): $cold ms, cache $(du -sh "$scratch/cold" | cut -f1)"
echo "GATE rebuild go $([ "$med" -le 5000 ] && echo PASS || echo FAIL) after a one-line edit: median $med ms of ${times[*]} (go build ${GK_REBUILD_FLAGS:-, no flags}); cold $cold ms"
