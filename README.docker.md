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
- **User:** `repo_search`
- **Password:** `repo_search`
- **Database:** `repo_search`
- **DSN:** `postgres://repo_search:repo_search@localhost:5432/repo_search?sslmode=disable`

## Configuration

### Custom Port

If port 5432 is already in use:

```bash
POSTGRES_PORT=5433 docker-compose up -d postgres
```

Then update your DSN:
```bash
export REPO_SEARCH_DB_DSN="postgres://repo_search:repo_search@localhost:5433/repo_search?sslmode=disable"
```

### Environment Variables

Create a `.env` file in this directory:

```env
POSTGRES_PORT=5432
POSTGRES_USER=repo_search
POSTGRES_PASSWORD=repo_search
POSTGRES_DB=repo_search
```

## Features

- **pgvector extension:** Automatically enabled via `init-pgvector.sql`
- **Data persistence:** Uses Docker volume `repo-search-pgdata`
- **Health checks:** Monitors PostgreSQL readiness
- **Auto-restart:** Restarts automatically unless stopped

## Verify Setup

Test the connection:

```bash
psql "postgres://repo_search:repo_search@localhost:5432/repo_search?sslmode=disable" -c "SELECT extname, extversion FROM pg_extension WHERE extname='vector';"
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
docker-compose exec postgres pg_isready -U repo_search
```

## Data Management

### Backup

```bash
docker-compose exec -T postgres pg_dump -U repo_search repo_search > backup.sql
```

### Restore

```bash
cat backup.sql | docker-compose exec -T postgres psql -U repo_search repo_search
```

### Reset (delete all data)

```bash
docker-compose down -v
docker-compose up -d postgres
```

## Using with repo-search

After starting the container, configure repo-search:

```bash
export REPO_SEARCH_DB_TYPE="postgres"
export REPO_SEARCH_DB_DSN="postgres://repo_search:repo_search@localhost:5432/repo_search?sslmode=disable"

# Generate embeddings
repo-search embed
```

Or add to your shell profile:
```bash
echo 'export REPO_SEARCH_DB_TYPE="postgres"' >> ~/.zshrc
echo 'export REPO_SEARCH_DB_DSN="postgres://repo_search:repo_search@localhost:5432/repo_search?sslmode=disable"' >> ~/.zshrc
source ~/.zshrc
```
