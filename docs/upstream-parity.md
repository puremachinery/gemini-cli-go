# Upstream Parity Matrix

This document compares `gemini-cli-go` against the upstream
[`google-gemini/gemini-cli`](https://github.com/google-gemini/gemini-cli) user
surface.

- Upstream baseline commit: [`9a73aa40724577e49e4391406bcb53810a4ed7c3`](https://github.com/google-gemini/gemini-cli/commit/9a73aa40724577e49e4391406bcb53810a4ed7c3)
- Scope: user-facing capabilities, not internal implementation details
- Status vocabulary:
  - `Supported`: implemented and usable today
  - `Partial`: available with notable limitations or narrower behavior
  - `Missing`: not implemented

For the internal implementation inventory, see
[`docs/feature-status.yaml`](feature-status.yaml).

## Authentication

| Upstream feature | Status here | Notes |
| --- | --- | --- |
| Sign in with Google | Supported | Interactive startup auth prompts and `/auth` sign-in/sign-out are implemented. Current default client setup is a temporary public-preview compatibility arrangement. |
| Gemini API key | Supported | `GEMINI_API_KEY` auth is supported and remains the stable fallback path. |
| Vertex AI auth | Missing | No Vertex/ADC/service-account auth path yet. |

## Built-in tools

| Upstream feature | Status here | Notes |
| --- | --- | --- |
| `read_file` | Supported | Implemented with line offset and limit support. |
| `read_many_files` | Supported | Implemented with glob expansion and `.gitignore` awareness. |
| `write_file` | Supported | Implemented with approval prompts in interactive mode. |
| `replace` | Supported | Exact-match replacement tool is implemented. |
| `run_shell_command` | Supported | Implemented with approval prompts in interactive mode. |
| `google_web_search` | Partial | Implemented, but currently only exposed when using `GEMINI_API_KEY` auth. |
| `web_fetch` | Partial | Implemented, but currently only exposed when using `GEMINI_API_KEY` auth. |
| `save_memory` | Missing | Memory can be managed via `/memory`, but there is no model-callable `save_memory` tool yet. |
| `edit` | Missing | No line-oriented edit tool yet. |
| `write_todos` | Missing | Not implemented. |

## Slash commands

| Upstream feature | Status here | Notes |
| --- | --- | --- |
| `/help` | Supported | Implemented. |
| `/clear` | Supported | Implemented. |
| `/quit` / `/exit` | Supported | Implemented. |
| `/model` | Supported | Implemented, including global persistence of the active model. |
| `/chat` | Supported | Save/list/resume/delete/share conversation checkpoints are implemented. |
| `/resume` | Supported | Implemented for auto-saved sessions. |
| `/memory` | Supported | Show/list/refresh/add `GEMINI.md` context is implemented. |
| `/auth` | Supported | Interactive sign-in/sign-out is implemented. |
| `/about` | Missing | Not implemented. |
| `/settings` | Missing | Not implemented. |
| `/compress` | Missing | Not implemented. |
| `/vim` | Missing | Not implemented. |

## Output modes

| Upstream feature | Status here | Notes |
| --- | --- | --- |
| Interactive text output | Supported | Default REPL flow is implemented. |
| Headless text output | Supported | `--prompt` and piped stdin are supported. |
| Structured JSON output | Missing | No JSON output flag yet. |
| Streaming JSON event output | Missing | No streaming JSON output flag yet. |

## Configuration

| Upstream feature | Status here | Notes |
| --- | --- | --- |
| Hierarchical settings merge | Supported | System, global, and workspace settings are merged. |
| Comment-tolerant JSON settings | Supported | Settings files allow comments via HuJSON parsing. |
| Model aliases | Supported | Alias resolution for `auto`, `pro`, `flash`, and `flash-lite` is implemented. |
| Persisted model selection | Supported | `/model` writes the selected model to global settings. |
| Interactive settings inspection | Missing | No `/settings` command yet. |

## Extensibility

| Upstream feature | Status here | Notes |
| --- | --- | --- |
| MCP | Missing | No MCP integration or top-level `mcp` support yet. |
| Extensions | Missing | Not implemented. |

## Notable differences

- `google_web_search` and `web_fetch` require `GEMINI_API_KEY` auth here.
- The current public-preview Google OAuth defaults are intentionally temporary.
- This matrix is a pinned snapshot, not an automated live mirror of upstream.
