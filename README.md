# gemini-cli-go

Go-first CLI agent inspired by `google-gemini/gemini-cli`.

> ⚠️ Public preview. Unofficial Go reimplementation with a smaller feature set
> than the upstream Node.js CLI. Expect rough edges and feature gaps while
> parity work continues.

## Status

This repo is not an official Google project. It intentionally ships a smaller,
still-evolving subset of `google-gemini/gemini-cli` while the core loop,
interactive UX, and auth flows are stabilized.

## Why Go

- Native release binaries with no Node.js, npm, or npx requirement.
- Straightforward install and distribution for terminal-first environments.
- Cross-platform builds from a single Go codebase.
- Familiar tooling for Go-centric teams and infrastructure.

## Feature summary

- Interactive prompt with streaming responses (raw output mode).
- Input editing with history, multiline (Ctrl+J), and paste-friendly line continuation.
- Markdown rendering with syntax highlighting when output is a TTY.
- Headless mode via `--prompt` or piped stdin.
- Basic tool execution loop (function calls → execute → return results).
- Built-in tools: `read_file`, `read_many_files`, `write_file`, `replace`,
  `run_shell_command`, `google_web_search`, `web_fetch`.
  - Read tools are enabled and run immediately.
  - Write/edit/shell tools prompt for confirmation before execution.
  - `google_web_search` and `web_fetch` are only available with `GEMINI_API_KEY` auth.
  - `web_fetch` local/private URL fallback is disabled by default and can be enabled with `tools.webFetch.allowPrivate=true`.
- Tool approval modes: `default`, `auto_edit`, `yolo`, `plan`.
  - `yolo` auto-approves all tools (including shell); use with caution.
- `/model` command to show or switch the active model (persisted globally).
- `@path/to/file` references to include file context (respects `.gitignore`).
- `/chat` checkpoints (save/list/resume/delete/share).
- `/memory` command to show/list/refresh/add GEMINI.md context.
- `/auth` command in the interactive UI for sign-in/sign-out.

## Parity snapshot

| Capability | Status here | Notes relative to upstream |
| --- | --- | --- |
| Interactive chat | Supported | Core REPL loop, streaming responses, tool execution, and prompt editing are implemented. |
| Headless prompt mode | Partial | `--prompt` and piped stdin work; structured output flags are not implemented yet. |
| Google OAuth | Supported | Usable today, but the current default client is a temporary public-preview compatibility setup. |
| API key auth | Supported | Stable fallback; `google_web_search` and `web_fetch` currently require API key auth. |
| Built-in tools | Partial | `read_file`, `read_many_files`, `write_file`, `replace`, `run_shell_command`, `google_web_search`, and `web_fetch` are implemented; line-oriented `edit` is not. |
| Slash commands | Partial | `/help`, `/clear`, `/quit`, `/model`, `/chat`, `/resume`, `/memory`, and `/auth` are implemented; `/about`, `/settings`, `/compress`, and `/vim` are not. |
| MCP and extensions | Missing | Not implemented yet. |
| Output format flags | Missing | No `--json` or `--streaming-json` support yet. |
| Themes and Vim mode | Missing | Not implemented yet. |
| Checkpoint restore / rewind | Missing | `/restore` and `/rewind` are not implemented yet. |

## Roadmap / known gaps

- MCP integration and extension support.
- Line-oriented edit tool.
- Output format flags (JSON/streaming JSON).
- UI themes and Vim mode.
- Checkpointing (/restore, /rewind).

## Install

### Release binaries (recommended)

