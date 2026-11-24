//go:build gkstatic

// C bridge compiled only in static cgo builds (-tags gkstatic).
// It wraps the Go-exported turn callback and populates gk_app before invoking
// gk_run or gk_headless_run.

#include "include/gpuikit.h"
#include "_cgo_export.h"

// go_callback forwards a library turn callback into Go and sends any reply
// tape produced by gkGoCallback back to the Rust core via gk_reply.
static int32_t go_callback(void *user, gk_cb *cb, uint32_t kind, const uint8_t *req, size_t len) {
    uint8_t *out = 0;
    size_t n = 0;
    int32_t rc = gkGoCallback(user, cb, kind, (uint8_t *)req, len, &out, &n);
    if (n > 0) {
        gk_reply(cb, out, n);
    }
    return rc;
}

// gk_go_run fills a gk_app descriptor and enters either the interactive event
// loop (gk_run) or the scripted headless loop (gk_headless_run).
// size_delta and no_callback are used by protocol tests to probe ABI checks.
int32_t gk_go_run(int headless, const uint8_t *script, size_t len, uint16_t major, uint16_t minor,
                  uint64_t hash, uint32_t size_delta, int no_callback) {
    gk_app app = {(uint32_t)sizeof(gk_app) - size_delta, major, minor, hash,
                  no_callback ? (gk_callback)0 : go_callback, (void *)0, 0};
    if (headless) {
        return gk_headless_run(&app, script, len);
    }
    return gk_run(&app);
}
