# ghapp — GitHub App Auth for git/gh

CLI tool that authenticates as a GitHub App, generates installation tokens, and configures `git` and `gh` to use them transparently.

## Install

```bash
# Homebrew (macOS/Linux)
brew tap operator-kit/tap
brew install ghapp

# One-liner (Linux/macOS)
curl -sSL https://raw.githubusercontent.com/operator-kit/ghapp-cli/main/install.sh | bash

# PowerShell (Windows)
irm https://raw.githubusercontent.com/operator-kit/ghapp-cli/main/install.ps1 | iex

# Specific version
curl -sSL https://raw.githubusercontent.com/operator-kit/ghapp-cli/main/install.sh | GHAPP_VERSION=v0.1.0 bash

# From source (requires Go)
go install github.com/operator-kit/ghapp-cli/cmd/ghapp@latest

# Build from cloned repo
go build -o build/ ./cmd/ghapp/
go build -o build/ ./cmd/gh-wrapper/   # optional: gh wrapper binary

# Cross-compile
GOOS=linux GOARCH=arm64 go build -o build/ ./cmd/ghapp/
```

## Prerequisite — Create a GitHub App

You need three values for `ghapp setup`: **App ID**, **Installation ID**, and a **private key** (.pem file).

### 1. Create the app
- Go to **Settings → Developer settings → GitHub Apps → New GitHub App** ([docs](https://docs.github.com/en/apps/creating-github-apps/registering-a-github-app/registering-a-github-app))
  - For org-owned apps: **Org settings → Developer settings → GitHub Apps**
- **Name**: anything unique (e.g., `Mr Fox`) - this will be used everywhere your App interacts.
- **Homepage URL**: can be any URL (e.g., your org's GitHub page)
- **Webhooks**: uncheck **Active** (not needed)
The other settings are not needed.

### 2. Set permissions

Select **Repository permissions** based on what you need:

| Use case | Required permissions |
|----------|---------------------|
| `git` clone/push/pull | **Contents**: Read & write |
| `gh` PRs, issues, etc. | **Contents**: Read & write, **Pull requests**: Read & write, **Issues**: Read & write, **Metadata**: Read |

> Add more as needed. `gh pr list` silently returns empty without Issues read permission.

### 3. Create & note your App ID
- Click **Create GitHub App**
- **App ID** is shown at the top of the app's settings page

### 4. Generate a private key
- On the app settings page, scroll to **Private keys → Generate a private key**
- A `.pem` file downloads — store it securely (GitHub won't show it again)

### 5. Install the app & note your Installation ID
- On the app settings page, click **Install App** in the left sidebar
- Select your account/org and choose which repos to grant access to
- After installing, the URL will be `github.com/settings/installations/12345678` — the number at the end is your **Installation ID**

## Quick Start

```bash
# 1. Setup — enter App ID, Installation ID, key path
#    (optionally configures git + gh auth at the end)
ghapp setup

# 2. Use git/gh normally — auth is transparent
git clone https://github.com/org/repo.git
gh pr list
```

> If you skipped auth configuration during setup, run `ghapp auth configure` separately.

## Commands

| Command | Description |
|---------|-------------|
| `ghapp setup [--import-key]` | Interactive setup — App ID, Installation ID, PEM key |
| `ghapp token [--no-cache]` | Print an installation token (cached; `--no-cache` forces fresh) |
| `ghapp auth configure [--gh-auth MODE]` | Configure git credential helper, gh CLI, and git identity |
| `ghapp auth status` | Show current auth configuration and diagnostics |
| `ghapp auth reset [--remove-key]` | Remove all auth config and restore previous git identity |
| `ghapp update` | Self-update to the latest release |
| `ghapp version` | Print version info |

### `--gh-auth` modes

During `auth configure`, you're prompted to choose how `gh` CLI gets authenticated. You can also pass it non-interactively:

| Mode | Flag value | Description |
|------|-----------|-------------|
| Shell function | `--gh-auth shell-function` | Wraps `gh` with a shell function that injects a fresh token per invocation |
| PATH binary | `--gh-auth path-shim` | Installs `ghapp-gh` wrapper binary as `gh` earlier in PATH |
| None | `--gh-auth none` | Only writes `hosts.yml` (token expires in ~1hr) |

## How It Works

### git auth

`ghapp` registers itself as a git credential helper. On every git network operation, git calls `ghapp credential-helper get`, which returns a fresh installation token. Tokens are cached locally so repeated operations within the same session are fast.

`auth configure` also sets `url."https://github.com/".insteadOf "git@github.com:"` so that SSH-style URLs (`git@github.com:org/repo.git`) are transparently rewritten to HTTPS. This means copy-pasted SSH clone URLs and submodules that reference `git@github.com:...` will work automatically.

### git identity

`auth configure` sets `user.name` and `user.email` to the app's bot account (e.g., `myapp[bot]`), so commits are attributed to the app with its icon on GitHub. If you already have a git identity, it will ask before overwriting and backs up your previous identity for `auth reset`.

### gh auth — shell function (recommended)

`auth configure` injects a managed block into your shell's rc file that wraps `gh` with a function. Every `gh` invocation automatically gets a fresh token via `GH_TOKEN`:

```bash
# What gets added to your .bashrc / .zshrc (managed automatically):
eval "$(ghapp auth shell-init)"
```

Under the hood, this defines a `gh()` function that calls `ghapp token`, sets `GH_TOKEN`, and delegates to the real `gh`. Tokens are cached so the overhead is negligible after the first call.

**Supported shells:** bash, zsh, fish, PowerShell

### gh auth — PATH binary (CI / non-shell)

For environments without shell rc files (CI, containers, cron), the `ghapp-gh` wrapper binary can be placed on PATH as `gh`. It resolves the real `gh`, generates/caches a token, and execs with `GH_TOKEN` set. Falls through to plain `gh` if config is missing.

### Token caching

All token paths (credential helper, `ghapp token`, shell function, wrapper binary) share a local cache file. Tokens are reused until they're within 5 minutes of expiry, then automatically refreshed. This means:

- `ghapp token` returns in <10ms on cache hit
- Back-to-back `git` operations don't re-generate tokens
- The shell function adds negligible latency to `gh` commands

## Config

Stored at `~/.config/ghapp/config.yaml`:

```yaml
app_id: 123456
installation_id: 789012
private_key_path: /path/to/key.pem
key_in_keyring: false
app_slug: myapp              # cached after first auth configure
bot_user_id: 149130343       # cached after first auth configure
```

Environment overrides: `GHAPP_APP_ID`, `GHAPP_INSTALLATION_ID`, `GHAPP_PRIVATE_KEY_PATH`, `GHAPP_NO_UPDATE_CHECK=1` (disable daily update notice)

## Private Key Storage

- **File** (default): path stored in config, key stays on disk
- **OS Keyring** (`--import-key`): key imported into Windows Credential Manager / macOS Keychain / Linux Secret Service
