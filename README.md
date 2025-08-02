[![CI Pipeline](https://github.com/neural-chilli/aceryx/actions/workflows/build.yml/badge.svg)](https://github.com/neural-chilli/aceryx/actions/workflows/build.yml)
[![Quarto Docs](https://img.shields.io/badge/docs-online-blue.svg)](https://neural-chilli.github.io/aceryx/)

# Aceryx 🍁

> **An open-source agentic flow builder for Rust**

Aceryx is a modern, scalable platform for building and orchestrating AI agent workflows. Named after the maple tree, it embodies the principle of natural coordination - multiple agents branching, merging, and collaborating toward common goals with elegant simplicity.

## ✨ Features

- **Visual Flow Designer** - Intuitive drag-and-drop interface powered by ReactFlow
- **MCP Integration** - Native Model Context Protocol support for seamless tool connectivity
- **Scalable Architecture** - From single binary to distributed clusters
- **Multiple Backends** - In-memory, Redis, and PostgreSQL storage options
- **AI-First** - Built on Rig for robust AI tooling and agent coordination
- **Production Ready** - Designed for enterprise deployment with clustering and resilience

## 🚀 Quick Start

### Single Binary (In-Memory)
```bash
# Download and run - no setup required
curl -L https://github.com/yourusername/aceryx/releases/latest/download/aceryx-linux-x64 -o aceryx
chmod +x aceryx
./aceryx serve
```

Open http://localhost:8080 to access the flow designer.

### Docker
```bash
docker run -p 8080:8080 aceryx/aceryx:latest
```

### From Source
```bash
git clone https://github.com/yourusername/aceryx.git
cd aceryx
cargo run -- serve
```

## 🏗️ Architecture

Aceryx is built with a modular, trait-based architecture that scales from development to production:

### Storage Backends

All backends implement the same `FlowStorage` trait, enabling seamless transitions between deployment modes:

- **In-Memory** - Perfect for development and single-node deployments
- **Redis** - Distributed coordination with persistence and pub/sub
- **PostgreSQL** - Enterprise-grade persistence with ACID guarantees

### Technology Stack

**Backend (Rust)**
- [Axum](https://github.com/tokio-rs/axum) - High-performance web framework
- [Rig](https://github.com/0xPlaygrounds/rig) - AI agent tooling and orchestration
- [MCP](https://modelcontextprotocol.io/) - Model Context Protocol integration
- [Tokio](https://tokio.rs/) - Async runtime

**Frontend**
- [ReactFlow](https://reactflow.dev/) - Visual flow designer
- [HTMX](https://htmx.org/) + [Alpine.js](https://alpinejs.dev/) - Interactive UI
- [Minijinja](https://github.com/mitsuhiko/minijinja) - Server-side templating
- [Tabler](https://tabler.io/) - UI components and styling
- [Tabulator.js](https://tabulator.info/) - Advanced data tables
- [Cytoscape.js](https://cytoscape.org/) - Graph visualization
- [Timeline.js](https://timeline.knightlab.com/) - Timeline components

## 🔧 Configuration

### Environment Variables

```bash
# Server Configuration
ACERYX_PORT=8080
ACERYX_HOST=0.0.0.0

# Storage Backend
ACERYX_STORAGE=memory  # memory | redis | postgres

# Redis Configuration (when using Redis backend)
REDIS_URL=redis://localhost:6379

# PostgreSQL Configuration (when using PostgreSQL backend)
DATABASE_URL=postgresql://user:pass@localhost/aceryx

# AI Configuration
OPENAI_API_KEY=your_openai_key
ANTHROPIC_API_KEY=your_anthropic_key
```

### Configuration File

Create `aceryx.toml` in your working directory:

```toml
[server]
port = 8080
host = "0.0.0.0"

[storage]
backend = "memory"  # memory | redis | postgres

[storage.redis]
url = "redis://localhost:6379"
pool_size = 10

[storage.postgres]
url = "postgresql://user:pass@localhost/aceryx"
max_connections = 20

[ai]
default_provider = "openai"
max_tokens = 4096
timeout = 30

[clustering]
node_id = "node-1"
discovery_interval = 30
heartbeat_interval = 10
```

## 🌊 Creating Flows

### Basic Flow Example

```yaml
name: "Document Processor"
description: "Processes uploaded documents with AI analysis"

nodes:
  - id: "upload"
    type: "trigger"
    config:
      trigger_type: "file_upload"
      accepted_types: ["pdf", "docx", "txt"]

  - id: "extract"
    type: "mcp_tool"
    config:
      tool: "document_extractor"
      input_mapping:
        file: "{{upload.file}}"

  - id: "analyze"
    type: "ai_agent"
    config:
      provider: "openai"
      model: "gpt-4"
      prompt: "Analyze this document and extract key insights: {{extract.content}}"

  - id: "store"
    type: "database"
    config:
      operation: "insert"
      table: "documents"
      data:
        content: "{{extract.content}}"
        analysis: "{{analyze.result}}"
        processed_at: "{{now()}}"

edges:
  - from: "upload"
    to: "extract"
  - from: "extract"
    to: "analyze"
  - from: "analyze"
    to: "store"
```

### Advanced Features

- **Conditional Branching** - Route flow execution based on data or AI decisions
- **Parallel Execution** - Run multiple agents simultaneously
- **Error Handling** - Automatic retries and fallback strategies
- **State Management** - Persistent context across distributed nodes
- **Real-time Monitoring** - Live flow execution tracking

## 🎯 Use Cases

- **Document Processing** - AI-powered document analysis and transformation
- **Customer Support** - Automated ticket routing and response generation
- **Data Pipelines** - AI-enhanced data processing and enrichment
- **Content Creation** - Multi-step content generation workflows
- **Research Automation** - Automated research and synthesis tasks
- **Integration Workflows** - Connect disparate systems with AI orchestration

## 🚀 Deployment

### Single Node
```bash
# Production single-node deployment
ACERYX_STORAGE=postgres \
DATABASE_URL=postgresql://user:pass@localhost/aceryx \
./aceryx serve --port 8080
```

### Distributed Cluster
```bash
# Node 1
ACERYX_STORAGE=redis \
REDIS_URL=redis://redis-cluster:6379 \
ACERYX_NODE_ID=node-1 \
./aceryx serve --port 8080

# Node 2
ACERYX_STORAGE=redis \
REDIS_URL=redis://redis-cluster:6379 \
ACERYX_NODE_ID=node-2 \
./aceryx serve --port 8081
```

### Kubernetes
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: aceryx
spec:
  replicas: 3
  selector:
    matchLabels:
      app: aceryx
  template:
    metadata:
      labels:
        app: aceryx
    spec:
      containers:
      - name: aceryx
        image: aceryx/aceryx:latest
        ports:
        - containerPort: 8080
        env:
        - name: ACERYX_STORAGE
          value: "redis"
        - name: REDIS_URL
          value: "redis://redis-service:6379"
```

### Development Setup

```bash
# Clone the repository
git clone https://github.com/yourusername/aceryx.git
cd aceryx

# Install dependencies
cargo build

# Run tests
cargo test

# Start development server
cargo run -- serve --dev

# Build frontend assets
npm install
npm run build
```

### Project Structure

```
aceryx/
├── src/
│   ├── main.rs              # Application entry point
│   ├── server/              # Axum web server
│   ├── storage/             # Storage trait and implementations
│   ├── agents/              # AI agent coordination
│   ├── flows/               # Flow execution engine
│   ├── mcp/                 # Model Context Protocol integration
│   └── config/              # Configuration management
├── web/                     # Frontend assets
│   ├── components/          # React components
│   ├── flows/              # ReactFlow designer
│   └── static/             # Static assets
├── migrations/             # Database migrations
└── docs/                   # Documentation
```

## 🔗 Links

- **Website**: [aceryx.org](https://aceryx.org)
- **Documentation**: [docs.aceryx.org](https://docs.aceryx.org)
- **Discord**: [Join our community](https://discord.gg/aceryx)
- **Twitter**: [@aceryx](https://twitter.com/aceryx)

## 📄 License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- The Rust community for amazing tooling and libraries
- [ReactFlow](https://reactflow.dev/) for the excellent flow designer foundation
- [Rig](https://github.com/0xPlaygrounds/rig) for AI agent orchestration
- The [MCP](https://modelcontextprotocol.io/) team for standardizing AI tool integration

---

**Made with 🍁 and Rust**