# API-сервер

`llm-translate --api` запускает HTTP-сервер, который одним запросом переводит
текст и выполняет анализы, включённые в конфиге.

## Запуск

```bash
llm-translate --api -c config.yaml                     # слушает :8080
llm-translate --api --listen 127.0.0.1:9000 -c config.yaml
```

В режиме API работают все флаги CLI, кроме флагов ввода/вывода:
`--provider`, `--model`, `--style`, `--glossary`, `--sentiment`, `--tags`,
`--parallel-analysis` и т.д. Они задают значения по умолчанию для всего сервера.

## Docker

```bash
cp config.example.yaml config.yaml   # указать провайдера, модель и анализы
docker compose up -d --build
```

`docker-compose.yml` монтирует `./config.yaml` в `/config.yaml` и публикует
порт 8080. Дополнительные флаги задаются через `command`:

```yaml
command: ["--api", "-c", "/config.yaml", "--parallel-analysis", "--tags", "5"]
```

Ключи облачных провайдеров можно передать через переменные окружения
(`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, ...) в секции `environment:`.

## Авторизация

Токен задаётся флагом `--api-token` или ключом `settings.api_token` в конфиге
(флаг важнее). После этого каждый `POST /v1/translate` должен его передавать:

```bash
llm-translate --api -c config.yaml --api-token s3cret
curl -s -X POST localhost:8080/v1/translate \
  -H 'Authorization: Bearer s3cret' \
  -d '{"text":"Hello","from":"en","to":"ru"}'
```

Без токена или с неверным токеном сервер вернёт 401 `{"error":"unauthorized"}`.
`GET /health` остаётся открытым. Если токен не задан, сервер принимает любые
запросы и пишет предупреждение при старте.

## Эндпоинты

### `GET /health`

```json
{"status":"ok"}
```

### `POST /v1/translate`

Запрос:

| Поле   | Обязательное | Описание |
|--------|--------------|----------|
| `text` | да           | Текст для перевода |
| `from` | нет          | Исходный язык; по умолчанию `--from` (`auto`) |
| `to`   | нет          | Целевой язык; по умолчанию `default_target_language` |

```bash
curl -s -X POST localhost:8080/v1/translate \
  -d '{"text":"Apple opened a new office in Berlin on Monday.","from":"en","to":"de"}'
```

Ответ:

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

`analysis` содержит только анализы, включённые в `settings` или флагами; если
ни один не включён, поле отсутствует.

Ошибки возвращаются как `{"error": "..."}` со статусом 400 (неверный запрос),
401 (неверный токен) или 502 (ошибка LLM-провайдера).

## Замечания

- Запросы обрабатываются параллельно, у каждого свой экземпляр translator.
- По HTTP токен передаётся открытым текстом. Если сервер доступен извне
  закрытой сети, ставьте перед ним reverse proxy с TLS.
- TranslateGemma требует явный `from` в каждом запросе (или `--from` при
  запуске). См. [local-servers.md](local-servers.md).
