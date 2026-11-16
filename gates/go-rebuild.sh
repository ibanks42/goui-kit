#!/bin/bash
# go-rebuild.sh: the Go edit loop. Copies examples/go/counter to scratch, builds it once (the
# package gk is in GOCACHE after the first build), then changes one line and builds again, three
# times; prints each time and "GATE go-rebuild ... median". With GK_REBUILD_FLAGS (default
# empty: plain `go build`) the flags tried. Also a cold build of package gk alone into an empty
# cache of its own, deleted after, for the first build an app developer waits for.
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
go build ${GK_REBUILD_FLAGS:-} -o app ./counter || exit 1
times=()
for i in 1 2 3; do
  sed -i "s/fmt.Sprintf(\"count [^\"]*\"/fmt.Sprintf(\"count $i %d\"/" counter/main.go   # a new string each time
  grep -q "\"count $i %d\"" counter/main.go || { echo "go-rebuild: the edit did not apply"; exit 1; }
  t0=$(date +%s%N)
  go build ${GK_REBUILD_FLAGS:-} -o app ./counter || exit 1
  t=$(( ($(date +%s%N) - t0) / 1000000 ))
  times+=("$t"); echo "rebuild $i: $t ms"
done
med=$(printf '%s\n' "${times[@]}" | sort -n | sed -n 2p)
echo "binary $(stat -c %s app) bytes"
# The cold build of package gk (and its cgo step) into a cache of its own, as a first build.
mkdir -p "$scratch/cold"
t0=$(date +%s%N)
GOCACHE=$scratch/cold go build ${GK_REBUILD_FLAGS:-} -o app ./counter || exit 1
cold=$(( ($(date +%s%N) - t0) / 1000000 ))
echo "cold build (empty cache): $cold ms, cache $(du -sh "$scratch/cold" | cut -f1)"
echo "GATE rebuild go $([ "$med" -le 5000 ] && echo PASS || echo FAIL) after a one-line edit: median $med ms of ${times[*]} (go build ${GK_REBUILD_FLAGS:-, no flags}); cold $cold ms"
