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

`docker-compose.yml` монтирует `./config.yaml` в `/config.yaml` только для чтения,
публикует порт 8080 и перезапускает контейнер (`unless-stopped`). Контейнер работает от
непривилегированного пользователя `app` (uid 10001), поэтому `config.yaml` должен быть ему доступен
для чтения (`chmod 644`). В образе есть `HEALTHCHECK`, который запрашивает `/health` на порту 8080
(предполагается `--listen :8080` по умолчанию). Дополнительные флаги задаются через `command`:

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

Заголовок должен быть строго `Authorization: Bearer <token>` (префикс `Bearer `
чувствителен к регистру). Без токена или с неверным токеном сервер вернёт 401 `{"error":"unauthorized","code":"unauthorized"}`.
`GET /health` остаётся открытым. Если токен не задан, сервер принимает любые
запросы и пишет предупреждение при старте.

## Эндпоинты

### `GET /health`

```json
{"status":"ok","version":"dev"}
```

### `POST /v1/translate`

Все поля, кроме `text`, необязательные. Клиент, который отправляет только
`{text, from, to}`, получает тот же ответ, что и раньше; в ответе могут
появиться только дополнительные ключи (например, `provider`, `model` и `cost`
в `usage`).

Запрос:

| Поле        | Обязательное | Описание |
|-------------|--------------|----------|
| `text`      | да           | Текст для перевода (или для анализа, см. `translate`) |
| `from`      | нет          | Исходный язык; по умолчанию `--from` (`auto`) |
| `to`        | нет          | Целевой язык; по умолчанию `default_target_language` (явно заданный `--to` заменяет его, если ничего не задано, то `en`) |
| `title`     | нет          | Заголовок. Переводится вместе с текстом, возвращается в `title` и анализируется вместе с текстом |
| `translate` | нет          | Boolean, по умолчанию `true`. При `false` текст и заголовок не переводятся и возвращаются как есть; выполняются только анализы |
| `analysis`  | нет          | Список имён анализов. Заменяет анализы, включённые в конфиге; пустой список `[]` отключает все |
| `tags`      | нет          | Сколько тегов извлечь; переопределяет `tags_count`. `0` отключает теги |
| `summary`   | нет          | Длина резюме в предложениях; переопределяет `settings.summary`. `0` отключает резюме |

Допустимые имена в `analysis`: `sentiment`, `tags`, `classify`, `emotions`,
`factuality`, `impact`, `sensationalism`, `entities`, `events`, `usefulness`,
`time_focus`, `ad_detect`. Регистр не важен; неизвестное имя даёт ошибку 400.

- Если в `analysis` есть `tags`, а `tags` не передан, число тегов берётся из
  `settings.tags_count`, а если он не задан, то равно 5.
- `tags` поверх `analysis`: значение больше 0 включает теги и задаёт их
  число, `0` их отключает.
- Без `analysis` используются анализы из `settings` (и флагов CLI).
- Отрицательные `tags` или `summary` дают ошибку 400.
- При `translate: false` (или когда сработал `skip_same_language`) анализ
  идёт по исходному тексту, а свободные значения запрашиваются на языке `to`.

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

Ответ:

| Поле               | Описание |
|--------------------|----------|
| `text`             | Переведённый текст (исходный при `translate: false`) |
| `title`            | Переведённый заголовок; есть, только если передан `title` |
| `summary`          | Резюме; есть, только если запрошено и получено |
| `detected_language` | Язык исходного текста, см. ниже |
| `analysis`         | Результаты анализов; отсутствует, если их нет |
| `analysis_errors`  | Имена анализов (или `summary`), которые не удались или не вернули результат |
| `usage`            | Расход токенов, есть всегда |

`usage`:

| Поле                | Описание |
|---------------------|----------|
| `prompt_tokens`     | Токены запроса по всем обращениям к LLM в этом вызове |
| `completion_tokens` | Токены ответа по всем обращениям к LLM в этом вызове |
| `provider`          | Использованный провайдер |
| `model`             | Использованная модель |
| `cost`              | Только если провайдер вернул `usage.cost` (например, OpenRouter); сумма по всем запросам, в кредитах, равных USD |

