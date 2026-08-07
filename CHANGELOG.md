# DocBase CLI Changelog

This file contains the version history and updates for DocBase CLI.

## [Unreleased]

### Added

- `docbase memo patch-body <id>` command — line-by-line partial update of memo body via `PATCH /posts/:id/body`. Uses `old_content` as a safety guard against accidental overwrites. `--include-body` returns the updated body in the response.
- `--exclude-body` flag for `docbase memo create` and `docbase memo edit` — omits the body from the API response to reduce bandwidth on large memos.
- `docbase api patch <path>` subcommand — generic PATCH passthrough, consistent with existing `api get/post/put/delete`.

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

## [0.0.6] - 2025-03-18

Current version at the time of this changelog update.

### Added
- Initial release
- Added DocBase package
- Added GitHub Actions workflow
- Memo listing, viewing, creating, editing, deleting, and archiving functionality
- Group listing and viewing functionality
- Tag listing functionality
- Comment listing, creating, editing, and deleting functionality
- Authentication functionality
- Configuration management functionality
- Export/import functionality

### Fixed
- Fixed version bump workflow
- Fixed attachment ID type handling
- Fixed group list response handling
- Fixed memo view command
- Fixed go install issue
- Fixed Makefile homebrew formula