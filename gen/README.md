# Generator (`gen/`)

Python and Go tooling that reads extracted rustdoc JSON from `api/rustdoc/` and generates the Rust decoder layer (`core/src/gen/`), split Go packages (`go/gk/`), generated Go test suites (`core/tests/go/{gensample,gen,app}`), coverage summaries (`api/coverage.json`, `docs/STATUS.md`), and the Go component index (`docs/API-GO.md`).

## Entry point

Run `make gen` (which also regenerates `core/wire/`, runs `gofmt`, runs `gates/conformance.py`, and validates `gates/accept.py --check`), or invoke `gen/gkgen.py` directly:

```bash
python3 gen/gkgen.py gen                   # generate Rust, Go, tests, coverage, and API index
python3 gen/gkgen.py strip gen-json/public # pack raw rustdoc JSON into api/rustdoc/*.json.gz
python3 gen/gkgen.py quarantine rust LOG   # record failing Rust lines in api/quarantine.json
```

## Pipeline stages and data flow

1. **Extract and strip** (`gen/extract.sh`, `gen/gkgen.py strip`, `gen/rdj.py`):
   Extracts rustdoc JSON (format version 61) for `gpui`, `gpui_base`, `gpui_component`, and `gpui_kit`, grafts `#[doc(hidden)]` structs listed in `GRAFTS` (`Editor`), separates doc comments and intra-doc links into `api/rustdoc/docs.json.gz`, drops non-local trait impls (except `From`/`Default`), and writes deterministic `.json.gz` archives.
2. **Model classification** (`gen/gk_model.py`, plus extensions `gk_app.py`, `gk_values.py`, `gk_overlay.py`, `gk_lists.py`, `gk_editor.py`, `gk_items.py`, `gk_comp6.py`):
   Loads `api/rustdoc/`, `api/overrides.toml`, and `api/clone.json`. Classifies every public type, trait, and function into nodes, states, library objects, unit enums, data enums, records, `Into` sets, handler shapes, style tokens, and theme/palette colors. Applies `api/quarantine.json`, prunes unreachable dependencies, and propagates `in`/`out` wire flags. Generation fails if any upstream function hits `norule` (unless `--allow-no-rule`) or if any override entry goes unused.
3. **Wire numbering and Rust emission** (`gen/gk_rust.py`, `gen/gk_fam.py`, `gen/gk_tests.py`):
   Assigns append-only numeric IDs in `api/opcodes.lock` (node types >= 256, inherent ops >= 256, trait ops at `0x4000 + (trait_num << 7) + idx`, call ops >= 256, handler shapes >= 64). Emits `codecs.rs`, `style.rs`, `traits.rs`, `calls.rs`, `n_*.rs`, and `mod.rs`, then passes them through `gk_fam.Families.split()` to partition non-core families into `f_<fam>.rs` modules with `extern "C" fn gk_use_<fam>()` hooks for static dead-stripping. Records line-to-item provenance in `gen/out/rust-lines.json`.
4. **Go emission and package splitting** (`gen/gk_names.py`, `gen/gk_doc.py`, `gen/gk_go.py`, `gen/gk_go_kinds.py`, `gen/gk_go_bridges.py`, `gen/gk_split.py`, `gen/gosplit/`):
   Assigns collision-free stems (`gpui_component` > `gpui_base` > `gpui` > `gpui_kit`), adapts upstream rustdoc comments (logging omitted Rust-only sentences in `gen/out/doc-rust-only.md`), emits a flat `gk` package in memory plus `s/`, `palette/`, and `theme/`, injects `use<Family>()` hooks for static linking (`gen_use.go` / `gen_use_dyn.go`), and runs `gk_split.Split` + `gen/gosplit` to partition declarations across the acyclic package hierarchy (`internal/rt -> gk -> motion -> table -> editor -> base -> kit -> chart -> overlay -> dock`). Verifies that no Go or Rust string literal cites a `gk.<Name>` that moved out of root `gk`.
5. **Generated tests and reports** (`gen/gk_tests_go.py`, `gen/gk_apidoc.py`, `gen/gkgen.py report`):
   Emits `core/tests/go/{gensample/main.go,gen/main.go,app/gen_ambient.go}` and migrates their imports via `gosplit migrate`, refreshes the component index in `docs/API-GO.md`, writes `api/coverage.json`, `api/gk.api.json.gz`, and `gen/out/report.md`, and runs `tools/status.py` to keep `docs/STATUS.md` synchronized.

## Key invariants

- **Append-only wire numbers**: `api/opcodes.lock` numbers are never renumbered or reused once assigned.
- **No silent drops**: every upstream item must end up `bound`, `bridged`, `skipped` (with an explicit reason), `todo` (known gap), or `failed` (quarantined); `norule` stops generation.
- **Unused overrides fail fast**: stale entries in `api/overrides.toml` (`[values]`, `[lists]`, `[editor]`, `[comp6]`, etc.) abort generation so config never rots silently.
- **Static dead-stripping**: every non-core opcode emitted by generated Go code must trigger its family's `use<Family>()` registration hook; missing links panic loudly at runtime with the missing family name.

## Helper scripts and data files

- `gen/extract.sh`: runs `cargo doc` with rustdoc JSON output for public and hidden passes into `gen-json/`.
- `gen/check.sh`: runs `cargo check --release --locked -p gk-capi` and writes `gen/out/build.check.log`.
- `gen/round.sh`: loops `gen/check.sh` and `gkgen.py quarantine rust` to isolate failing items when updating upstream crate pins.
- `api/overrides.toml`: hand-maintained instantiations, skips, runtime shims, and Go naming rules.
- `api/opcodes.lock`: append-only wire opcode registry; never edit or reorder existing numbers by hand.
- `api/clone.json`: `Clone` struct inventory generated by `tools/clone_scan.py`.
- `api/quarantine.json`: items temporarily excluded after compiler errors; stale entries are pruned automatically.
