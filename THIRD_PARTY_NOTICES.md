# Third-party notices

Simulator AI is licensed under MIT; third-party software retains its original terms.

- `internal/third_party/pdf` is a modified BSD-3-Clause fork of `github.com/ledongthuc/pdf`.
  Its [license](https://github.com/corezoid/simulator-ai/blob/main/internal/third_party/pdf/LICENSE) and [source revision/local changes](https://github.com/corezoid/simulator-ai/blob/main/internal/third_party/pdf/README.md)
  are kept with the source. Review upstream fixes when changing PDF handling.
- Go module versions are pinned in `go.mod` and `go.sum`. Release packaging includes each
  linked dependency module's license and notices, plus the Go toolchain license, under `licenses/`.
- Each archive includes `BUILD_INFO.txt` with its compiler, platform, revision and module
  inventory. The release's `build-info.txt` collects that information for all platforms.

Packaging fails if a linked dependency module has no discoverable license file; a maintainer must
resolve its notice requirements before distributing that build. This inventory is not a
claim that a dependency or the copied PDF fork has no vulnerabilities.
