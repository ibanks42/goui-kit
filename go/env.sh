# go/env.sh: sourced by the Go scripts (examples/go/build.sh, gates/go.sh and the other Go gates).
# The modules here depend on one outside module, purego (github.com/ebitengine/purego, pinned in
# go/go.mod and go/go.sum; gpuikit.dev is replaced by a local folder): the first build fetches it
# into Go's module cache through your GOPROXY, and no build after that needs the network. Your own
# GOCACHE, GOPATH, GOPROXY and Go toolchain are used as they are.
# By default the Go programs load the shared library at start-up through purego (docs/GO.md
# section 8): no C compiler, and cgo is off here so every gate shows that. The library is found
# through GK_LIB, the program's folder, or the system loader (the gates set LD_LIBRARY_PATH), so
# one build runs against lib/ship or lib/diag. GK_STATIC=1 builds the static way instead (build
# tag gkstatic, cgo, the static library linked into each program; gates/go-apps.sh checks it).
GOFLAGS=${GOFLAGS:--mod=mod}
if [ "${GK_STATIC:-}" = 1 ]; then
  case " $GOFLAGS " in *" -tags=gkstatic "*) ;; *) GOFLAGS="$GOFLAGS -tags=gkstatic" ;; esac
  export CGO_ENABLED=1 GOFLAGS
else
  export CGO_ENABLED=0 GOFLAGS="${GOFLAGS// -tags=gkstatic/}"
fi
