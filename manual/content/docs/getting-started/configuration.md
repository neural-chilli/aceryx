---
title: Configuration
weight: 4
---

Aceryx is configured entirely via environment variables. This page documents all available options.

## Setting Environment Variables

Create a `.env` file in the Aceryx root directory:

```bash
ACERYX_HTTP_ADDR=:8080
ACERYX_DB_URL=postgres://localhost/aceryx?sslmode=disable
ACERYX_JWT_SECRET=your-secret-key-here
# ... other variables
```

Then load it when running:

```bash
set -a && source .env && set +a && go run ./cmd/aceryx serve
```

Or in Docker, pass them as flags:

```bash
docker run -e ACERYX_HTTP_ADDR=:8080 -e ACERYX_JWT_SECRET=... aceryx:latest
```

---

## Server Configuration

### `ACERYX_HTTP_ADDR`
- **Default**: `:8080`
- **Description**: The address and port the HTTP server listens on
- **Example**: `:8080`, `127.0.0.1:3000`, `0.0.0.0:9000`

### `ACERYX_PORT`
- **Default**: `8080`
- **Description**: Alternative way to specify port (ignored if `ACERYX_HTTP_ADDR` includes a port)
- **Example**: `3000`, `8443`

---

## Database Configuration

{{< callout type="warning" >}}
The connection string must point to a PostgreSQL 17+ database with the `pgvector` extension installed. See [Installation](/docs/getting-started/installation#database-setup) for database setup.
{{< /callout >}}

### `ACERYX_DB_URL` (or `ACERYX_DATABASE_URL` or `DATABASE_URL`)
- **Default**: (none — required)
- **Description**: PostgreSQL connection string. The CLI checks these in priority order: `ACERYX_DB_URL` → `ACERYX_DATABASE_URL` → `DATABASE_URL`
- **Example**: `postgres://user:password@localhost:5432/aceryx?sslmode=disable`
- **Production note**: Use `sslmode=require` in production

### `ACERYX_DB_MAX_OPEN_CONNS`
- **Default**: `50`
- **Description**: Maximum number of open database connections
- **Tuning**: Increase for high-concurrency workloads; decrease if you run out of database connections

### `ACERYX_DB_MAX_IDLE_CONNS`
- **Default**: `25`
- **Description**: Maximum number of idle connections to keep open
- **Tuning**: Should be <= `ACERYX_DB_MAX_OPEN_CONNS`

### `ACERYX_DB_CONN_MAX_LIFETIME`
- **Default**: `1h`
- **Description**: Maximum lifetime of a database connection before it's closed and replaced
- **Format**: Go duration string (e.g., `30m`, `2h`, `0` for unlimited)

### `ACERYX_DB_CONN_MAX_IDLE_TIME`
- **Default**: `15m`
- **Description**: Maximum time an idle connection is kept before closing
- **Format**: Go duration string (e.g., `5m`, `30m`)

---

## Security Configuration

{{< callout type="warning" >}}
**CRITICAL**: In production, `ACERYX_JWT_SECRET` must be a strong, randomly generated secret. Use a password manager or `openssl rand -base64 32` to generate one. Committing secrets to version control is a serious security risk.
{{< /callout >}}

### `ACERYX_JWT_SECRET`
- **Default**: `test-secret` (development only)
- **Description**: Secret key used to sign and verify JWT tokens for user sessions and API authentication
- **Development**: Default is `test-secret`; fine for local dev
- **Production**: MUST be changed to a strong, randomly generated secret
- **Length**: At least 32 characters recommended
- **Example**: `openssl rand -base64 32` → `AbCdEf+/1234567890GhIjKlMnOpQrStUvWxYz==`
- **Security**: Change this key regularly in production; existing sessions will be invalidated

### `ACERYX_SESSION_TTL`
- **Default**: `24h`
- **Description**: How long a user session is valid before requiring re-login
- **Format**: Go duration string (e.g., `12h`, `7d`, `30m`)
- **Tuning**: Shorter TTL (e.g., `4h`) for high-security environments; longer for convenience

---

## LLM Configuration

These settings control integration with large language models for AI agent steps in workflows.

### `ACERYX_LLM_ENDPOINT`
- **Default**: `https://api.openai.com/v1`
- **Description**: The LLM provider's API endpoint
- **Examples**:
  - OpenAI: `https://api.openai.com/v1`
  - Azure OpenAI: `https://<your-resource>.openai.azure.com/`
  - Local Ollama: `http://localhost:11434/v1`

### `ACERYX_LLM_MODEL`
- **Default**: `gpt-4o`
- **Description**: The model to use for agent steps
- **Examples**: `gpt-4`, `gpt-4-turbo`, `claude-3-opus`, `llama2`

### `ACERYX_LLM_API_KEY`
- **Default**: (none)
- **Description**: API key for authentication with the LLM provider
- **Security**: Store in a secret manager, never commit to version control
- **Example**: `sk-...` (OpenAI), `sk-ant-...` (Anthropic)

### LLM Configuration Examples

**OpenAI GPT-4**
```bash
ACERYX_LLM_ENDPOINT=https://api.openai.com/v1
ACERYX_LLM_MODEL=gpt-4o
ACERYX_LLM_API_KEY=sk-...
```

**Azure OpenAI**
```bash
ACERYX_LLM_ENDPOINT=https://my-resource.openai.azure.com/
ACERYX_LLM_MODEL=gpt-4-deployment-name
ACERYX_LLM_API_KEY=<api-key>
```

**Local Ollama**
```bash
ACERYX_LLM_ENDPOINT=http://localhost:11434/v1
ACERYX_LLM_MODEL=llama2
# No API key needed for local
```

---

## Connector Configuration

Connector secrets (for integrations like Slack, GitHub, Jira, etc.) are configured via environment variables using the pattern:

### `ACERYX_SECRET_{KEY}`
- **Pattern**: `ACERYX_SECRET_` followed by the connector key (uppercase, dots → underscores)
- **Example**: For a Slack connector with key `slack.webhook`, set `ACERYX_SECRET_SLACK_WEBHOOK=https://hooks.slack.com/...`
- **Example**: For Jira with key `jira.api_token`, set `ACERYX_SECRET_JIRA_API_TOKEN=...`
- **Security**: Store in a secret manager, never commit to version control

---

## Vault Configuration

The vault stores encrypted documents and case attachments with cryptographic audit trails.

### `ACERYX_VAULT_ROOT`
- **Default**: `./data/vault`
- **Description**: Filesystem path where encrypted vault files are stored
- **Example**: `/var/lib/aceryx/vault`, `/mnt/secure/vault`
- **Note**: Ensure this directory is backed up and has restricted permissions (`700`)

### `ACERYX_VAULT_SIGNING_KEY`
- **Default**: Falls back to `ACERYX_JWT_SECRET` if not set
- **Description**: Master key for signing and verifying vault entries. If not provided, falls back to the JWT secret (good for development)
- **Production**: Set explicitly to ensure consistency across replicas and independent from JWT secret
- **Example**: Base64-encoded 32-byte key from `openssl rand -base64 32`

### `ACERYX_VAULT_CLEANUP_INTERVAL`
- **Default**: `24h`
- **Description**: How often the vault cleanup job runs to purge orphaned or deleted vault documents
- **Format**: Go duration string (e.g., `6h`, `12h`, `7d`)
- **Tuning**: Shorter intervals (e.g., `1h`) for high-churn environments; longer for low-volume

---

## Workflow Engine Configuration

These settings tune the case orchestration and task management engine.

### `ACERYX_MAX_CONCURRENT_STEPS`
- **Default**: `10`
- **Description**: Maximum number of workflow steps that can execute in parallel across the system
- **Tuning**:
  - Higher values (e.g., `50`) for high-throughput systems with many small steps
  - Lower values (e.g., `5`) if your database or integrations are bottlenecked
  - Increase if you see "queue full" errors in logs

### `ACERYX_SLA_INTERVAL`
- **Default**: `60s`
- **Description**: How often the SLA tracking job runs to check for breached deadlines and escalate tasks
- **Format**: Go duration string (e.g., `30s`, `5m`, `15m`)
- **Tuning**: Shorter intervals for strict SLA enforcement; longer for lower overhead

---

## Logging Configuration

### `ACERYX_LOG_LEVEL`
- **Default**: `info`
- **Description**: Minimum log level to output
- **Options**: `debug`, `info`, `warn`, `error`
- **Examples**:
  - `debug`: Verbose logging including SQL queries and request/response traces (development only)
  - `info`: Standard operational logging (recommended for production)
  - `warn`: Only warnings and errors
  - `error`: Only errors (not recommended; makes troubleshooting hard)

---

## Complete Example `.env` File

Here's a production-ready configuration:

```bash
# Server
ACERYX_HTTP_ADDR=:8080

# Database
ACERYX_DB_URL=postgres://aceryx:securepass@db.example.com:5432/aceryx?sslmode=require
ACERYX_DB_MAX_OPEN_CONNS=100
ACERYX_DB_MAX_IDLE_CONNS=50
ACERYX_DB_CONN_MAX_LIFETIME=1h
ACERYX_DB_CONN_MAX_IDLE_TIME=15m

# Security
ACERYX_JWT_SECRET=AbCdEf+/1234567890GhIjKlMnOpQrStUvWxYz==
ACERYX_SESSION_TTL=8h

# LLM
ACERYX_LLM_ENDPOINT=https://api.openai.com/v1
ACERYX_LLM_MODEL=gpt-4o
ACERYX_LLM_API_KEY=sk-...

# Vault
ACERYX_VAULT_ROOT=/var/lib/aceryx/vault
ACERYX_VAULT_SIGNING_KEY=VaultSigningKeyBase64EncodedHere==
ACERYX_VAULT_CLEANUP_INTERVAL=12h

# Engine
ACERYX_MAX_CONCURRENT_STEPS=25
ACERYX_SLA_INTERVAL=5m

# Logging
ACERYX_LOG_LEVEL=info
```

## Development vs. Production

**Development (`make serve`)**
```bash
ACERYX_DB_URL=postgres://localhost/aceryx?sslmode=disable
ACERYX_JWT_SECRET=dev-secret-not-secure
ACERYX_LOG_LEVEL=debug
ACERYX_LLM_API_KEY=sk-... (if testing LLM steps)
```

**Production (Kubernetes/Docker)**
```bash
ACERYX_DB_URL=postgres://user:${DB_PASSWORD}@db.example.com/aceryx?sslmode=require
ACERYX_JWT_SECRET=${JWT_SECRET_FROM_VAULT}
ACERYX_LOG_LEVEL=info
ACERYX_MAX_CONCURRENT_STEPS=50  # Higher for load
ACERYX_DB_MAX_OPEN_CONNS=200
ACERYX_LLM_API_KEY=${LLM_API_KEY_FROM_VAULT}
```

---

## Troubleshooting Configuration

**"database connection refused"**
- Check `ACERYX_DB_URL` points to a running Postgres instance
- Verify the user/password are correct
- Ensure `sslmode=disable` in development (or `require` if your DB enforces SSL)

**"JWT signature invalid"**
- Confirm `ACERYX_JWT_SECRET` is set and consistent across all instances
- If you change it, all existing sessions are invalidated

**"LLM request failed"**
- Verify `ACERYX_LLM_API_KEY` is valid and has quota
- Check `ACERYX_LLM_ENDPOINT` is reachable from your network
- Look at logs with `ACERYX_LOG_LEVEL=debug` for detailed error messages

**"too many database connections"**
- Increase `ACERYX_DB_MAX_OPEN_CONNS` and `ACERYX_DB_MAX_IDLE_CONNS`
- Check if your workload has legitimate spikes or a connection leak

For more help, see the [documentation](/docs) or open an issue on [GitHub](https://github.com/neural-chilli/aceryx).
