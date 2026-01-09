# LLM-Translate

A powerful command-line tool for translating text between languages using various Large Language Model (LLM) providers.

## Features

- **Multiple LLM Providers**: Support for OpenAI, Anthropic, Google, Ollama, and OpenRouter
- **Flexible Input/Output**: File-based and pipe modes
- **Smart Text Processing**: Automatic chunking for long texts
- **Format Preservation**: Maintains Markdown and HTML formatting
- **Glossary Support**: Ensure consistent translation of technical terms
- **Strong Validation Mode**: Verify translations don't contain source language text
- **Proxy Support**: HTTP, HTTPS, and SOCKS5 proxy configuration
- **Retry Logic**: Automatic retries with exponential backoff
- **Configurable**: YAML configuration files with environment variable support
- **Text Analysis**: Sentiment analysis, emotion detection, topic classification, and tag extraction

## Installation

### From Source

```bash
go install github.com/user/llm-translate/cmd/llm-translate@latest
```

### Build Manually

```bash
git clone https://github.com/user/llm-translate.git
cd llm-translate
make build
```

## Quick Start

### Basic Translation

```bash
# Translate text from English to Russian
echo "Hello world" | llm-translate -t ru

# Translate a file
llm-translate -i document.txt -o document_ru.txt -f en -t ru
```

### Using Different Providers

```bash
# Use Anthropic Claude
llm-translate -p anthropic -m claude-3-5-sonnet-20241022 -i text.txt -t ru

# Use local Ollama
llm-translate -p ollama -m llama3.2 -i text.txt -t es

# Use Google Gemini
llm-translate -p google -m gemini-2.0-flash -i text.txt -t fr
```

## Configuration

### Configuration File

Create a configuration file at `~/.config/llm-translate/config.yaml`:

