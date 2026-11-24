// The C half of the Go front end's boundary (docs/GO.md section 5): it builds the gk_app the
// library is entered with, in C memory, pointing at the one exported Go callback. cgo forbids
// definitions in the preamble of a file with //export, so this lives here.
#include "include/gpuikit.h"
#include "_cgo_export.h"

// The reply is handed to gk_reply here, after the Go callback has returned: the Go side writes it
// into C memory (gk_go_reply_buf), so the turn costs one crossing into Go and none back out.
static int32_t go_callback(void *user, gk_cb *cb, uint32_t kind, const uint8_t *req, size_t len) {
    uint8_t *out = 0;
    size_t n = 0;
    int32_t rc = gkGoCallback(user, cb, kind, (uint8_t *)req, len, &out, &n);
    if (n > 0) {
        gk_reply(cb, out, n); // copies before it returns
    }
    return rc;
}

// gk_go_run enters the library (gk_run, or gk_headless_run with a script when headless). The
// version fields come from the Go package; size_delta and no_callback exist only for the
// runtime's version test, which shows the library refusing a wrong gk_app.
int32_t gk_go_run(int headless, const uint8_t *script, size_t len, uint16_t major, uint16_t minor,
                  uint64_t hash, uint32_t size_delta, int no_callback) {
    gk_app app = {(uint32_t)sizeof(gk_app) - size_delta, major, minor, hash,
                  no_callback ? (gk_callback)0 : go_callback, (void *)0, 0};
    if (headless) {
        return gk_headless_run(&app, script, len);
    }
    return gk_run(&app);
}