Токены берутся из объекта `usage` в ответах провайдера; если провайдер его не
возвращает, они равны 0.

`detected_language` возвращается, только если выполняется всё сразу:

- анализируется исходный текст: `translate` равен `false` или включён
  `parallel_analysis` (иначе анализируется перевод);
- `from` пустой или `auto`;
- выполняется комбинированный анализ (2 и более анализов или резюме), и
  модель вернула язык.

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

`summary` — отдельное поле и никогда не попадает в `analysis`. При резюме или
2+ анализах сервер делает один комбинированный вызов LLM; резюме возможно
только в нём. Если этот вызов не удался, сервер переходит к отдельным вызовам
и добавляет `summary` в `analysis_errors`.

### Ошибки

Каждая ошибка — JSON-объект с ключами `error` (сообщение), `code` и, для
`rate_limited`, `retry_after` (секунды, только если известно).

| Статус | `code`            | Когда |
|--------|-------------------|-------|
| 400    | `invalid_request` | Некорректный JSON или тело больше 10 МиБ, пустой или состоящий из пробелов `text`, `text` длиннее `max_text_length`, неизвестное имя в `analysis`, отрицательные `tags` или `summary` |
| 401    | `unauthorized`    | Нет токена или он неверный |
| 429    | `rate_limited`    | Провайдер ответил 429. Заголовок `Retry-After` выставляется, если его прислал провайдер |
| 504    | `timeout`         | Истёк `request_timeout` (в том числе при ожидании слота `max_concurrent`) |
| 502    | `provider_error`  | Любая другая ошибка LLM-провайдера |

```json
{"error":"provider returned 429","code":"rate_limited","retry_after":30}
```

```json
{"error":"text is too long: 12034 characters, limit 10000","code":"invalid_request"}
```

Остальные пути и методы обрабатывает сам HTTP-сервер Go: он отвечает
текстовым 404 или 405 (например, `GET /v1/translate` даёт 405), а не
JSON-ошибкой.

## Настройки

Все настройки ниже лежат в `settings`, по умолчанию выключены (`0` / `false`)
и не имеют флагов CLI, кроме `summary` (`--summary`).

| Настройка            | Описание |
|----------------------|----------|
| `summary`            | Длина резюме в предложениях; `0` отключает. В запросе: `summary` |
| `max_concurrent`     | Сколько запросов обрабатывается одновременно; остальные ждут слот |
| `request_timeout`    | Лимит времени одного запроса в секундах, включая ожидание слота; по истечении ответ 504 `timeout` |
| `max_text_length`    | Максимальная длина `text` в символах; более длинный текст даёт ошибку 400 |
| `skip_same_language` | Если `from` равен `to` (без учёта регистра), текст не переводится, как при `translate: false` |
| `strict_analysis`    | Отбрасывает значения анализа вне допустимых наборов, см. ниже |

`strict_analysis` очищает объект `analysis`:

- `sentiment`, `factuality`, `sensationalism`, `time_focus` и `ad_type` должны
  быть одним из допустимых значений; иначе поле удаляется вместе со своей
  уверенностью (`sentiment_score`, `factuality_confidence`,
  `sensationalism_confidence`, `time_focus_confidence`, `ad_confidence`).
- В списках `topics`, `scope`, `news_type`, `affected` и
  `factuality_evidence` неизвестные элементы удаляются; пустой список
  удаляется.
- В `emotions` удаляются неизвестные имена и оценки вне диапазона 0..1;
  пустой объект удаляется.

Допустимые значения остаются без изменений.

## Замечания

- Запросы обрабатываются параллельно, у каждого свой экземпляр translator.
- При SIGINT/SIGTERM сервер завершается корректно, ожидая активные запросы
  до 10 секунд.
- По HTTP токен передаётся открытым текстом. Если сервер доступен извне
  закрытой сети, ставьте перед ним reverse proxy с TLS.
- TranslateGemma требует явный `from` в каждом запросе (или `--from` при
  запуске): `auto` отклоняется, поэтому `detected_language` для неё не
  возвращается. Резюме и 2+ анализа идут через комбинированный анализ, он
  работает через `/v1/completions`. См. [local-servers.md](local-servers.md).
