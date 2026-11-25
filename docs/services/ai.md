# AI assistant (internal/ai)

The assistant panel in every app and "Ask Adrient" in the
search bar talk to one server-side gateway. The browser never sees a provider key.
Contract for app developers: [docs/ai.md](../ai.md).

**Note:** answer quality depends on the model you connect.

## Pieces

| Piece | Where |
| --- | --- |
| Gateway, providers, quotas, prompt hygiene | `internal/ai` |
| HTTP API `/api/v1/ai/*` (auth wall) | `internal/ai/http.go`, wired in `internal/server/ai_wiring.go` |
| Tables | migration `0240_ai.sql`: `ai_settings`, `ai_providers`, `ai_usage` |
| Who may use it | app control `ai.assistant` (Admin > App controls), plus App access for the app |
| Admin page | Admin > AI assistant (`/admin/ai`, `web/app/src/pages/admin/AISection.tsx`) |
| Panel, tool loop | `web/app/src/lib/ai` |
| Tool sets | `web/app/src/pages/{sheets,docs,slides}/ai/tools.ts` |
| Ask Adrient | `web/app/src/lib/ai/AskAdrient.tsx`, row at the top of the search dropdown |
| Fake model server | `go run ./cmd/aifake -addr 127.0.0.1:8199 [-key K]` |

## Providers

`openai` covers every OpenAI-compatible `/chat/completions` endpoint: OpenAI, Ollama (`http://127.0.0.1:11434/v1`),
llama.cpp server, vLLM, LM Studio and OpenRouter. `azure` uses deployments (`api-key` header, `api-version`).
`anthropic` (Messages API) and `gemini` (`streamGenerateContent`) are native clients. The Anthropic and Gemini
clients are tested only against hand-written SSE in the documented formats, not the real services.

Keys:
- typed in the admin page: sealed with AES-256-GCM using a key derived from `ADRIENT_SECRETS_KEY`, bound to the
  org and provider id. Write-only: the API returns only `has_key` and `key_source` (`stored`/`env`).
- or `key_env`: the name of an environment variable, which must start with `ADRIENT_AI_`, so an admin cannot
  make a provider send another server secret.
- Keys are never logged; provider error bodies are not forwarded, and errors are scrubbed of the key.

Plain `http` base URLs are accepted only for loopback/private hosts. "Local" marks a provider on your own
servers; **Local models only** restricts the org to those.

## Request checks (in order)

1. Signed in; app name valid.
2. Kill switch off; assistant enabled for the app (Admin > AI assistant); `ai.assistant` control true for the
   caller (default: members yes, guests and external users no); caller can open the app (App access).
3. A model resolves: request model (must be an enabled provider's model), else the app's model, else the
   default, else the first enabled provider.
4. Per-user requests per minute (in memory, per process; default 20) and daily token budgets per user and per org
   (from `ai_usage`, UTC day).
5. Document access, for `sheets`, `docs` and `slides` with a `document_id`: the same checks as the collab
   sockets. No read access: refused. Read-only: only `risk: "read"` tools reach the model and it is told the
   document is view-only.
6. Size limits: 80 messages, 200 000 characters, 64 tools, 8 KB per tool schema, 5 attachments (40 000 chars).

## Prompt-injection hygiene

- The fixed rules come first in the system prompt; app hints are labelled lower priority.
- Document selection, attachments, search results and every tool result are wrapped in
  `<untrusted_data>` blocks; data cannot close the block (the tag is escaped).
- Secrets in user text, context, attachments and tool results are replaced before sending (private keys, AWS
  keys, `sk-…`, GitHub/Slack tokens, Google API keys, JWTs, bearer tokens, `password=…`-style pairs).
- The server forwards only tool calls whose name the page declared in the same request, with JSON-object
  arguments; others are dropped and counted in the audit entry. Tool results must answer a tool call in the
  history.
- Tools run in the browser with the user's own session, so the editors' save paths still enforce permissions.
  `destructive` tools need **Allow** in the panel; every write shows **Undo** and the header has
  **Undo all AI changes**.

## Usage log and audit

`ai_usage` stores org, user, app, provider, model, token counts, a cost estimate (provider prices per 1M
tokens) and status. No message text. Audit events (`service=ai`): `chat` (app, model, token counts, tool names,
rejected tool calls, redaction count, source count), `settings.update`, `provider.create|update|delete|test`
(`key_changed` flag, never the key).

## API

| Method | Path | Who |
| --- | --- | --- |
| GET | `/api/v1/ai/status?app=` | everyone |
| POST | `/api/v1/ai/chat` (SSE) | everyone allowed |
| POST | `/api/v1/ai/ask` (SSE) `{question, history?}` | everyone allowed |
| GET/PUT | `/api/v1/ai/admin/settings` | org admin |
| GET/POST | `/api/v1/ai/admin/providers` | org admin |
| PUT/DELETE | `/api/v1/ai/admin/providers/{id}` (`api_key`: omit = keep, `""` = remove) | org admin |
| POST | `/api/v1/ai/admin/providers/{id}/test` | org admin |
| GET | `/api/v1/ai/admin/usage?by=app\|model\|user&days=30` | org admin |
