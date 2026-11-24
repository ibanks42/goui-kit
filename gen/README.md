# gen: the generator

Reads the extracted API of the pinned kit crates and writes the generated half of the
binding: Rust decoders and call ops, the Go package, the coverage table. Python 3 standard
library only; app developers never run it. Design: [../docs/DESIGN.md](../docs/DESIGN.md)
section 13. What generated code calls: [../docs/PROTOCOLS.md](../docs/PROTOCOLS.md).

## Commands

| command | needs | what |
|---|---|---|
| `make gen` (`python3 gen/gkgen.py gen`, then `gates/conformance.py`) | Python only, 2 s | classify every public function, apply `api/quarantine.json`, number from `api/opcodes.lock`, write the Rust decoders (`core/src/gen/`), the Go package (`go/gk/gen_*.go`), the generated tests (`core/tests/go/gensample`, `gen`, `app`), the opcode tables and the interface hash (`core/wire/gen.py`), `api/gk.api.json.gz`, `api/coverage.json`, `gen/out/report.md`, the coverage block of `docs/STATUS.md`, and the gallery's conformance pages. Exits 1 if a signature meets no rule (`gen/out/no-rule.md`) |
| `make lib` | Rust | rustc as judge: the release build of the library with the generated decoders (a few minutes from cold, well under a minute after a generator change) |
| `gen/round.sh [N]` | Rust | the quarantine loop: `cargo check --release -p gk-capi`, errors charged to items by line, regenerate, up to N rounds |
| `make gates-quick` | the library | the Go compiler as judge (every example and the sample by generation build), and the gallery compares every page with Rust |
| `gen/extract.sh`, then `python3 gen/gkgen.py strip DIR` | Rust, then Python | a new upstream pin: rustdoc JSON (about a minute and 1.5 GB of `target/`), reduced to `api/rustdoc/*.json.gz` (3.3 MB, committed), and the doc comments to `api/rustdoc/docs.json.gz` |

## Files

| file | what |
|---|---|
| `gk_model.py` | the rules: one wire descriptor per parameter type, one status per function |
| `gk_rust.py` | the Rust emitter and `api/opcodes.lock` |
| `gk_names.py` | the stem of every name the package declares, before the Go emitter spells it |
| `gk_go.py` | the Go emitter |
| `gkgen.py` | the command line, quarantine, the sample, the reports, the families of what remains |
| `gk_values.py` | the rules for values: generic objects (`[values] objects`), results, arrays, `Rc` objects, the theme's fields, kit action values, event closures (PROTOCOLS.md 13.8 to 13.12); `gates/accept.py` counts the values nothing accepts |
| `rdj.py` | rustdoc JSON reader (format 61) |
| `out/report.md` | counts by reason, compiler rounds, what was refused and why |

Inputs under `../api/`: `rustdoc/` (the extract), `overrides.toml` (the only hand-edited
input), `opcodes.lock` (append-only), `quarantine.json` (what a compiler refused).

Two tables of `overrides.toml` belong to the delegate families (PROTOCOLS.md section 9):
`[instantiate] states` (`"Generic<Arg> as Name"`: a generic entity state instantiated as one
state type; every method of an impl that holds for `Arg` is classified as a state method of
`Name`, and the coverage row of the generic counts a function once, bridged when one
instantiation binds it) and `[shims]` (`"Trait::method" = "where"`: a trait method a
hand-written shim implements, counted as bridged).

## How a function gets its status

- **bound**: a rule generated it and the compilers accepted it; nothing but the tape is
  involved (constructors, builders, style tokens, colours).
- **bridged**: generated against a protocol of the runtime (a handler, a fragment or build
  closure, a state behind a handle, an action). Handlers and state run; actions are not
  built in the runtime yet (docs/STATUS.md, "Runtime protocols").
- **skipped**: left out on purpose, with one of the reasons in `SKIP_REASONS`.
- **failed**: generated and refused by a compiler; the error is in `out/report.md`.
- **not yet**: no rule fits; the reason names the missing protocol or rule.

To add a rule: teach `Model.desc` the type (it returns a descriptor), teach `Rust.dec` and
`Go.gtype` / `Go.put` / `Go.rd` the descriptor, run `make gen`, then `gen/round.sh` and
`make gates-quick`.
