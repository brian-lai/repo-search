# Docker Setup for PostgreSQL + pgvector

This directory includes a `docker-compose.yml` file for running PostgreSQL with the pgvector extension.

## Quick Start

```bash
# Start PostgreSQL
docker-compose up -d postgres

# Check status
docker-compose ps

# View logs
docker-compose logs -f postgres

# Stop (preserves data)
docker-compose stop postgres

# Remove (deletes data)
docker-compose down -v
```

## Connection Details

- **Host:** `localhost`
- **Port:** `5432`
- **User:** `codetect`
- **Password:** `codetect`
- **Database:** `codetect`
- **DSN:** `postgres://codetect:codetect@localhost:5432/codetect?sslmode=disable`

## Configuration

### Custom Port

If port 5432 is already in use:

```bash
POSTGRES_PORT=5433 docker-compose up -d postgres
```

Then update your DSN:
```bash
export CODETECT_DB_DSN="postgres://codetect:codetect@localhost:5433/codetect?sslmode=disable"
```

### Environment Variables

Create a `.env` file in this directory:

```env
POSTGRES_PORT=5432
POSTGRES_USER=codetect
POSTGRES_PASSWORD=codetect
POSTGRES_DB=codetect
```

## Features

- **pgvector extension:** Automatically enabled via `init-pgvector.sql`
- **Data persistence:** Uses Docker volume `codetect-pgdata`
- **Health checks:** Monitors PostgreSQL readiness
- **Auto-restart:** Restarts automatically unless stopped

## Verify Setup

Test the connection:

```bash
psql "postgres://codetect:codetect@localhost:5432/codetect?sslmode=disable" -c "SELECT extname, extversion FROM pg_extension WHERE extname='vector';"
```

Expected output:
```
 extname | extversion
---------+------------
 vector  | 0.8.1
(1 row)
```

## Troubleshooting

### Port already in use

Check what's using port 5432:
```bash
lsof -i :5432
```

Use a different port:
```bash
POSTGRES_PORT=5433 docker-compose up -d postgres
```

### Container won't start

Check logs:
```bash
docker-compose logs postgres
```

Remove and recreate:
```bash
docker-compose down -v
docker-compose up -d postgres
```

### Can't connect

Ensure container is running:
```bash
docker-compose ps
```

Check network connectivity:
```bash
docker-compose exec postgres pg_isready -U codetect
```

## Data Management

### Backup

```bash
docker-compose exec -T postgres pg_dump -U codetect codetect > backup.sql
```

### Restore

```bash
cat backup.sql | docker-compose exec -T postgres psql -U codetect codetect
```

### Reset (delete all data)

```bash
docker-compose down -v
docker-compose up -d postgres
```

## Using with codetect

After starting the container, configure codetect:

```bash
export CODETECT_DB_TYPE="postgres"
export CODETECT_DB_DSN="postgres://codetect:codetect@localhost:5432/codetect?sslmode=disable"

# Generate embeddings
codetect embed
```

Or add to your shell profile:
```bash
echo 'export CODETECT_DB_TYPE="postgres"' >> ~/.zshrc
echo 'export CODETECT_DB_DSN="postgres://codetect:codetect@localhost:5432/codetect?sslmode=disable"' >> ~/.zshrc
source ~/.zshrc
```
