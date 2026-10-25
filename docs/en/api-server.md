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

`docker-compose.yml` mounts `./config.yaml` to `/config.yaml` and publishes
port 8080. To pass extra flags, override `command`:

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

A missing or wrong token returns 401 `{"error":"unauthorized"}`.
`GET /health` stays open. Without a token the server accepts any request and
logs a warning at start.

## Endpoints

### `GET /health`

```json
{"status":"ok"}
```

### `POST /v1/translate`

Request:

| Field  | Required | Description |
|--------|----------|-------------|
| `text` | yes      | Text to translate |
| `from` | no       | Source language; defaults to `--from` (`auto`) |
| `to`   | no       | Target language; defaults to `default_target_language` |

```bash
curl -s -X POST localhost:8080/v1/translate \
  -d '{"text":"Apple opened a new office in Berlin on Monday.","from":"en","to":"de"}'
```

Response:

```json
{
  "text": "Apple hat am Montag ein neues Büro in Berlin eröffnet.",
  "analysis": {
    "dates": ["Montag"],
    "locations": ["Berlin"],
    "organizations": ["Apple"],
    "sentiment": "neutral",
    "sentiment_score": 0,
    "tags": ["büro", "berlin", "apple", "eröffnung", "montag"]
  }
}
```

`analysis` contains only the analyses enabled in `settings` or by flags; it is
omitted when none are enabled.

Errors are returned as `{"error": "..."}` with status 400 (bad request),
401 (bad token) or 502 (the LLM provider failed).

## Notes

- Requests are processed concurrently; each one gets its own translator.
- The token is sent in plain text over HTTP. When exposing the server outside
  a private network, put it behind a reverse proxy with TLS.
- TranslateGemma needs an explicit `from` in every request (or `--from` at
  start). See [local-servers.md](local-servers.md).
