# go/env.sh: sourced by the Go scripts (examples/go/build.sh, gates/go.sh and the other Go gates).
# The bindings use cgo, and the modules here have no outside dependencies (gpuikit.dev is replaced
# by a local folder), so the go command never needs the network. Your own GOCACHE, GOPATH and Go
# toolchain are used as they are.
# The repository's own Go builds link the shared library (build tag gkshared), so a test can run
# the same program against lib/ship or lib/diag; gates/go-apps.sh with GK_STATIC=1 checks the
# static build, the default for an app (docs/GO.md section 8).
export CGO_ENABLED=1 GOPROXY=${GOPROXY:-off} GOFLAGS=${GOFLAGS:--mod=mod -tags=gkshared}
