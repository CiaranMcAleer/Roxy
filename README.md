# 🚀 Roxy

Roxy is a high-performance proxy server designed to enhance your interactions with Large Language Models (LLMs). It sits seamlessly between your application and various LLM providers, offering intelligent API key rotation, model substitution, and advanced request routing capabilities.

## ✨ Key Features

- **💬 Chat-Based Configuration**: Manage configuration via chat commands:
  - Add/remove API keys at runtime
  - Modify model substitution rules
  - Get help documentation
  - All changes persist across restarts

- **🧠 Prompt Caching**: Reduce API costs with:
  - Automatic response caching
  - Configurable TTL
  - Smart cache invalidation
  - Significant cost savings

- **� Smart Key Rotation**: Sophisticated API key management:
  - Automatic rate limit avoidance
  - Intelligent load balancing
  - Configurable cooldown periods
  - Usage-based rotation
  - Secure key storage via environment variables

- **🔄 Model Substitution & Routing**: Advanced request routing with:
  - Multiple selection policies (Random, Round-robin, Fallback)
  - Automatic failover handling
  - Cost optimization
  - Cross-provider model substitution

- **🌐 Multi-Provider Support**: Seamlessly work with multiple LLM providers:
  - OpenAI
  - Anthropic
  - OpenRouter
  - Chutes AI
  - Easily extensible for more providers

- **🔒 Security First**: Built with security in mind:
  - Environment variable-based key management
  - No plaintext keys in configuration
  - Secure default settings
  - Rate limit protection

## 🛠️ Technical Stack

- Built with Go for maximum performance and reliability
- Clean, modular architecture
- Easy to configure and extend
- Production-ready logging and monitoring

## 🚀 Getting Started

1. Clone and set up the project:
```bash
# Clone the repository
git clone https://github.com/CiaranMcAleer/Roxy.git

# Navigate to the project directory
cd Roxy

# Install dependencies
go mod download
```

2. Set up your environment variables:
```bash
# OpenAI keys
export OPENAI_API_KEY_1="your-key-1"
export OPENAI_API_KEY_2="your-key-2"

# Anthropic key
export ANTHROPIC_API_KEY_1="your-anthropic-key"

# OpenRouter key
export OPENROUTER_API_KEY="your-openrouter-key"

# Chutes AI key
export CHUTES_API_KEY="your-chutes-key"
```

3. Create your configuration:
```bash
# Copy the example config
cp configs/config.example.yaml configs/config.yaml

# Edit the config file with your settings
vim configs/config.yaml
```

4. Run the server:
```bash
go run cmd/roxy/main.go -config configs/config.yaml
```

## 📦 Project Structure

```
.
├── cmd/
│   └── roxy/           # Application entrypoint
├── internal/           # Private application code
│   ├── api/           # API handlers
│   ├── config/        # Configuration management
│   ├── proxy/         # Proxy logic
│   └── rotation/      # Key rotation logic
├── pkg/               # Public libraries
└── configs/           # Configuration files
```

## 📝 Configuration

Configuration combines environment variables for sensitive data and a YAML file for general settings:

### Environment Variables

- `OPENAI_API_KEY_*`: OpenAI API keys
- `ANTHROPIC_API_KEY_*`: Anthropic API keys
- `OPENROUTER_API_KEY`: OpenRouter API key
- `CHUTES_API_KEY`: Chutes AI API key

### Config File (config.yaml)

```yaml
listen_addr: ":8080"

# API key configurations
api_keys:
  - key_env_var: "OPENAI_API_KEY_1"    # Reference to environment variable
    provider: "openai"
    max_rpm: 3500                       # Rate limit: requests per minute
    max_tpm: 90000                      # Rate limit: tokens per minute
    cooldown_sec: 60                    # Cooldown period after rate limit

# Model substitution rules
model_rules:
  - source_model: "gpt-4"              # Original requested model
    target_models:                      # Alternative models to try
      - "gpt-4"
      - "claude-2"
    selection_policy: "fallback"        # Policy: random, roundrobin, or fallback

# Provider configurations
providers:
  openai:
    base_url: "https://api.openai.com/v1"
  anthropic:
    base_url: "https://api.anthropic.com/v1"
  openrouter:
    base_url: "https://openrouter.ai/api/v1"
  chutes:
    base_url: "https://api.chutesai.com/v1"
```

### Selection Policies

1. **Random**: Randomly select from available models
2. **Round-robin**: Cycle through models in order
3. **Fallback**: Try models in order until successful

## 💬 Chat Commands

Roxy supports configuration via special chat commands (prefixed with #roxy):

### Key Management
```
#roxy add key [provider] [key] - Add new API key
#roxy remove key [provider] [key] - Remove API key
#roxy list keys - List configured keys
```

### Model Configuration
```
#roxy add model [source] [target] - Add model substitution
#roxy remove model [source] - Remove model substitution
```

### System Commands
```
#roxy help - Show available commands
#roxy status - Show system status
```

### Cache Management
```
#roxy cache clear - Clear all cached responses
#roxy cache stats - Show cache statistics
```

##  Security Considerations

- Never store API keys in the config file
- Use environment variables for all sensitive data
- Regularly rotate API keys
- Monitor usage patterns for anomalies
- Set appropriate rate limits

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## 📜 License

This project is licensed under the unlicense. See the [LICENSE](LICENSE) file for details.