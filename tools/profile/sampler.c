// sampler.c: a sampling profiler of a process's main thread, loaded with LD_PRELOAD, for a
// machine where perf may not be used (kernel.perf_event_paranoid above 2 and no root). A timer on the main thread's own CPU clock (CLOCK_THREAD_CPUTIME_ID, the clock the
// frame-cost gate measures) sends SIGPROF to that thread every GK_PROF_US microseconds of its
// CPU time (default 50; the kernel checks CPU timers at its tick, so in practice about once a
// millisecond of CPU); the handler records the interrupted instruction address, the monotonic
// time and the thread's CPU clock, so each sample is weighted by the CPU time since the one
// before. At exit the samples go to $GK_PROF_OUT.samples (u64 address, u64 ns since start,
// u64 thread CPU ns; little-endian) and the process's mappings to $GK_PROF_OUT.maps, for
// tools/profile/symbolize.py. Without GK_PROF_OUT it does nothing.
//   zig cc -shared -fPIC -O2 -o sampler.so tools/profile/sampler.c
#define _GNU_SOURCE
#include <fcntl.h>
#include <signal.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/syscall.h>
#include <time.h>
#include <ucontext.h>
#include <unistd.h>

#define MAX_SAMPLES (1u << 21)

struct sample { uint64_t ip, ns, cpu; };
static struct sample *buf;
static volatile uint32_t n;
static uint64_t t0;
static timer_t timer;
static int on;

static uint64_t clock_ns(clockid_t c) {
    struct timespec t;
    clock_gettime(c, &t);
    return (uint64_t)t.tv_sec * 1000000000u + (uint64_t)t.tv_nsec;
}
static uint64_t now_ns(void) { return clock_ns(CLOCK_MONOTONIC); }

static void handler(int sig, siginfo_t *si, void *ctx) {
    (void)sig; (void)si;
    ucontext_t *uc = ctx;
    uint32_t i = n;
    if (i < MAX_SAMPLES) {
        buf[i].ip = (uint64_t)uc->uc_mcontext.gregs[REG_RIP];
        buf[i].ns = now_ns() - t0;
        buf[i].cpu = clock_ns(CLOCK_THREAD_CPUTIME_ID);
        n = i + 1;
    }
}

__attribute__((constructor)) static void start(void) {
    const char *out = getenv("GK_PROF_OUT");
    if (!out || !*out) return;
    buf = mmap(NULL, sizeof(struct sample) * MAX_SAMPLES, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANONYMOUS, -1, 0);
    if (buf == MAP_FAILED) return;
    const char *us = getenv("GK_PROF_US");
    long period = (us ? atol(us) : 50) * 1000;
    struct sigaction sa;
    memset(&sa, 0, sizeof sa);
    sa.sa_sigaction = handler;
    sa.sa_flags = SA_SIGINFO | SA_RESTART;
    sigemptyset(&sa.sa_mask);
    sigaction(SIGPROF, &sa, NULL);
    struct sigevent sev;
    memset(&sev, 0, sizeof sev);
    sev.sigev_notify = SIGEV_THREAD_ID;
    sev.sigev_signo = SIGPROF;
    sev._sigev_un._tid = (pid_t)syscall(SYS_gettid);  // the constructor runs on the main thread
    if (timer_create(CLOCK_THREAD_CPUTIME_ID, &sev, &timer) != 0) return;
    struct itimerspec its = {{0, period}, {0, period}};
    t0 = now_ns();
    timer_settime(timer, 0, &its, NULL);
    on = 1;
}

static void dump(const char *path, const void *p, size_t len) {
    int fd = open(path, O_WRONLY | O_CREAT | O_TRUNC, 0644);
    if (fd < 0) return;
    const char *c = p;
    while (len > 0) {
        ssize_t w = write(fd, c, len);
        if (w <= 0) break;
        c += w; len -= (size_t)w;
    }
    close(fd);
}

__attribute__((destructor)) static void stop(void) {
    if (!on) return;
    on = 0;
    timer_delete(timer);
    const char *out = getenv("GK_PROF_OUT");
    char path[4096];
    snprintf(path, sizeof path, "%s.samples", out);
    dump(path, buf, sizeof(struct sample) * n);
    // the mappings, copied as they are
    int in = open("/proc/self/maps", O_RDONLY);
    if (in < 0) return;
    snprintf(path, sizeof path, "%s.maps", out);
    int fd = open(path, O_WRONLY | O_CREAT | O_TRUNC, 0644);
    char b[65536];
    ssize_t r;
    while (fd >= 0 && (r = read(in, b, sizeof b)) > 0) {
        if (write(fd, b, (size_t)r) != r) break;
    }
    close(in);
    if (fd >= 0) close(fd);
}
