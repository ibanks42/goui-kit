# Commands for working on the bindings themselves. `make help` lists them; docs/CONTRIBUTING.md
# explains them. App developers don't need any of this: they use a prebuilt download.
#
# Two kinds of work:
#   - Rust: the library (core/) and the Rust reference apps (ref/). `make lib` builds them with
#     cargo into the usual target/ folder and copies the results to lib/.
#   - Everything else: the generator (Python), the Go module, the examples and the tests. These
#     need Go and a library in lib/ (from `make lib` or a download).
#
# The gates draw frames offscreen and in a private display, so they run on Linux.

# Rendering engine of the shipped library: gpui-fast (default) or gpui (upstream GPUI).
ENGINE ?= gpui-fast
# Frame-cost run: refresh rate and scene.
HZ    ?= 240
SCENE ?= panels 60 64 2
# Logs of the long runs.
LOGS  ?= gates/out/logs

.PHONY: help gen status coverage lib lib-release dist cross check-windows \
        gates gates-quick proto docs fps pixels gallery gallery-go memory misuse counter rebuild \
        go-test go-proto-all go-apps go-static go-fps clean

help:
	@echo "Generate (Python only, a few seconds)"
	@echo "  make gen            regenerate the bindings from the extracted API in api/rustdoc: the Rust"
	@echo "                     decoders (core/src/gen), the Go packages (go/gk/**/gen_*), the opcode tables,"
	@echo "                     the coverage table and docs/STATUS.md"
	@echo "  make status         rewrite the coverage table in docs/STATUS.md from api/coverage.json"
	@echo "  make coverage       check that coverage of the upstream API has not gone down"
	@echo ""
	@echo "Build the library (Rust, 5-15 minutes, about 4 GB of disk and up to 12 GB of memory)"
	@echo "  make lib            release build for this machine: lib/ship (what apps link), lib/diag"
	@echo "                     (offscreen rendering and scripted input, for tests), the Rust reference"
	@echo "                     apps and lib/licences. ENGINE=gpui builds the ship library on upstream GPUI"
	@echo "  make lib-release    the same, linked against glibc 2.28 with zig so it runs on older Linux"
	@echo "                     distributions too (what the published downloads use)"
	@echo "  make dist           assemble the download an app developer gets into dist/ (from lib/)"
	@echo "  make check-windows  type-check the library for Windows from Linux (needs cargo-xwin)"
	@echo ""
	@echo "Test (Linux; need lib/ from make lib)"
	@echo "  make gates          every check, with one summary table (about 25 minutes)"
	@echo "  make gates-quick    the shorter set to run after each change (a few minutes)"
	@echo "  make proto          the runtime's protocol tests (make go-proto-all)"
	@echo "  make docs           every snippet in the guides builds and the tutorial runs"
	@echo "  make cross          compile the Go front end for macOS and Windows from Linux (under a minute)"
	@echo "  make pixels | gallery | misuse | counter | rebuild | memory    one check each"
	@echo "  make gallery-go     the same as make gallery: every page against the Rust reference"
	@echo "  make fps            one frame-cost run: HZ=$(HZ) SCENE='$(SCENE)'"
	@echo "  make go-test        the Go front end: examples, protocol tests, misuse cases, pixels"
	@echo "  make go-proto-all   every protocol test in Go: examples/go/proto and the families (core/tests/go)"
	@echo "  make go-apps        the Go apps and tutorial: notes against Rust, the others against themselves"
	@echo "  make go-static      the Go apps built as an app is: static, only the component families used"
	@echo "  make go-fps         frame cost of Go against Rust (about ten minutes)"
	@echo ""
	@echo "  make clean          delete bin/ and gates/out/ (keeps lib/ and target/)"

# Everything generated, in one command, from the committed extract (api/rustdoc). Fails when an
# upstream signature meets no generator rule. rustc then judges the Rust side in `make lib`
# (or, quicker, gen/round.sh: cargo check with errors charged to items), the Go compiler in
# `make gates`.
gen:
	python3 core/wire/gen.py
	python3 gen/gkgen.py gen
	@if command -v gofmt >/dev/null; then gofmt -w go/gk examples/go; else echo "gen: gofmt not found: the Go files are left as written"; fi
	python3 gates/conformance.py
	python3 gates/accept.py --check

status:
	python3 tools/status.py

coverage:
	gates/coverage.sh

lib:
	GK_REFS=1 GK_ENGINE=$(ENGINE) tools/build-lib.sh ship diag

lib-release:
	GK_REFS=1 GK_ENGINE=$(ENGINE) GK_TARGET=x86_64-unknown-linux-gnu.2.28 tools/build-lib.sh ship diag

dist:
	tools/dist.sh

# The Windows library itself is built on Windows (its release build needs the Windows SDK's
# fxc.exe). This checks everything else from Linux, with cargo-xwin and Microsoft's CRT and SDK
# libraries, in the debug profile, where gpui-fast compiles its shaders at run time.
check-windows:
	rustup target add x86_64-pc-windows-msvc
	cargo xwin check --locked --target x86_64-pc-windows-msvc -p gk-capi
	cargo xwin check --locked --target x86_64-pc-windows-msvc -p gk-capi --features diag

cross:
	gates/cross.sh

gates:
	@mkdir -p $(LOGS)
	gates/all.sh > $(LOGS)/gates.log 2>&1; rc=$$?; cat gates/out/summary.md; echo "log: $(LOGS)/gates.log"; exit $$rc

gates-quick:
	@mkdir -p $(LOGS)
	gates/all.sh quick > $(LOGS)/gates.log 2>&1; rc=$$?; cat gates/out/summary.md; echo "log: $(LOGS)/gates.log"; exit $$rc

# The runtime's own tests: one per protocol of docs/PROTOCOLS.md, offscreen.
proto: go-proto-all

docs:
	gates/docs-go.sh

fps:
	gates/fps.sh --hz $(HZ) $(SCENE)

pixels:
	gates/go.sh pixels

gallery:
	gates/gallery.sh

gallery-go:
	gates/gallery.sh

memory:
	gates/memory.sh

misuse:
	gates/go-misuse.sh

counter:
	gates/counter.sh

rebuild:
	gates/go-rebuild.sh

go-test:
	gates/go.sh build hello proto misuse pixels rebuild

# The protocol tests in Go (core/tests/go/README.md): the core ones (gates/go-proto.sh, after
# building bin/proto-go), every family's (core/tests/go/run.sh) and the sample by generation.
go-proto-all:
	examples/go/build.sh proto port
	gates/go-proto.sh; a=$$?; core/tests/go/run.sh; b=$$?; gates/go-gensample.sh; c=$$?; [ $$a = 0 ] && [ $$b = 0 ] && [ $$c = 0 ]

go-apps:
	gates/go-apps.sh

go-static:
	GK_STATIC=1 gates/go-apps.sh build notes todo reader expenses tutorial

go-fps:
	gates/go.sh build fps

clean:
	rm -rf bin gates/out
