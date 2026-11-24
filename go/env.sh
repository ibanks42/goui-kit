# Source this script before invoking `go build` or `go test` on this module or
# its example programs.
#
# By default, builds use CGO_ENABLED=0 without -tags=gkstatic so binaries load
# libgouikit dynamically at startup via purego (requiring no C compiler).
# Setting GK_STATIC=1 switches to CGO_ENABLED=1 and adds -tags=gkstatic so
# binaries link libgouikit_static.a statically via cgo.
GOFLAGS=${GOFLAGS:--mod=mod}
if [ "${GK_STATIC:-}" = 1 ]; then
  case " $GOFLAGS " in *" -tags=gkstatic "*) ;; *) GOFLAGS="$GOFLAGS -tags=gkstatic" ;; esac
  export CGO_ENABLED=1 GOFLAGS
else
  export CGO_ENABLED=0 GOFLAGS="${GOFLAGS// -tags=gkstatic/}"
fi
