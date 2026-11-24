# Top-level build, code-generation, and verification targets for gpui-kit-bindings.
# Typical workflow:
#   1. `make gen`        regenerate Rust decoders, Go packages, conformance pages, and API audits.
#   2. `make lib`        compile the Rust core (`lib/ship` and `lib/diag`) and Rust reference apps.
#   3. `make gates`      run the full verification suite, or `make gates-quick` for a shorter pass.
# Run `make help` for an interactive summary of every target.

# Overridable build and benchmark defaults:
#   ENGINE: GPUI backend crate (`gpui-fast` by default, or `gpui` for upstream GPUI).
#   HZ:     virtual display refresh rate passed to `gates/fps.sh` by `make fps`.
#   SCENE:  scene name and arguments passed to `gates/fps.sh` by `make fps`.
#   LOGS:   directory where `make gates` and `make gates-quick` save full run logs.
ENGINE ?= gpui-fast
HZ    ?= 240
SCENE ?= panels 60 64 2
LOGS  ?= gates/out/logs

.PHONY: help gen status coverage lib lib-release dist cross check-windows \
        gates gates-quick proto docs fps pixels gallery gallery-go memory misuse counter rebuild \
        go-test go-proto-all go-apps go-static go-fps clean

# Print a categorized summary of all targets and current default settings.
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

# Code generation and API coverage checks (Python only, no Rust compiler needed).
# `gen` regenerates wire opcodes, Rust decoders, Go bindings, conformance pages, and checks
# that no reachable non-element value type is unaccepted without a stated reason.
# `status` refreshes docs/STATUS.md; `coverage` verifies docs/STATUS.md and the coverage ratchet.
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

# Rust core builds, release packaging, and cross-platform compilation checks.
# `lib` builds both `ship` (production library) and `diag` (offscreen renderer + scripted input)
# along with the Rust reference binaries (`GK_REFS=1`).
# `lib-release` targets glibc 2.28 via zig for portable Linux distribution binaries.
# `check-windows` type-checks the Rust C ABI for Windows MSVC; `cross` checks Go cross-builds.
lib:
	GK_REFS=1 GK_ENGINE=$(ENGINE) tools/build-lib.sh ship diag

lib-release:
	GK_REFS=1 GK_ENGINE=$(ENGINE) GK_TARGET=x86_64-unknown-linux-gnu.2.28 tools/build-lib.sh ship diag

dist:
	tools/dist.sh

check-windows:
	rustup target add x86_64-pc-windows-msvc
	cargo xwin check --locked --target x86_64-pc-windows-msvc -p gk-capi
	cargo xwin check --locked --target x86_64-pc-windows-msvc -p gk-capi --features diag

cross:
	gates/cross.sh

# Verification suites and single-gate entry points (Linux; require `lib/` from `make lib`).
# `gates` runs the full suite and prints gates/out/summary.md while teeing full logs to $(LOGS).
# `gates-quick` runs the shorter tier (fewer FPS rounds, 1000 Hz only, shorter memory runs).
# Individual targets (`proto`, `docs`, `fps`, `pixels`, `gallery`, `memory`, `misuse`, `counter`,
# `rebuild`) invoke one gate script directly for fast iteration.
gates:
	@mkdir -p $(LOGS)
	gates/all.sh > $(LOGS)/gates.log 2>&1; rc=$$?; cat gates/out/summary.md; echo "log: $(LOGS)/gates.log"; exit $$rc

gates-quick:
	@mkdir -p $(LOGS)
	gates/all.sh quick > $(LOGS)/gates.log 2>&1; rc=$$?; cat gates/out/summary.md; echo "log: $(LOGS)/gates.log"; exit $$rc

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

# Go-focused test suites:
#   go-test:      builds core examples and runs hello, proto, misuse, pixels, and rebuild gates.
#   go-proto-all: runs examples/go/proto, core/tests/go family tests, and gensample together.
#   go-apps:      tests the full Go sample apps and 9 tutorial steps against Rust twins.
#   go-static:    same apps built statically (`GK_STATIC=1`) with section garbage collection.
#   go-fps:       rebuilds Go examples and runs quick panels + clicking counter FPS benchmarks.
go-test:
	gates/go.sh build hello proto misuse pixels rebuild

go-proto-all:
	examples/go/build.sh proto port
	gates/go-proto.sh; a=$$?; core/tests/go/run.sh; b=$$?; gates/go-gensample.sh; c=$$?; [ $$a = 0 ] && [ $$b = 0 ] && [ $$c = 0 ]

go-apps:
	gates/go-apps.sh

go-static:
	GK_STATIC=1 gates/go-apps.sh build notes todo reader expenses tutorial

go-fps:
	gates/go.sh build fps

# Remove compiled Go binaries (`bin/`) and gate run outputs (`gates/out/`).
# Keeps `lib/` and Cargo `target/` so expensive Rust rebuilds are preserved.
clean:
	rm -rf bin gates/out
