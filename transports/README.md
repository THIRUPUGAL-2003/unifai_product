# UnifAI Gateway

UnifAI Gateway is a blazing-fast HTTP API that unifies access to 15+ AI providers (OpenAI, Anthropic, AWS Bedrock, Google Vertex, and more) through a single OpenAI-compatible interface. Deploy in seconds with zero configuration and get automatic fallbacks, semantic caching, tool calling, and enterprise-grade features.

**Complete Documentation**: [https://docs.unifai.ai](https://docs.unifai.ai)

---

## Quick Start

### Installation

Choose your preferred method:

#### NPX (Recommended)

```bash
# Install and run locally
npx -y @unifai/unifai

# Open web interface at http://localhost:8081
```

#### Docker

```bash
# Pull and run UnifAI Gateway
docker pull unifai/unifai
docker run -p 8081:8081 unifai/unifai

# For persistent configuration
docker run -p 8081:8081 -v $(pwd)/data:/app/data unifai/unifai
```

### Configuration

UnifAI starts with zero configuration needed. Configure providers through the **built-in web UI** at `http://localhost:8081` or via API:

```bash
# Add OpenAI provider via API
curl -X POST http://localhost:8081/api/providers \
  -H "Content-Type: application/json" \
  -d '{
    "provider": "openai",
    "keys": [{"value": "sk-your-openai-key", "models": ["gpt-4o-mini"], "weight": 1.0}]
  }'
```

For file-based configuration, create `config.json` in your app directory:

```json
{
  "providers": {
    "openai": {
      "keys": [{"value": "env.OPENAI_API_KEY", "models": ["gpt-4o-mini"], "weight": 1.0}]
    }
  }
}
```

### Your First API Call

```bash
curl -X POST http://localhost:8081/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "openai/gpt-4o-mini",
    "messages": [{"role": "user", "content": "Hello, UnifAI!"}]
  }'
```

**That's it!** You now have a unified AI gateway running locally.

---

## Key Features

UnifAI Gateway provides enterprise-grade AI infrastructure with these core capabilities:

### Core Features

- **[Unified Interface](https://docs.unifai.ai/features/unified-interface)** - Single OpenAI-compatible API for all providers
- **[Multi-Provider Support](https://docs.unifai.ai/quickstart/gateway/provider-configuration)** - OpenAI, Anthropic, AWS Bedrock, Google Vertex, Cerebras, Azure, Cohere, Mistral, Ollama, Groq, and more
- **[Drop-in Replacement](https://docs.unifai.ai/features/drop-in-replacement)** - Replace OpenAI/Anthropic/GenAI SDKs with zero code changes
- **[Automatic Fallbacks](https://docs.unifai.ai/features/fallbacks)** - Seamless failover between providers and models
- **[Streaming Support](https://docs.unifai.ai/quickstart/gateway/streaming)** - Real-time response streaming for all providers

### Advanced Features

- **[Model Context Protocol (MCP)](https://docs.unifai.ai/features/mcp)** - Enable AI models to use external tools (filesystem, web search, databases)
- **[Semantic Caching](https://docs.unifai.ai/features/semantic-caching)** - Intelligent response caching based on semantic similarity
- **[Load Balancing](https://docs.unifai.ai/features/fallbacks)** - Distribute requests across multiple API keys and providers
- **[Governance & Budget Management](https://docs.unifai.ai/features/governance)** - Usage tracking, rate limiting, and cost control
- **[Custom Plugins](https://docs.unifai.ai/enterprise/custom-plugins)** - Extensible middleware for analytics, monitoring, and custom logic

### Enterprise Features

- **[Clustering](https://docs.unifai.ai/enterprise/clustering)** - Multi-node deployment with shared state
- **[User Provisioning (OIDC)](https://docs.unifai.ai/enterprise/user-provisioning)** - OAuth 2.0 / OIDC login with background directory sync
- **[Vault Support](https://docs.unifai.ai/enterprise/vault-support)** - Secure API key management
- **[Custom Analytics](https://docs.unifai.ai/features/observability)** - Detailed usage insights and monitoring
- **[In-VPC Deployments](https://docs.unifai.ai/enterprise/invpc-deployments)** - Private cloud deployment options

**Learn More**: [Complete Feature Documentation](https://docs.unifai.ai/features/unified-interface)

---

## SDK Integrations

Replace your existing SDK base URLs to unlock UnifAI's features instantly:

### OpenAI SDK

```python
import openai
client = openai.OpenAI(
    base_url="http://localhost:8081/openai",
    api_key="dummy"  # Handled by UnifAI
)
```

### Anthropic SDK

```python
import anthropic
client = anthropic.Anthropic(
    base_url="http://localhost:8081/anthropic",
    api_key="dummy"  # Handled by UnifAI
)
```

### Google GenAI SDK

```python
import google.generativeai as genai
genai.configure(
    transport="rest",
    api_endpoint="http://localhost:8081/genai",
    api_key="dummy"  # Handled by UnifAI
)
```

**Complete Integration Guides**: [SDK Integrations](https://docs.unifai.ai/integrations/what-is-an-integration)

---

## Documentation

### Getting Started

- [Quick Setup Guide](https://docs.unifai.ai/quickstart/gateway/setting-up) - Detailed installation and configuration
- [Provider Configuration](https://docs.unifai.ai/quickstart/gateway/provider-configuration) - Connect multiple AI providers
- [Integration Guide](https://docs.unifai.ai/quickstart/gateway/integrations) - SDK replacements

### Advanced Topics

- [MCP Tool Calling](https://docs.unifai.ai/features/mcp) - External tool integration
- [Semantic Caching](https://docs.unifai.ai/features/semantic-caching) - Intelligent response caching
- [Fallbacks & Load Balancing](https://docs.unifai.ai/features/fallbacks) - Reliability and scaling
- [Budget Management](https://docs.unifai.ai/features/governance) - Cost control and governance

**Browse All Documentation**: [https://docs.unifai.ai](https://docs.unifai.ai)

---

*Built with ❤️ by [Maxim](https://getmaxim.ai)*
