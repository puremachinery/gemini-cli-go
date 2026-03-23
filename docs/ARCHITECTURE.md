# Architecture

Goal: a Go-first CLI with a UI/UX aligned to the upstream gemini-cli, while keeping a clean separation between CLI, core runtime, and UI.

## Upstream mapping

- CLI entry + lifecycle: packages/cli/index.ts -> cmd/gemini-cli
- CLI config/auth/policy: packages/cli/src/config -> internal/config, internal/auth, internal/storage
- CLI commands/services: packages/cli/src/commands + services -> internal/cli (router) and future internal/commands
- UI (Ink TUI): packages/cli/src/ui -> internal/ui (minimal TUI for MVP)
- Core runtime (client, prompts, tools, agents): packages/core/src -> internal/client, internal/llm, internal/session, internal/tools (later)

## Settings and paths

- Settings format: JSON (comment-tolerant read, format-preserving write later).
- Global settings: ~/.gemini/settings.json
- Workspace settings: <workspace>/.gemini/settings.json
- System settings: /etc/gemini-cli/settings.json (Linux), /Library/Application Support/GeminiCli/settings.json (macOS), C:\ProgramData\gemini-cli\settings.json (Windows)

Path helpers live in internal/storage/paths.go.

## MVP command flow (interactive)

1. Parse args
2. Load settings (system -> global -> workspace)
3. If GEMINI_API_KEY is set, initialize Gemini API client; otherwise ensure Google login credentials
4. Initialize Gemini client (API key or OAuth-backed)
5. Run minimal TUI loop (streaming output)

## MVP command flow (headless)

1. Parse args/stdin for prompt
2. Load settings and resolve model
3. Initialize Gemini client (API key or OAuth-backed)
4. Run single streaming prompt (non-interactive)

## Core interfaces (current skeleton)

- internal/auth
  - Provider: Login/Refresh for Google OAuth
  - Store: Load/Save/Clear credential storage
- internal/client
  - Client: ChatStream for streaming responses
- internal/llm
  - Shared chat message/stream types
- internal/session
  - Session and Store for history/resume
- internal/config
  - Settings as open-ended JSON with dotted-path access
  - Store interface for load/save
- internal/cli
  - Command tree abstraction (router to be added)

## Notes

- MVP supports Google OAuth and Gemini API key auth; Vertex auth follows.
- TUI should resemble upstream behavior early to avoid rework.
