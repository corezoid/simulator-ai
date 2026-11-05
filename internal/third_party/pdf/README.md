# pdf (fork)

Fork of [github.com/ledongthuc/pdf](https://github.com/ledongthuc/pdf) at
v0.0.0-20261020125853-5e46805f718e (BSD-3-Clause, see LICENSE), the non-test sources only, kept
in upstream's layout (so `cmd/check-source` does not apply its module-size rule here).

simulator-ai changes, each marked `simulator-ai` in the code:

- `limits.go`: `Limits` (bytes and deadline) and `NewReaderLimited`; every stream read counts the
  output of each filter layer against one budget per file and checks the deadline, panicking with
  `ErrLimit` past either.
- `read.go`: the reader keeps its limits; xref stream `Size`, `W` widths, object numbers and the
  PNG predictor's `Columns`, which size allocations, are bounded.
- `page.go`: a page read stopped by a limit reports `ErrLimit` itself, not its text.
