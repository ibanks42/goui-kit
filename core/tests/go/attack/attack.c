/* C shim forwarding gpuikit C ABI callbacks into the Go `attackCallback` export. */
#include "gpuikit.h"
#include "_cgo_export.h"

/* Bridges the `const uint8_t *` callback parameter to cgo's non-const `uint8_t *`. */
static int32_t attack_trampoline(void *user, gk_cb *cb, uint32_t kind, const uint8_t *req, size_t len) {
    return attackCallback(user, cb, kind, (uint8_t *)req, len);
}

/* Starts a headless libgpuikit session using the compiled ABI version and hash. */
int32_t attack_run(const uint8_t *script, size_t len) {
    gk_app app = {(uint32_t)sizeof(gk_app), GK_ABI_MAJOR, GK_ABI_MINOR, GK_API_HASH, attack_trampoline, (void *)0, 0};
    return gk_headless_run(&app, script, len);
}
