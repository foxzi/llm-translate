# API server

`llm-translate --api` starts an HTTP server that translates text and runs the
analyses enabled in the config, all in one request.

## Running

```bash
llm-translate --api -c config.yaml                     # listens on :8080
llm-translate --api --listen 127.0.0.1:9000 -c config.yaml
```

Every CLI flag except the input/output ones also applies in API mode:
`--provider`, `--model`, `--style`, `--glossary`, `--sentiment`, `--tags`,
`--parallel-analysis` and so on. They act as server-wide defaults.

## Docker

```bash
cp config.example.yaml config.yaml   # set provider, model and analyses
docker compose up -d --build
```

`docker-compose.yml` mounts `./config.yaml` read-only to `/config.yaml`, publishes
port 8080 and restarts the container (`unless-stopped`). To pass extra flags, override `command`:

```yaml
command: ["--api", "-c", "/config.yaml", "--parallel-analysis", "--tags", "5"]
```

API keys for cloud providers can be passed through environment variables
(`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, ...) under `environment:`.

## Authorization

Set a token with `--api-token` or `settings.api_token` in the config (the flag
wins). Then every `POST /v1/translate` must carry it:

```bash
llm-translate --api -c config.yaml --api-token s3cret
curl -s -X POST localhost:8080/v1/translate \
  -H 'Authorization: Bearer s3cret' \
  -d '{"text":"Hello","from":"en","to":"ru"}'
```

The header must be exactly `Authorization: Bearer <token>` (the `Bearer ` prefix
is case-sensitive). A missing or wrong token returns 401 `{"error":"unauthorized","code":"unauthorized"}`.
`GET /health` stays open. Without a token the server accepts any request and
logs a warning at start.

## Endpoints

### `GET /health`

```json
{"status":"ok","version":"dev"}
```

### `POST /v1/translate`

All fields except `text` are optional. A client that sends only
`{text, from, to}` gets the same response as before; the response only gains
extra keys (for example `provider`, `model` and `cost` in `usage`).

Request:

| Field       | Required | Description |
|-------------|----------|-------------|
| `text`      | yes      | Text to translate (or to analyze, see `translate`) |
| `from`      | no       | Source language; defaults to `--from` (`auto`) |
| `to`        | no       | Target language; defaults to `default_target_language` (`--to` replaces it when set explicitly, `en` if neither is set) |
| `title`     | no       | Title. Translated together with the text, returned in `title` and analyzed together with the text |
| `translate` | no       | Boolean, default `true`. With `false` the text and the title are not translated and are returned as is; only the analyses run |
| `analysis`  | no       | List of analysis names. Replaces the analyses enabled in the config; an empty list `[]` disables all of them |
| `tags`      | no       | Number of tags to extract; overrides `tags_count`. `0` disables tags |
| `summary`   | no       | Summary length in sentences; overrides `settings.summary`. `0` disables the summary |

Allowed `analysis` names: `sentiment`, `tags`, `classify`, `emotions`,
`factuality`, `impact`, `sensationalism`, `entities`, `events`, `usefulness`,
`time_focus`, `ad_detect`. Names are case-insensitive; an unknown name is a
400 error.

- If `analysis` contains `tags` and `tags` is not given, the number of tags is
  `settings.tags_count`, or 5 when that is not set.
- `tags` applied on top of `analysis`: a value above 0 enables tags and sets
  their count, `0` disables them.
- Without `analysis`, the analyses from `settings` (and CLI flags) are used.
- Negative `tags` or `summary` is a 400 error.
- With `translate: false` (or when `skip_same_language` applies) the analysis
  works on the original text, and free-text values are requested in the `to`
  language.

```bash
curl -s -X POST localhost:8080/v1/translate -d '{
  "text": "Tesla sold a record number of cars in the third quarter.",
  "title": "Tesla record sales",
  "from": "en",
  "to": "de",
  "analysis": ["sentiment", "tags", "entities"],
  "tags": 3,
  "summary": 1
}'
```

Response:

| Field               | Description |
|---------------------|-------------|
| `text`              | Translated text (the original with `translate: false`) |
| `title`             | Translated title; present only if `title` was sent |
| `summary`           | Summary; present only if requested and produced |
| `detected_language`  | Language of the original text, see below |
| `analysis`          | Results of the analyses; omitted when there are none |
| `analysis_errors`   | Names of the analyses (or `summary`) that failed or returned no result |
| `usage`             | Token usage, always present |

`usage`:

| Field               | Description |
|---------------------|-------------|
| `prompt_tokens`     | Prompt tokens of all LLM requests of this call |
| `completion_tokens` | Completion tokens of all LLM requests of this call |
| `provider`          | Provider used |
| `model`             | Model used |
| `cost`              | Only if the provider returns `usage.cost` (for example OpenRouter), summed over all requests; in credits, which equal USD |

Tokens are counted from the `usage` object of the provider responses; if the
provider does not return it, they are 0.

`detected_language` is set only when all of this holds:

- the original text is analyzed: `translate` is `false`, or
  `parallel_analysis` is on (otherwise the translation is analyzed);
- `from` is empty or `auto`;
- the combined analysis runs (2 or more analyses, or a summary), and the
  model returned the language.

```json
{
  "text": "Tesla hat im dritten Quartal eine Rekordzahl an Autos verkauft.",
  "title": "Rekordabsatz bei Tesla",
  "summary": "Tesla hat im dritten Quartal so viele Autos verkauft wie nie zuvor.",
  "analysis": {
    "sentiment": "positive",
    "sentiment_score": 0.7,
    "tags": ["tesla", "absatz", "rekord"],
    "organizations": ["Tesla"]
  },
  "usage": {
    "prompt_tokens": 812,
    "completion_tokens": 240,
    "provider": "openrouter",
    "model": "anthropic/claude-3.5-sonnet",
    "cost": 0.0031
  }
}
```

The `summary` is a separate field and is never part of `analysis`. With a
summary or 2+ analyses the server uses one combined LLM call; a summary exists
only there. If that call fails, the server falls back to individual calls and
reports `summary` in `analysis_errors`.

### Errors

Every error is a JSON object with the keys `error` (message), `code` and, for
`rate_limited`, `retry_after` (seconds, only when known).

| Status | `code`            | When |
|--------|-------------------|------|
| 400    | `invalid_request` | Invalid JSON or body above 10 MiB, empty or whitespace-only `text`, `text` longer than `max_text_length`, unknown `analysis` name, negative `tags` or `summary` |
| 401    | `unauthorized`    | Missing or wrong token |
| 429    | `rate_limited`    | The provider answered 429. The `Retry-After` header is set when the provider sent it |
| 504    | `timeout`         | `request_timeout` expired (also while waiting for a `max_concurrent` slot) |
| 502    | `provider_error`  | Any other failure of the LLM provider |

```json
{"error":"provider returned 429","code":"rate_limited","retry_after":30}
```

```json
{"error":"text is too long: 12034 characters, limit 10000","code":"invalid_request"}
```

Other paths and methods are answered by the Go HTTP server with a plain-text
404 or 405 (for example `GET /v1/translate` is 405), not with a JSON error.

## Settings

All settings below are in `settings`, are off by default (`0` / `false`) and
have no CLI flag, except `summary` (`--summary`).

| Setting              | Description |
|----------------------|-------------|
| `summary`            | Summary length in sentences; `0` disables it. Per request: `summary` |
| `max_concurrent`     | Number of requests processed at once; the others wait for a slot |
| `request_timeout`    | Time limit of one request in seconds, including the wait for a slot; on expiry the answer is 504 `timeout` |
| `max_text_length`    | Maximum length of `text` in characters; longer text is a 400 error |
| `skip_same_language` | If `from` equals `to` (case-insensitive), the text is not translated, as with `translate: false` |
| `strict_analysis`    | Drops analysis values outside the allowed sets, see below |

`strict_analysis` cleans the `analysis` object:

- `sentiment`, `factuality`, `sensationalism`, `time_focus` and `ad_type` must
  be one of the allowed values; otherwise the field is removed together with
  its confidence (`sentiment_score`, `factuality_confidence`,
  `sensationalism_confidence`, `time_focus_confidence`, `ad_confidence`).
- In the lists `topics`, `scope`, `news_type`, `affected` and
  `factuality_evidence` unknown items are removed; an empty list is removed.
- In `emotions` unknown names and scores outside 0..1 are removed; an empty
  object is removed.

Valid values are kept unchanged.

## Notes

- Requests are processed concurrently; each one gets its own translator.
- On SIGINT/SIGTERM the server shuts down gracefully, waiting up to 10 seconds
  for active requests.
- The token is sent in plain text over HTTP. When exposing the server outside
  a private network, put it behind a reverse proxy with TLS.
- TranslateGemma needs an explicit `from` in every request (or `--from` at
  start): `auto` is rejected, so `detected_language` is never returned for it.
  A summary and 2+ analyses go through the combined analysis, which works
  through `/v1/completions`. See [local-servers.md](local-servers.md).
