# Changelog

## [Unreleased]

### Added
- Required CI for formatting, lint, contracts, tests, race detection and vulnerability/secret scans.
- Reproducible CDU reference checks from committed public input projections.
- Version inspection without credentials: `simulator-ai --version`.
- Verified release artifacts for Linux and macOS, coordinated by sim-deploy.

### Fixed
- Upgrade the Go toolchain to 1.27.3 for GO-2026-6809.
- Resolve static-analysis findings and refuse redirects in the live smoke HTTP client.
