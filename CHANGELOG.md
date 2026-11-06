# Changelog

## [Unreleased]

- Add MIT licensing, packaged dependency notices and checksum/install verification.
- Support release archive rehearsals without publishing and protected-PR weekly promotions.

### Added
- Required CI for formatting, lint, contracts, tests, race detection and vulnerability/secret scans.
- Reproducible CDU reference checks from committed public input projections.
- Version inspection without credentials: `simulator-ai --version`.
- Verified release artifacts for Linux and macOS, coordinated by sim-deploy.

### Changed
- Rewrite the public README around installation, profiles and examples; move the detailed tool catalog into its own reference.

### Fixed
- Upgrade the Go toolchain to 1.27.3 for GO-2026-6809.
- Resolve static-analysis findings and refuse redirects in the live smoke HTTP client.
