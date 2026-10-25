# Local OpenAI-compatible servers (vLLM, llama-swap)

[Русская версия](../ru/local-servers.md)

The `vllm` provider works with any OpenAI-compatible server: vLLM, llama-swap
and similar. It uses `/v1/chat/completions` for regular models and adds
special handling for TranslateGemma models.

## Configuration

```yaml
default_provider: vllm

settings:
  timeout: 300          # model loading on llama-swap can take minutes

providers:
  vllm:
    base_url: http://localhost:8000/v1   # required
    model: translategemma-12b            # required
    api_key: ""                          # optional
```

Command-line equivalent:

```bash
llm-translate -p vllm -u http://localhost:8000/v1 -m translategemma-12b \
  -f en -t ru -i text.md --timeout 300
```

### Timeout

llama-swap loads a model on the first request. A 27B model took 160-200
seconds to load in our tests, so the default `timeout: 60` fails with
`context deadline exceeded`. Use `timeout: 300` or keep the model loaded.

## TranslateGemma

The provider detects TranslateGemma when the model name contains
`translategemma` (case-insensitive).

Translation:

- The request is a single `user` message with `source_lang_code`,
  `target_lang_code` and the text, as the model's chat template requires.
- The source language must be set explicitly: `--from auto` is rejected.
- `--style`, `--context` and glossaries are not supported.

Text analysis:

- Works for all analysis types.
- Requests go to `/v1/completions` with a raw Gemma prompt, bypassing the
  model's strict chat template.

## Reasoning models (Qwen3 and similar)

Reasoning can consume the whole `max_tokens` budget, so the analysis comes
back empty. Pass `chat_template_kwargs` to the server to disable thinking:

```yaml
providers:
  vllm:
    base_url: http://localhost:8000/v1
    model: qwen3.8-27b
    chat_template_kwargs:
      enable_thinking: false
```

`chat_template_kwargs` is sent as is in the request body. It is supported by
the `openai` and `vllm` providers.

In our test (qwen3.8-27b, translation plus 12 analyses) this reduced the run
from 291 s with empty analysis to about 21 s with all sections filled.

## Parallel analysis

By default the text is translated first and then the translation is
analyzed: two requests, one after another.

With `--parallel-analysis` (or `settings.parallel_analysis: true`) the
original text is analyzed at the same time as it is translated. Servers that
batch requests (vLLM) process both together, so the total time is close to
the longest request.

```bash
llm-translate -c config.yaml -i article.md -f en -t ru --parallel-analysis \
  --sentiment --tags 5 --classify --entities --events
```

Measured via CLI, en->ru, all 12 analyses, model already loaded:

| Model | Sequential (default) | `--parallel-analysis` |
|---|---|---|
| qwen3.8-27b (thinking disabled) | 20.1 s | 13.3 s |
| translategemma-12b | 10.8-11.1 s | 6.9 s |

The test used one news article of about 900 characters. On long texts the
share of translation grows and the gain may differ.

Things to know:

- The analysis prompt asks for free-text values (tags, entities, events,
  markers) in the target language. Fixed values such as `negative` or
  `economics` stay in English.
- Models follow the language instruction unevenly: tags come back in the
  target language, entities and events may stay in the source language.
- Tags extracted from the original may differ from tags extracted from the
  translation.
- If the combined analysis fails and the CLI falls back to individual
  analysis requests, the language instruction is not sent with them.
- The gain matters only when the model is already loaded. Switching models
  on llama-swap costs minutes and outweighs any request optimization, so use
  one model for both translation and analysis.

## Other ways to speed things up

- Enable only the analyses you need: analysis time depends on the number of
  output tokens (about 390 for all 12 analyses).
- Smaller models are faster: translategemma-12b is about twice as fast as
  27B models, but its analysis quality was lower in our tests.
