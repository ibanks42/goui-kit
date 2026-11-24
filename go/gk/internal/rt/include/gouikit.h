/* The whole C interface of libgouikit: seven functions and one callback
 * (docs/DESIGN.md section 5). Numbers are in gk_ops.h, generated from wire/ops.txt. */
#ifndef GOUIKIT_H
#define GOUIKIT_H
#include <stddef.h>
#include <stdint.h>
#include "gk_ops.h"

#define GK_MAGIC 0x314b5047u /* "GPK1" */

typedef struct gk_info {
    uint32_t size, magic;
    uint16_t abi_major, abi_minor;
    uint64_t api_hash;          /* hash of wire/ops.txt and api/opcodes.lock */
    uint32_t features;          /* GK_FEAT_* */
    const char *versions;       /* kit, gpui-fast, rustc; not NUL-terminated */
    size_t versions_len;
} gk_info;

typedef struct gk_cb gk_cb;     /* valid only inside the callback that received it */
typedef int32_t (*gk_callback)(void *user, gk_cb *cb, uint32_t kind,
                               const uint8_t *req, size_t len);
typedef struct gk_app {
    uint32_t size;
    uint16_t abi_major, abi_minor;
    uint64_t api_hash;
    gk_callback callback;
    void *user;
    uint32_t flags;
} gk_app;

const gk_info *gk_info_get(void);                                   /* any thread */
int32_t gk_run(const gk_app *app);                                  /* blocks; caller becomes the UI thread */
int32_t gk_call(const uint8_t *req, size_t len,
                const uint8_t **res, size_t *res_len);              /* UI thread, inside a callback */
int32_t gk_reply(gk_cb *cb, const uint8_t *out, size_t len);        /* copies; inside that callback */
int32_t gk_post(uint64_t token);                                    /* any thread */
const char *gk_last_error(size_t *len);                             /* thread-local */
int32_t gk_headless_run(const gk_app *app, const uint8_t *script, size_t len);  /* diag variant only */
#endif