Release archives and `SHA256SUMS` are published on
[`Releases`](https://github.com/puremachinery/gemini-cli-go/releases).

Available assets:

- `gemini-cli-go_<version>_darwin_amd64.tar.gz`
- `gemini-cli-go_<version>_darwin_arm64.tar.gz`
- `gemini-cli-go_<version>_linux_amd64.tar.gz`
- `gemini-cli-go_<version>_linux_arm64.tar.gz`
- `gemini-cli-go_<version>_windows_amd64.zip`

macOS/Linux example:

```bash
VERSION=v0.1.0
ASSET=gemini-cli-go_${VERSION}_darwin_arm64.tar.gz
curl -LO "https://github.com/puremachinery/gemini-cli-go/releases/download/${VERSION}/${ASSET}"
tar -xzf "${ASSET}"
chmod +x gemini-cli
sudo mv gemini-cli /usr/local/bin/
```

Verify the download against the matching release `SHA256SUMS` before moving it
into `PATH`.

Windows:

- Download `gemini-cli-go_<version>_windows_amd64.zip` from the release page.
- Unzip it and place `gemini-cli.exe` somewhere on your `PATH`.

### Install from source

Prerequisite: Go 1.25.0+

Install directly:

```bash
go install github.com/puremachinery/gemini-cli-go/cmd/gemini-cli@latest
```

Build from source:

```bash
go build -o bin/gemini-cli ./cmd/gemini-cli
```

Run:

```bash
./bin/gemini-cli
```

Or install locally:

```bash
go install ./cmd/gemini-cli
```

## Usage

Interactive:

```bash
gemini-cli
```

Headless (non-interactive):

```bash
gemini-cli --prompt "Summarize this repo"
echo "input" | gemini-cli --prompt "Summarize"
```

Positional prompts open the interactive UI when stdin is a TTY:

```bash
gemini-cli "What is this project?"
```

Tool approval modes:

```bash
gemini-cli --approval-mode auto_edit
gemini-cli --yolo
```

Include file context:

```bash
gemini-cli "Summarize @README.md"
```

## Authentication

### Google OAuth (current default)

Current OAuth login behavior mirrors the upstream Node.js CLI client for
compatibility during this public preview. That setup is temporary, unofficial,
and may change or stop working without notice if Google changes or disables the
mirrored client.

If you need a setup you control, override the OAuth client with
`GEMINI_OAUTH_CLIENT_ID` and `GEMINI_OAUTH_CLIENT_SECRET`, or use API key auth
instead.

Run `gemini-cli` and follow the browser flow. Credentials are stored at
`~/.gemini/oauth_creds.json` and reused on subsequent runs. If your org uses
Code Assist, set a project ID:

```bash
export GOOGLE_CLOUD_PROJECT="YOUR_PROJECT_ID"
```

### Gemini API key

Stable fallback auth path. `google_web_search` and `web_fetch` currently require
`GEMINI_API_KEY` auth.

```bash
export GEMINI_API_KEY="YOUR_GEMINI_API_KEY"
```

## Privacy and telemetry

- This CLI does not include telemetry or analytics collection.
- Network requests are limited to model/auth endpoints and user-requested tool operations (for example `google_web_search` and `web_fetch`).
- Credentials are stored locally in `~/.gemini/`.
- See `PRIVACY.md`, `SECURITY.md`, and `SUPPORT.md` for details.

## Configuration

Settings are JSON (comment-tolerant) and loaded in this order:

- System: `/etc/gemini-cli/settings.json` (Linux),
  `/Library/Application Support/GeminiCli/settings.json` (macOS),
  `C:\ProgramData\gemini-cli\settings.json` (Windows)
- Global: `~/.gemini/settings.json`
- Workspace: `<repo>/.gemini/settings.json`

`/model` updates `model.name` in the global settings file.

Tool approval settings:

- `tools.approvalMode`: `default`, `auto_edit`, or `plan`. (Use CLI flags for `yolo`.)
- `experimental.plan`: set to `true` to enable plan mode.
- `tools.requireReadApproval`: optional boolean to require confirmations for `read_file`/`read_many_files`.
- `tools.webFetch.allowPrivate`: optional boolean to allow private/local URL fallback in `web_fetch` (default false).

Model settings:

- `model.maxSessionTurns`: optional integer limit on the number of turns per session.
- `model.maxHistoryMessages`: optional integer limit on stored history messages (0 means unlimited).

Environment variables:

- `GEMINI_API_KEY`: API key for Gemini API auth.
- `GEMINI_API_KEY_AUTH_MECHANISM`: `x-goog-api-key` (default) or `bearer`.
- `GEMINI_OAUTH_CLIENT_ID`: override the OAuth client ID (advanced).
- `GEMINI_OAUTH_CLIENT_SECRET`: override the OAuth client secret (advanced).
- `GOOGLE_CLOUD_PROJECT`: project ID for Code Assist auth.
- `CODE_ASSIST_ENDPOINT`: override Code Assist endpoint.
- `CODE_ASSIST_API_VERSION`: override Code Assist API version.
- `GOOGLE_APPLICATION_CREDENTIALS`: optional credentials file for OAuth refresh.
- `OAUTH_CALLBACK_HOST`: override localhost callback host for OAuth web flow.
- `OAUTH_CALLBACK_PORT`: override localhost callback port for OAuth web flow.
- `NO_BROWSER`: set to disable browser-based login.
- `GEMINI_CLI_SYSTEM_SETTINGS_PATH`: override system settings path.
- `GEMINI_CLI_SYSTEM_DEFAULTS_PATH`: override system defaults path.
- `GEMINI_CLI_LOG_LEVEL`: `debug|info|warn|error` to enable logging.
- `GEMINI_CLI_LOG_FORMAT`: `json` for structured logs.

## Comparison to upstream

The upstream `google-gemini/gemini-cli` is the official Node.js CLI with full
feature coverage (MCP, extensions, advanced UI, output formats, etc.). This Go
port focuses on core agent behavior and minimal UI; expect missing features
while parity work continues.

## License

Apache-2.0
