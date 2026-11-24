# Generator report

Written by `gen/gkgen.py gen`. Counts are functions.

- 7765 functions in 682 rows: 5199 bound, 1881 bridged, 682 skipped, 0 failed, 3 not attempted yet.
- 489 node types (263 elements), 22 builder traits, 122 enums, 60 enums with data, 103 records, 27 Into sets, 68 handler shapes, 3146 style tokens, 244 palette colors, 140 theme colors.
- Names that collided: 5.

## Compiler rounds

| round | judge | errors | items or types refused | errors outside any item |
|---|---|---|---|---|
| 1 | rust | 1 | 1 | 0 |
| 2 | rust | 3 | 3 | 0 |
| 3 | rust | 1 | 1 | 0 |

## Not attempted yet, by reason

| reason | functions |
|---|---|
| protocol: data (a closure that returns a value) | 2 |
| protocol: delegates (a trait the app implements: one bridge per trait) | 1 |

## Skipped, by reason

| reason | functions |
|---|---|
| value-helper | 230 |
| builder-getter | 190 |
| rust-only-extension | 177 |
| third-party-type | 45 |
| feature-off | 26 |
| deprecated | 10 |
| builder-mutator | 3 |
| test-only | 1 |

## Refused by a compiler (quarantined)

| what | judge | error |
|---|---|---|
