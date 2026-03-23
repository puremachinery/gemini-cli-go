# Privacy

This project does not include telemetry or analytics collection.

## Data handling

- Prompts and tool results are sent only to configured model/auth providers (for example Gemini APIs) to fulfill user requests.
- `google_web_search` and `web_fetch` make network requests only when explicitly invoked by the model/tool loop.
- Credentials are stored locally in `~/.gemini/`.
- Conversation/session state is stored locally under the configured project and user paths.

## Sensitive environments

- By default, private/local URL fallback for `web_fetch` is disabled.
- Enable it only when you explicitly trust the runtime environment and requested targets.
