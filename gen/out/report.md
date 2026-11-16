# Generator report

Written by `gen/gkgen.py gen`. Counts are functions.

- 7637 functions in 674 rows: 5134 bound, 1839 bridged, 664 skipped, 0 failed, 0 not attempted yet.
- 484 node types (262 elements), 21 builder traits, 119 enums, 60 enums with data, 102 records, 27 Into sets, 66 handler shapes, 3146 style tokens, 244 palette colours, 139 theme colours.
- The Odin sample calls 29046 generated procedures; 142 have an argument the sample cannot make.
- Odin names that collided: 3.

## Compiler rounds

| round | judge | errors | items or types refused | errors outside any item |
|---|---|---|---|---|
| 1 | rust | 1 | 1 | 0 |
| 2 | rust | 3 | 3 | 0 |
| 3 | rust | 1 | 1 | 0 |

## Not attempted yet, by reason

| reason | functions |
|---|---|

## Skipped, by reason

| reason | functions |
|---|---|
| value-helper | 222 |
| builder-getter | 180 |
| rust-only-extension | 177 |
| third-party-type | 45 |
| feature-off | 26 |
| deprecated | 10 |
| builder-mutator | 3 |
| test-only | 1 |

## Refused by a compiler (quarantined)

| what | judge | error |
|---|---|---|