```yaml
default_provider: openai
default_target_language: ru

settings:
  temperature: 0.3
  max_tokens: 4096
  timeout: 60
  chunk_size: 3000
  preserve_format: false
  retry_count: 3
  retry_delay: 1

providers:
  openai:
    api_key: ${OPENAI_API_KEY}
    base_url: https://api.openai.com/v1
    model: gpt-4o-mini
    
  anthropic:
    api_key: ${ANTHROPIC_API_KEY}
    base_url: https://api.anthropic.com
    model: claude-3-5-sonnet-20241022
    
  google:
    api_key: ${GOOGLE_API_KEY}
    base_url: https://generativelanguage.googleapis.com/v1beta
    model: gemini-2.0-flash
    
  ollama:
    base_url: http://localhost:11434
    model: llama3.2
    
  openrouter:
    api_key: ${OPENROUTER_API_KEY}
    base_url: https://openrouter.ai/api/v1
    model: anthropic/claude-3.5-sonnet

# Strong validation settings
strong_validation:
  enabled: false
  max_retries: 3
  allowed_patterns:
    - '\b[A-Z]{2,}\b'  # Acronyms
    - '`[^`]+`'        # Code blocks
  allowed_terms:
    - API
    - HTTP
    - JSON

# Proxy configuration
proxy:
  url: socks5://proxy.example.com:1080
  username: ${PROXY_USER}
  password: ${PROXY_PASS}
  no_proxy:
    - localhost
    - 127.0.0.1
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `LLM_TRANSLATE_CONFIG` | Path to configuration file |
| `LLM_TRANSLATE_PROVIDER` | Default provider |
| `LLM_TRANSLATE_MODEL` | Default model |
| `LLM_TRANSLATE_PROXY` | Proxy URL |
| `OPENAI_API_KEY` | OpenAI API key |
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `GOOGLE_API_KEY` | Google API key |
| `OPENROUTER_API_KEY` | OpenRouter API key |

## Command-Line Options

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--input` | `-i` | Input file | stdin |
| `--output` | `-o` | Output file | stdout |
| `--dir` | `-d` | Input directory for recursive translation | - |
| `--ext` | | File extensions to translate | .md,.txt |
| `--suffix` | | Output file suffix (e.g., _ru) | _\<lang\> |
| `--prefix` | | Output file prefix (e.g., ru_) | - |
| `--from` | `-f` | Source language | auto |
| `--to` | `-t` | Target language | en |
| `--provider` | `-p` | LLM provider | from config |
| `--model` | `-m` | Model to use | from config |
| `--config` | `-c` | Config file path | ~/.config/llm-translate/config.yaml |
| `--api-key` | `-k` | API key | from config |
| `--temperature` | | Generation temperature | 0.3 |
| `--max-tokens` | | Max response tokens | 4096 |
| `--style` | | Translation style | - |
| `--glossary` | `-g` | Glossary file | - |
| `--preserve-format` | | Keep formatting | false |
| `--strong` | `-s` | Strong validation mode | false |
| `--sentiment` | | Analyze sentiment of translated text | false |
| `--tags` | | Extract N tags from text (0 to disable) | 0 |
| `--classify` | | Classify text by topics, scope, and type | false |
| `--emotions` | | Analyze emotions (fear, anger, hope, etc.) | false |
| `--factuality` | | Check factuality (confirmed, rumors, forecasts, unsourced) | false |
| `--impact` | | Analyze who is affected (individuals, business, government, etc.) | false |
| `--sensationalism` | | Analyze sensationalism level (neutral, emotional, clickbait, manipulative) | false |
| `--verbose` | | Verbose output | false |
| `--version` | `-v` | Show version | - |
| `--quiet` | `-q` | Quiet mode | false |
| `--proxy` | `-x` | Proxy server | from config |
| `--help` | `-h` | Show help | - |

## Advanced Features

### Directory Translation

Translate all files in a directory recursively:

```bash
# Translate all .md and .txt files (default extensions)
llm-translate -d ./docs -t ru

# Output: file.md -> file_ru.md

# Custom suffix
llm-translate -d ./docs -t ru --suffix _russian
# Output: file.md -> file_russian.md

# Use prefix instead
llm-translate -d ./docs -t ru --prefix ru_
# Output: file.md -> ru_file.md

# Specific extensions
llm-translate -d ./content -t ru --ext ".md,.html"

# Already translated files are automatically skipped
```

### Translation Styles

```bash
# Formal style for official documents
llm-translate -i letter.txt -o letter_de.txt -t de --style formal

# Technical style preserving terminology
llm-translate -i docs.md -o docs_ru.md -t ru --style technical
```

### Using Glossaries

Create a glossary file `terms.yaml`:

```yaml
terms:
  - source: "machine learning"
    target: "машинное обучение"
  - source: "API"
    target: "API"
    note: "не переводить"
```

Use it in translation:

```bash
llm-translate -i tech.txt -o tech_ru.txt -t ru --glossary terms.yaml
```

### Strong Validation Mode

Ensures the translation doesn't contain untranslated source language text:

```bash
llm-translate -i document.txt -o document_ru.txt -f en -t ru --strong
```

### Text Analysis

Analyze translated text for sentiment, emotions, classification, impact, and extract key tags. Results are added to frontmatter in Markdown files.

```bash
# Analyze sentiment of translated text
llm-translate -i article.txt -o article_ru.txt -t ru --sentiment

# Extract 5 tags from translated text
llm-translate -i article.txt -o article_ru.txt -t ru --tags 5

# Classify by topics, scope, and news type
llm-translate -i article.txt -o article_ru.txt -t ru --classify

# Analyze emotions (fear, anger, hope, uncertainty, optimism, panic)
llm-translate -i article.txt -o article_ru.txt -t ru --emotions

# Check factuality (confirmed data, rumors, forecasts, unsourced claims)
llm-translate -i article.txt -o article_ru.txt -t ru --factuality

# Analyze who is affected by the news
llm-translate -i article.txt -o article_ru.txt -t ru --impact

# Analyze sensationalism level
llm-translate -i article.txt -o article_ru.txt -t ru --sensationalism

# Full analysis - combine all
llm-translate -i article.txt -o article_ru.txt -t ru \
  --sentiment --tags 5 --classify --emotions --factuality --impact --sensationalism
```

Output frontmatter example:

```yaml
---
title: Article
sentiment: positive
sentiment_score: 0.75
tags:
  - technology
  - innovation
  - ai
topics:
  - technology
  - economics
scope:
  - international
news_type:
  - corporate
emotions:
  fear: 0.2
  hope: 0.8
  optimism: 0.7
factuality: confirmed
factuality_confidence: 0.85
factuality_evidence:
  - official_source
  - statistics
  - quotes
affected:
  - business
  - investors
  - consumers
sensationalism: neutral
sensationalism_confidence: 0.9
sensationalism_markers:
  - factual_language
---
```

**Classification categories:**
- **Topics**: politics, economics, technology, medicine, incidents
- **Scope**: regional, international
- **News type**: corporate, regulatory, macro

**Emotions detected:**
- fear, anger, hope, uncertainty, optimism, panic (with intensity 0.0-1.0)

**Factuality types:**
- **confirmed**: verified facts with clear sources or official data
- **rumors**: unverified information, hearsay, "sources say"
- **forecasts**: predictions, projections, future expectations
- **unsourced**: claims without attribution or evidence

**Impact - who is affected:**
- individuals, business, government, investors, consumers

**Sensationalism levels:**
- **neutral**: factual, balanced reporting without emotional language
- **emotional**: emotionally charged language, dramatic descriptions
- **clickbait**: exaggerated headlines, curiosity gaps, misleading hooks
- **manipulative**: deliberate distortion, fear-mongering, propaganda techniques

Configuration in YAML:

```yaml
settings:
  sentiment: true
  tags_count: 5
  classify: true
  emotions: true
  factuality: true
  impact: true
  sensationalism: true
```

### Proxy Configuration

```bash
# Using SOCKS5 proxy
llm-translate -i file.txt -o output.txt -t ru \
  --proxy socks5://proxy.example.com:1080

# HTTP proxy with authentication
llm-translate -i file.txt -o output.txt -t ru \
  --proxy http://user:pass@proxy.example.com:8080
```

## Examples

### Translate Documentation

```bash
# Translate README preserving Markdown formatting
llm-translate -i README.md -o README_ru.md -t ru --preserve-format
```

### Batch Translation

```bash
# Translate all files in a directory
llm-translate -d ./docs -t ru

# Translate only markdown files with custom suffix
llm-translate -d ./content -t ru --ext ".md" --suffix _translated
```

### Pipeline Integration

```bash
# Extract and translate
curl https://example.com/api/docs | \
  jq -r '.content' | \
  llm-translate -t ru > translated.txt
```

### Using with Local Models

```bash
# Start Ollama server first
ollama serve

# Translate using local model
llm-translate -p ollama -m llama3.2 -i text.txt -t es
```

## Language Codes

Use standard ISO 639-1 codes:

- `en` - English
- `ru` - Russian  
- `de` - German
- `fr` - French
- `es` - Spanish
- `zh` - Chinese
- `ja` - Japanese
- `ko` - Korean
- `auto` - Auto-detect source language

## Error Codes

| Code | Description |
|------|-------------|
| 0 | Success |
| 1 | Invalid arguments |
| 2 | Configuration error |
| 3 | Input file error |
| 4 | Output file error |
| 5 | API error |
| 6 | Token limit exceeded |
| 7 | Timeout |
| 8 | Strong validation failed |

## Building from Source

### Prerequisites

- Go 1.21 or later
- Make (optional)

### Build

```bash
# Using make
make build

# Or directly with go
go build -o bin/llm-translate cmd/llm-translate/main.go

# Build for all platforms
make build-all
```

### Running Tests

```bash
make test

# With coverage
make test-coverage
```

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

MIT License - see LICENSE file for details

## Support

For issues and feature requests, please use the [GitHub issue tracker](https://github.com/user/llm-translate/issues).