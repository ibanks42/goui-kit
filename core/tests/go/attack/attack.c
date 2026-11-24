// The C half of the raw client (core/tests/go/attack/main.go): the gk_app it enters the library
// with, pointing at its own exported Go callback. cgo forbids definitions in the preamble of a file
// with //export, so this lives here.
#include "gpuikit.h"
#include "_cgo_export.h"

static int32_t attack_trampoline(void *user, gk_cb *cb, uint32_t kind, const uint8_t *req, size_t len) {
    return attackCallback(user, cb, kind, (uint8_t *)req, len);
}

int32_t attack_run(const uint8_t *script, size_t len) {
    gk_app app = {(uint32_t)sizeof(gk_app), GK_ABI_MAJOR, GK_ABI_MINOR, GK_API_HASH, attack_trampoline, (void *)0, 0};
    return gk_headless_run(&app, script, len);
}
