# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.7.1] - 2025-10-28
### Added
- Fix sorting order of results. Now sorting by `Order` field in ascending order, instead of `PathId`.

## [0.7.0] - 2025-10-24
## Changed
- Update qdrant's docker compose file name


## [0.5.0] - 2025-10-08
## Added
- Recursive scanning with early stopping based on a configurable threshold.
- New settings to control recursion and minimum accepted match score.
- Deduplicated results across folders with improved per-component selection.

## Changed
- Search/scoring switched to a distance-based model and removed fixed top‑K behavior.

## Removed
- Remove dockerfile and docker installation instructions


## [0.4.2] -
### Added
- TLS support for secure connections ([#15](https://github.com/scanoss/folder-hashing-api/pull/15))
- TLS setup scripts and documentation
- Docker healthcheck script

### Changed
- Improved query performance with score thresholds and filters ([#16](https://github.com/scanoss/folder-hashing-api/pull/16))
- Added score conversion function for better query accuracy
- Updated papi version with TLS support
- Enhanced Docker deployment scripts
- Updated certificate default file names


## [0.2.0] - 2025-01-07
### Added
- Initial Folder Hashing Service API Release

[0.2.0]: https://github.com/scanoss/folder-hashing-api/compare/v0.0.0...v0.2.0
[0.4.2]: https://github.com/scanoss/folder-hashing-api/compare/v0.2.0...v0.4.2
[0.5.0]: https://github.com/scanoss/folder-hashing-api/compare/v0.4.2...v0.5.0
[0.6.0]: https://github.com/scanoss/folder-hashing-api/compare/v0.5.0...v0.6.0
[0.7.0]: https://github.com/scanoss/folder-hashing-api/compare/v0.6.0...v0.7.0
[0.7.1]: https://github.com/scanoss/folder-hashing-api/compare/v0.7.0...v0.7.1
