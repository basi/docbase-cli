# DocBase CLI

> [!WARNING]
> **Experimental**: This CLI is still in development and may have breaking changes.

DocBase CLI is a command-line interface for [DocBase](https://docbase.io/), a knowledge-sharing platform for teams. It allows you to interact with DocBase from the command line, making it easy to automate tasks and integrate with other tools.

## Features

- Authentication and configuration management
- Memo operations (list, view, create, edit, delete, archive, search)
- Group operations (list, view, members)
- Tag operations (list, search)
- Comment operations (list, create, delete)
- Direct API access
- A token broker that lets an AI agent use DocBase read-only without being able to read the token (Linux)
- Export memos to local files
- Import memos from local files
- Shell completion for Bash, Zsh, Fish, and PowerShell

## Changelog

For a detailed list of changes and version history, please see the [CHANGELOG.md](CHANGELOG.md) file.

## API Spec

For details on the API specification, please refer to the following documentation:
[DocBase API Spec](https://help.docbase.io/posts/45703)

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

The token is stored per team domain in your operating system's credential store (macOS Keychain, Windows Credential Manager, or Secret Service on Linux), not in the config file. To keep the token out of your shell history, pass it through the environment instead of `--token`:

```bash
DOCBASE_API_TOKEN=your-access-token docbase auth login --team your-team
```

The access token is resolved in this order: the `--token` flag, the `DOCBASE_API_TOKEN` environment variable, then the stored token.

#### Upgrading from earlier versions

Earlier versions stored the token in plaintext in `~/.config/docbase/config.json`. It is moved to the credential store automatically the first time a command needs it. If the credential store is unavailable, the plaintext token keeps working and a warning is printed to stderr.

The `ACCESS_TOKEN` environment variable is no longer read; use `DOCBASE_API_TOKEN` instead.

#### CI and GitHub Actions

In CI, do not run `docbase auth login`; nothing needs to be stored. Pass the token only to the steps that need it through `DOCBASE_API_TOKEN`:

```yaml
- name: Fetch a DocBase memo
  env:
    DOCBASE_API_TOKEN: ${{ secrets.DOCBASE_API_TOKEN }}
  run: docbase --team your-team memo view 12345 > memo.md
```

Use a dedicated token with read-only scope and an expiry. If the fetched content is given to an AI agent, enable the token's AI-use option so DocBase excludes groups and `no-ai` memos that are not allowed for AI.

Do not reference the DocBase secret in a job that runs an AI agent: a runner keeps every secret its job references in memory, and on hosted runners the agent can become root. Fetch in a separate job and pass the result to the agent job as an artifact.

### Using DocBase from an AI agent without exposing the token (`docbase broker`)

`docbase broker` lets an AI agent run read-only `docbase` commands without being able to read the access token. A trusted step starts a broker process that holds the token in memory and serves read-only DocBase requests for one team on a Unix socket. The agent's environment contains only the socket path, in `DOCBASE_BROKER_SOCKET`. While that variable is set, `docbase` sends its requests to the broker and never looks for a token itself. The broker runs on Linux only.

```yaml
- name: Start DocBase broker
  env:
    DOCBASE_API_TOKEN: ${{ secrets.DOCBASE_API_TOKEN }}
  run: docbase broker start --team your-team --ttl 30m >> "$GITHUB_ENV"
# ... drop sudo/containers, then run the agent; it sees only DOCBASE_BROKER_SOCKET
- name: Stop DocBase broker
  if: always()
  run: docbase broker stop || true
```

`docbase broker start` prints `DOCBASE_BROKER_SOCKET=<path>` as the only line on stdout (messages go to stderr), so the line can be appended to `$GITHUB_ENV`.

- **The agent must not be able to become root.** The broker protects the token from other processes of the same user that are not root: the token never appears in the arguments, the environment or a file of the broker, the broker process cannot be dumped, so `/proc/<pid>/environ`, `/proc/<pid>/mem` and core files are closed to other non-root processes, and only the user that started it can use its socket. Root can read the memory of any process. Unlike the separate-job pattern above, the secret is referenced in the job that runs the agent, so remove `sudo` and container access before the agent step, for example with a runner-hardening action.
- **Use a read-only, AI-use token.** Create a dedicated token with read-only scope and an expiry, and enable its AI-use option so that DocBase excludes groups and `no-ai` memos that are not allowed for AI.
- **Start the broker before anything untrusted runs.** The broker keeps the token from processes that start after it reported ready. A process of the same user that is already running while `docbase broker start` runs can take the token while it is handed over: it can read the standard input of the new broker process before that process has hardened itself, or answer in its place on the status pipe. Such a process can equally read the environment of the start step or the credential store the token came from, so this is inherent and not something the broker can close. Start the broker before the agent, and on a machine where other programs of the same user are running (a shared desktop, for example) assume that those programs can read the token anyway.
- **The broker protects only the token it holds.** A token saved with `docbase auth login` in the operating system's credential store, or exported in the environment of the agent, stays readable by the agent. Pass the token to the step that starts the broker only, as in the example above.
- **Read-only.** The broker forwards `GET` requests for the team it was started for and nothing else. It answers other methods, other teams, other paths and unknown query parameters with an error. The allowed endpoints are the ones the read commands use:
  - `/teams/{team}/posts` (`q`, `page`, `per_page`)
  - `/teams/{team}/posts/{id}`
  - `/teams/{team}/posts/{id}/comments` (`page`, `per_page`)
  - `/teams/{team}/groups` (`page`, `per_page`)
  - `/teams/{team}/groups/{id}`
  - `/teams/{team}/tags` (`q`, `page`, `per_page`)

  So `memo list|search|view`, `comment list`, `group list|view|members`, `tag list|search`, `export` and `api get` of these paths work; commands that write get an error. `page` is 1 to 10000 and `per_page` is 1 to 100.
- **TTL and stop.** The broker stops by itself after `--ttl` (default 1 hour, from 1 minute to 12 hours) and removes its socket. `docbase broker stop` stops it earlier and `docbase broker status` shows its team and expiry time. Both read `DOCBASE_BROKER_SOCKET`, or `--socket`.
- **Broker mode.** `--token` and `DOCBASE_API_TOKEN` cannot be combined with `DOCBASE_BROKER_SOCKET`, `--team` must name the broker's team, and `docbase auth login`, `docbase auth logout` and `docbase config set --token` are refused. Broker mode is never left by accident: a `DOCBASE_BROKER_SOCKET` that is set but empty is an error, not a way back to direct access, and an unreachable broker is an error too. In broker mode `docbase` needs neither a home directory nor a config file. The broker, and the token check that `docbase broker start` makes before it starts one, ignore the proxy settings of the environment, do not follow redirects, and talk to the official DocBase API only.

### Configuration

You can view and modify your configuration using the `config` command:

```bash
# List all configuration values
docbase config list

# Set configuration values
docbase config set --team your-team
docbase config set --output-format json
```

#### Environment variables

| Variable | Description |
| --- | --- |
| `DOCBASE_API_TOKEN` | Access token to use without saving it. Overrides the stored token, but not `--token`. |
| `DOCBASE_BROKER_SOCKET` | Socket of a running `docbase broker`. While it is set, requests go through the broker, which holds the token; see [above](#using-docbase-from-an-ai-agent-without-exposing-the-token-docbase-broker). |
| `DOCBASE_INSECURE_STORAGE` | Last resort for long-lived machines without a credential store: set to `1` to store the token in plaintext in the config file (mode 0600) instead of the credential store. In CI, use `DOCBASE_API_TOKEN` instead. |

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

# Overwrite tags/groups from file (default is to merge)
docbase import file ./memo.md --group "全員" --overwrite

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

Make sure that `$(go env GOPATH)/bin` (usually `~/go/bin`) is in your `PATH`.

```bash
export PATH="$HOME/go/bin:$PATH"
```

## License

MIT
