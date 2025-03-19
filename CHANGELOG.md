# DocBase CLI Changelog

This file contains the version history and updates for DocBase CLI.

## [Unreleased]

No unreleased changes at this time.

## [0.0.7] - 2025-03-19

### Added
- Debug build script `debug-build.sh` for easier testing and development

### Fixed
- Improved API response handling in `docbase memo create` command
  - Enhanced error response processing to display error types explicitly
  - Added support for different API response formats
- Improved group name to group ID conversion process
  - Added more detailed error messages when group list retrieval fails
  - Enhanced error messages to display available groups when a group name is not found
- Improved API response handling in `docbase memo view` command
  - Added support for different response formats
- Improved group list response handling
  - Added support for both array and object formats
- Translated Japanese comments and messages in `.github/scripts/update-version.js` to English

## [0.0.6] - 2025-03-01

Current version at the time of this changelog update.

## [0.0.5] - 2025-02-15

### Fixed
- Fixed version bump workflow
- Fixed attachment ID type handling

## [0.0.4] - 2025-02-01

### Fixed
- Fixed group list response handling
- Fixed memo view command

## [0.0.3] - 2025-01-15

### Fixed
- Fixed go install issue
- Fixed Makefile homebrew formula

## [0.0.2] - 2025-01-01

### Added
- Added DocBase package
- Added GitHub Actions workflow

## [0.0.1] - 2024-12-15

### Added
- Initial development release

## [0.1.0] - 2023-01-01

### Added
- Initial release
- Memo listing, viewing, creating, editing, deleting, and archiving functionality
- Group listing and viewing functionality
- Tag listing functionality
- Comment listing, creating, editing, and deleting functionality
- Authentication functionality
- Configuration management functionality
- Export/import functionality