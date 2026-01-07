# LLM-Translate

A powerful command-line tool for translating text between languages using various Large Language Model (LLM) providers.

## Features

- **Multiple LLM Providers**: Support for OpenAI, Anthropic, Google, Ollama, and OpenRouter
- **Flexible Input/Output**: File-based, pipe, and interactive modes
- **Smart Text Processing**: Automatic chunking for long texts
- **Format Preservation**: Maintains Markdown and HTML formatting
- **Glossary Support**: Ensure consistent translation of technical terms
- **Strong Validation Mode**: Verify translations don't contain source language text
- **Proxy Support**: HTTP, HTTPS, and SOCKS5 proxy configuration
- **Retry Logic**: Automatic retries with exponential backoff
- **Configurable**: YAML configuration files with environment variable support

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
echo "Hello world" | llm-translate -to ru

# Translate a file
llm-translate -i document.txt -o document_ru.txt -from en -to ru

# Interactive mode
llm-translate -to de
# Type text and press Ctrl+D to translate
```

### Using Different Providers

```bash
# Use Anthropic Claude
llm-translate -p anthropic -m claude-3-5-sonnet-20241022 -i text.txt -to ru

# Use local Ollama
llm-translate -p ollama -m llama3.2 -i text.txt -to es

# Use Google Gemini
llm-translate -p google -m gemini-2.0-flash -i text.txt -to fr
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
| `--verbose` | `-v` | Verbose output | false |
| `--quiet` | `-q` | Quiet mode | false |
| `--proxy` | `-x` | Proxy server | from config |
| `--help` | `-h` | Show help | - |

## Advanced Features

### Translation Styles

```bash
# Formal style for official documents
llm-translate -i letter.txt -o letter_de.txt -to de --style formal

# Technical style preserving terminology
llm-translate -i docs.md -o docs_ru.md -to ru --style technical
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
llm-translate -i tech.txt -o tech_ru.txt -to ru --glossary terms.yaml
```

### Strong Validation Mode

Ensures the translation doesn't contain untranslated source language text:

```bash
llm-translate -i document.txt -o document_ru.txt -from en -to ru --strong
```

### Proxy Configuration

```bash
# Using SOCKS5 proxy
llm-translate -i file.txt -o output.txt -to ru \
  --proxy socks5://proxy.example.com:1080

# HTTP proxy with authentication
llm-translate -i file.txt -o output.txt -to ru \
  --proxy http://user:pass@proxy.example.com:8080
```

## Examples

### Translate Documentation

```bash
# Translate README preserving Markdown formatting
llm-translate -i README.md -o README_ru.md -to ru --preserve-format
```

### Batch Translation

```bash
# Translate all .txt files in a directory
for file in *.txt; do
  llm-translate -i "$file" -o "${file%.txt}_ru.txt" -to ru
done
```

### Pipeline Integration

```bash
# Extract and translate
curl https://example.com/api/docs | \
  jq -r '.content' | \
  llm-translate -to ru > translated.txt
```

### Using with Local Models

```bash
# Start Ollama server first
ollama serve

# Translate using local model
llm-translate -p ollama -m llama3.2 -i text.txt -to es
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