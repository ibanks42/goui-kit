# Generator report

Written by `gen/gkgen.py gen`. Counts are functions.

- 7700 functions in 677 rows: 5174 bound, 1850 bridged, 671 skipped, 0 failed, 5 not attempted yet.
- 486 node types (262 elements), 22 builder traits, 121 enums, 60 enums with data, 103 records, 27 Into sets, 66 handler shapes, 3146 style tokens, 244 palette colours, 140 theme colours.
- The Odin sample calls 32507 generated procedures; 143 have an argument the sample cannot make.
- Odin names that collided: 5.

## Compiler rounds

| round | judge | errors | items or types refused | errors outside any item |
|---|---|---|---|---|
| 1 | rust | 1 | 1 | 0 |
| 2 | rust | 3 | 3 | 0 |
| 3 | rust | 1 | 1 | 0 |

## Not attempted yet, by reason

| reason | functions |
|---|---|
| protocol: object (no constructor, global or bound function hands one out) | 2 |
| protocol: data (a closure that returns a value) | 2 |
| protocol: delegates (a trait the app implements: one bridge per trait) | 1 |

## Skipped, by reason

| reason | functions |
|---|---|
| value-helper | 228 |
| builder-getter | 181 |
| rust-only-extension | 177 |
| third-party-type | 45 |
| feature-off | 26 |
| deprecated | 10 |
| builder-mutator | 3 |
| test-only | 1 |

## Refused by a compiler (quarantined)

| what | judge | error |
|---|---|---|
