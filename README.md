# DocBase CLI

DocBase CLI is a command-line interface for [DocBase](https://docbase.io/), a knowledge-sharing platform for teams. It allows you to interact with DocBase from the command line, making it easy to automate tasks and integrate with other tools.

## Features

- Authentication and configuration management
- Memo operations (list, view, create, edit, delete, archive, search)
- Group operations (list, view, members)
- Tag operations (list, search)
- Comment operations (list, create, delete)
- Direct API access
- Export memos to local files
- Import memos from local files
- Shell completion for Bash, Zsh, Fish, and PowerShell

## Installation

### Using Homebrew (macOS and Linux)

```bash
brew tap basi/tap
brew install docbase
```

### Using Go

If you're installing from a public repository:

```bash
go install github.com/basi/docbase-cli@latest
```

If you're installing from a private repository, you need to configure Go to skip the public proxy:

```bash
# Set GOPRIVATE environment variable
export GOPRIVATE=github.com/basi/docbase-cli

# Install the CLI
go install github.com/basi/docbase-cli@latest
```

For permanent configuration, add the GOPRIVATE setting to your shell profile (~/.bashrc, ~/.zshrc, etc.):

```bash
echo 'export GOPRIVATE=github.com/basi/docbase-cli' >> ~/.bashrc
source ~/.bashrc
```

### Manual Installation

Download the latest binary from the [releases page](https://github.com/basi/docbase-cli/releases) and place it in your PATH.

## Getting Started

### Authentication

To use DocBase CLI, you need to authenticate with your DocBase team and API token. You can generate an API token from the DocBase settings page: `https://[your-team].docbase.io/settings/tokens`.

```bash
docbase auth login --team your-team --token your-access-token
```

### Configuration

You can view and modify your configuration using the `config` command:

```bash
# List all configuration values
docbase config list

# Set configuration values
docbase config set --team your-team
docbase config set --output-format json
```

## Usage Examples

### Memos

```bash
# List memos
docbase memo list

# Search memos
docbase memo search "keyword"
docbase memo search --tag "週報" --author "john"

# View a memo
docbase memo view 12345

# Create a memo
docbase memo create --title "Test Memo" --body "This is a test memo" --group "全員" --tag "テスト"

# Edit a memo
docbase memo edit 12345 --title "Updated Title"

# Delete a memo
docbase memo delete 12345
```

### Groups

```bash
# List groups
docbase group list

# View a group
docbase group view 123

# List group members
docbase group members 123
```

### Tags

```bash
# List tags
docbase tag list

# Search tags
docbase tag search "weekly"
```

### Comments

```bash
# List comments for a memo
docbase comment list 12345

# Create a comment
docbase comment create 12345 --body "This is a comment"

# Delete a comment
docbase comment delete 12345 67890
```

### Export and Import

```bash
# Export memos from a group
docbase export group "全員" --output ./exports

# Export memos with a tag
docbase export tag "週報" --output ./exports

# Import a memo from a file
docbase import file ./memo.md --group "全員"

# Import memos from a directory
docbase import dir ./exports --group "全員"
```

### Direct API Access

```bash
# Make a GET request
docbase api get /posts?q=tag:weekly

# Make a POST request
docbase api post /posts --data '{"title":"Test","body":"Test body","draft":false,"tags":["test"],"scope":"group","groups":[1],"notice":false}'
```

## Shell Completion

DocBase CLI supports shell completion for Bash, Zsh, Fish, and PowerShell. To enable it, run:

```bash
# Bash
docbase completion bash > /usr/local/etc/bash_completion.d/docbase

# Zsh
docbase completion zsh > "${fpath[1]}/_docbase"

# Fish
docbase completion fish > ~/.config/fish/completions/docbase.fish

# PowerShell
docbase completion powershell > docbase.ps1
```

## Building from Source

```bash
# Clone the repository
git clone https://github.com/basi/docbase-cli.git
cd docbase-cli

# Build
make build

# Install
make install
```

## License

MIT
