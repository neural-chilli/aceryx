---
title: Installation
weight: 2
---

This guide covers setting up Aceryx from source for development and local testing.

## Prerequisites

Before you begin, ensure you have:

- **Go 1.26+** — [Download](https://go.dev/dl)
- **Node 20+** — [Download](https://nodejs.org) (for frontend build)
- **PostgreSQL 17** with the `pgvector` extension — [Download](https://www.postgresql.org/download) or use Docker
- **Docker & Docker Compose** (optional, recommended for local Postgres)
- **Git** — for cloning the repository

### Checking Your Versions

```bash
go version      # Should output go version 1.26 or higher
node --version  # Should output v20.x.x or higher
psql --version  # Should output PostgreSQL 17.x or higher
docker --version
```

## Clone the Repository

```bash
git clone https://github.com/neural-chilli/aceryx.git
cd aceryx
```

## Database Setup

### Option 1: Docker Compose (Recommended for Development)

If you have Docker installed, the quickest way is:

```bash
docker compose up -d
```

This starts a PostgreSQL 17 container with pgvector (using `pgvector/pgvector:pg17` image) pre-installed, listening on `localhost:5432`. The database name is `aceryx`, with user `aceryx` and password `aceryx`.

Verify the container is running:

```bash
docker compose ps
```

### Option 2: Manual PostgreSQL Installation

If you're installing PostgreSQL locally:

1. Start the PostgreSQL service (system-dependent)
2. Create a database named `aceryx`:

```bash
createdb -U postgres aceryx
```

3. Install the `pgvector` extension:

```bash
psql -U postgres -d aceryx -c "CREATE EXTENSION IF NOT EXISTS vector;"
```

4. Verify the connection string works:

```bash
psql -U postgres -d aceryx -c "SELECT version();"
```

## Run Database Migrations

The `migrate` command applies all pending schema migrations to your database:

```bash
go run ./cmd/aceryx migrate
```

You should see output indicating schema tables were created (case_types, workflows, cases, tasks, etc.). If no output appears, check that your `DATABASE_URL` environment variable is set correctly.

{{< callout type="info" >}}
By default, the CLI looks for a PostgreSQL connection string in the `DATABASE_URL` environment variable. If you're using the Docker setup above, you can set:
```bash
export DATABASE_URL="postgres://aceryx:aceryx@localhost:5432/aceryx?sslmode=disable"
```
{{< /callout >}}

## Seed Development Data (Optional)

To populate the database with example data (an admin user, sample case type, and workflow), run:

```bash
go run ./cmd/aceryx seed
```

This creates:
- **Admin user**: `admin@localhost` with password `admin` (default dev credentials)
- **Example case type**: "Support Ticket"
- **Example workflow**: A simple approval workflow

You can now log in to the web UI with these credentials.

{{< callout type="warning" >}}
The seed credentials are for **development only**. Change the admin password immediately in production, or remove this account entirely.
{{< /callout >}}

## Build from Source

To compile Aceryx from source, use the provided Makefile:

```bash
make build
```

This:
1. Installs frontend dependencies and builds the Vue app
2. Embeds the frontend into the Go binary
3. Compiles the Go code
4. Produces a single binary at `./bin/aceryx`

You can then run the server:

```bash
./bin/aceryx serve
```

The server listens on `http://localhost:8080` by default.

{{< callout type="info" >}}
If you don't have `make` installed, you can manually run the build steps:

```bash
# Build frontend
cd web && npm install && npm run build && cd ..

# Build Go binary
go build -o ./bin/aceryx ./cmd/aceryx
```
{{< /callout >}}

## Docker Deployment

For production or containerized deployments, a multi-stage Dockerfile is included:

```dockerfile
# Stage 1: Build frontend
FROM node:20 AS frontend-build
WORKDIR /app
COPY web ./web
RUN cd web && npm install && npm run build

# Stage 2: Build Go binary
FROM golang:1.26 AS go-build
WORKDIR /app
COPY . .
COPY --from=frontend-build /app/web/dist ./web/dist
RUN go build -o aceryx ./cmd/aceryx

# Stage 3: Runtime
FROM alpine:latest
WORKDIR /app
COPY --from=go-build /app/aceryx ./
EXPOSE 8080
CMD ["./aceryx", "serve"]
```

Build and run:

```bash
docker build -t aceryx:latest .
docker run -p 8080:8080 \
  -e DATABASE_URL="postgres://user:pass@db:5432/aceryx" \
  aceryx:latest
```

## Alternative: Using the qp Task Runner

If you prefer a task runner, the project includes `qp` (quick project) configuration:

```bash
# Install qp if you don't have it
go install github.com/neural-chilli/qp@latest

# Fresh database (drop + migrate + seed in one command)
qp db:fresh

# Build all (frontend + Go binary)
qp build

# Start development servers (backend + frontend in parallel)
qp dev

# Full test suite
qp test:all

# Verify feature isolation
qp check:imports
```

The `qp db:fresh` command is handy for resetting your development database to a clean state. You can also use the `bootstrap.sh` script to scaffold the entire project in one go.

## Verify Installation

Once everything is set up, start the development server:

```bash
go run ./cmd/aceryx serve
```

You should see:

```
Starting Aceryx server on :8080
```

Open your browser to `http://localhost:8080` and log in with:
- **Email**: `admin@localhost`
- **Password**: `admin`

If seeding was skipped, create your first user via the setup wizard in the UI.

## Next Steps

Now that Aceryx is running, check out the [Quick Start](/docs/getting-started/quick-start) guide to create your first workflow.
