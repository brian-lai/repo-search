# PostgreSQL + pgvector Setup Guide

This guide covers setting up PostgreSQL with pgvector for scalable vector similarity search in codetect.

## Table of Contents

- [Why PostgreSQL + pgvector?](#why-postgresql--pgvector)
- [Performance Comparison](#performance-comparison)
- [Quick Start with Docker](#quick-start-with-docker)
- [Manual Installation](#manual-installation)
- [Configuration](#configuration)
- [Migration from SQLite](#migration-from-sqlite)
- [Troubleshooting](#troubleshooting)

## Why PostgreSQL + pgvector?

By default, codetect uses SQLite with brute-force vector search. This works well for small to medium codebases (< 10,000 files), but has performance limitations at scale.

PostgreSQL + pgvector provides:

- **Scalability**: Efficient HNSW indexing for 100K+ vectors
- **Performance**: 60x faster search on large datasets (see benchmarks below)
- **Persistence**: Separate database server with better reliability
- **Flexibility**: Advanced querying and management capabilities

**When to use PostgreSQL:**
- Large codebases (> 10,000 files)
- Multiple projects sharing embeddings
- Production deployments requiring high performance
- Teams needing centralized search infrastructure

**When SQLite is sufficient:**
- Small to medium projects (< 10,000 files)
- Personal use / single developer
- Quick prototyping
- No embedding server available

## Performance Comparison

Real-world benchmarks on Apple M3 Pro (2024):

| Dataset Size | SQLite (brute-force) | PostgreSQL (pgvector) | Speedup |
|--------------|----------------------|-----------------------|---------|
| 100 vectors  | 77 μs                | 603 μs                | 0.13x (slower) |
| 1,000 vectors | 1.19 ms             | 745 μs                | 1.6x faster |
| 10,000 vectors | 58.1 ms            | 963 μs                | **60x faster** |

**Key Insights:**
- For small datasets (< 1,000 vectors), brute-force SQLite is faster due to lower overhead
- pgvector with HNSW indexing scales logarithmically: O(log n) vs O(n) for brute-force
- At 10,000+ vectors, PostgreSQL provides massive performance improvements
- pgvector enables sub-millisecond search even on 100K+ vector datasets

## Quick Start with Docker

The easiest way to get PostgreSQL + pgvector running:

### 1. Start PostgreSQL

```bash
cd /path/to/codetect
docker-compose up -d
```

This starts PostgreSQL 16 with pgvector extension pre-installed.

**Container details:**
- Image: `pgvector/pgvector:pg16`
- Port: `5432` (configurable via `POSTGRES_PORT` env var)
- User: `codetect`
- Password: `codetect`
- Database: `codetect`
- Volume: `codetect-pgdata` (persistent storage)

### 2. Configure codetect

Set environment variables:

```bash
export CODETECT_DB_TYPE=postgres
export CODETECT_DB_DSN="postgres://codetect:codetect@localhost:5432/codetect?sslmode=disable"
```

Add to your `~/.bashrc`, `~/.zshrc`, or shell profile for persistence.

### 3. Initialize and embed

```bash
cd /path/to/your/project
codetect index    # Index symbols
codetect embed    # Generate embeddings (stored in PostgreSQL)
```

### 4. Verify

```bash
# Check PostgreSQL connection
docker-compose ps

# View embeddings
docker-compose exec postgres psql -U codetect -c "SELECT COUNT(*) FROM embeddings;"
```

That's it! codetect will now use PostgreSQL for all vector operations.

## Manual Installation

If you prefer to install PostgreSQL manually without Docker:

### macOS (Homebrew)

```bash
# Install PostgreSQL
brew install postgresql@16

# Start PostgreSQL
brew services start postgresql@16

# Install pgvector
brew install pgvector

# Create database and enable extension
createdb codetect
psql codetect -c "CREATE EXTENSION vector;"
```

### Ubuntu/Debian

```bash
# Add PostgreSQL repository
sudo sh -c 'echo "deb http://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list'
wget --quiet -O - https://www.postgresql.org/media/keys/ACCC4CF8.asc | sudo apt-key add -
sudo apt-get update

# Install PostgreSQL 16
sudo apt-get install -y postgresql-16 postgresql-client-16

# Install pgvector
sudo apt-get install -y postgresql-16-pgvector

# Start PostgreSQL
sudo systemctl start postgresql
sudo systemctl enable postgresql

# Create database and user
sudo -u postgres psql -c "CREATE USER codetect WITH PASSWORD 'codetect';"
sudo -u postgres psql -c "CREATE DATABASE codetect OWNER codetect;"
sudo -u postgres psql codetect -c "CREATE EXTENSION vector;"
```

### Verify Installation

```bash
# Check pgvector version
psql codetect -c "SELECT extname, extversion FROM pg_extension WHERE extname = 'vector';"

# Expected output:
#  extname | extversion
# ---------+------------
#  vector  | 0.7.0
```

## Configuration

codetect supports flexible PostgreSQL configuration via environment variables.

### Environment Variables

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `CODETECT_DB_TYPE` | Database type (`sqlite` or `postgres`) | `sqlite` | `postgres` |
| `CODETECT_DB_DSN` | PostgreSQL connection string | - | `postgres://user:pass@host:5432/dbname` |
| `CODETECT_DB_PATH` | SQLite database path (if using SQLite) | `.codetect/symbols.db` | `/custom/path.db` |
| `CODETECT_VECTOR_DIMENSIONS` | Embedding vector size | `768` | `1536` |

### DSN Format

PostgreSQL connection string format:

```
postgres://[user]:[password]@[host]:[port]/[database]?[options]
```

**Common examples:**

```bash
# Local PostgreSQL
export CODETECT_DB_DSN="postgres://codetect:codetect@localhost:5432/codetect?sslmode=disable"

# Docker (custom port)
export CODETECT_DB_DSN="postgres://codetect:codetect@localhost:5465/codetect?sslmode=disable"

# Remote PostgreSQL with SSL
export CODETECT_DB_DSN="postgres://user:pass@db.example.com:5432/codetect?sslmode=require"

# Connection pooling
export CODETECT_DB_DSN="postgres://user:pass@localhost:5432/codetect?pool_max_conns=20"
```

### Auto-Detection

codetect automatically detects PostgreSQL from the DSN:

```bash
# Explicitly set type
export CODETECT_DB_TYPE=postgres
export CODETECT_DB_DSN="postgres://..."

# Auto-detect from DSN (type not required)
export CODETECT_DB_DSN="postgres://..."  # Type inferred automatically
```

### Vector Dimensions

Different embedding models use different dimensions:

| Model | Dimensions | Setting |
|-------|------------|---------|
| `nomic-embed-text` (default) | 768 | `CODETECT_VECTOR_DIMENSIONS=768` |
| OpenAI `text-embedding-3-small` | 1536 | `CODETECT_VECTOR_DIMENSIONS=1536` |
| OpenAI `text-embedding-3-large` | 3072 | `CODETECT_VECTOR_DIMENSIONS=3072` |

**Important:** Set dimensions before running `codetect embed`. Changing dimensions requires re-embedding.

### Shell Configuration

Add to your shell profile for persistence:

**Bash (`~/.bashrc`):**
```bash
export CODETECT_DB_TYPE=postgres
export CODETECT_DB_DSN="postgres://codetect:codetect@localhost:5432/codetect?sslmode=disable"
```

**Zsh (`~/.zshrc`):**
```zsh
export CODETECT_DB_TYPE=postgres
export CODETECT_DB_DSN="postgres://codetect:codetect@localhost:5432/codetect?sslmode=disable"
```

**Fish (`~/.config/fish/config.fish`):**
```fish
set -x CODETECT_DB_TYPE postgres
set -x CODETECT_DB_DSN "postgres://codetect:codetect@localhost:5432/codetect?sslmode=disable"
```

## Migration from SQLite

To migrate existing embeddings from SQLite to PostgreSQL, use the migration script:

```bash
# Start PostgreSQL
docker-compose up -d

# Set PostgreSQL configuration
export CODETECT_DB_TYPE=postgres
export CODETECT_DB_DSN="postgres://codetect:codetect@localhost:5432/codetect?sslmode=disable"

# Run migration
codetect migrate-to-postgres
```

The migration script will:
1. Read embeddings from SQLite (`.codetect/symbols.db`)
2. Create embeddings table in PostgreSQL
3. Copy all vectors with metadata
4. Create pgvector index for fast search
5. Verify data integrity

**Migration Performance:**
- ~1,000 vectors/sec on typical hardware
- 10,000 vectors: ~10 seconds
- 100,000 vectors: ~2 minutes

**After migration:**
- SQLite database is preserved (not deleted)
- You can switch back by removing PostgreSQL env vars
- No need to re-run `codetect embed`

See the [Migration Script](#migration-script) section below for advanced options.

## Troubleshooting

### Connection Issues

**Error: `connection refused`**

```bash
# Check if PostgreSQL is running
docker-compose ps

# If not running, start it
docker-compose up -d

# Check logs
docker-compose logs postgres
```

**Error: `password authentication failed`**

```bash
# Verify credentials match docker-compose.yml
docker-compose exec postgres psql -U codetect -c "SELECT version();"

# Reset password if needed
docker-compose exec postgres psql -U postgres -c "ALTER USER codetect PASSWORD 'codetect';"
```

### pgvector Issues

**Error: `extension "vector" does not exist`**

```bash
# Verify pgvector is installed
docker-compose exec postgres psql -U codetect -c "SELECT * FROM pg_available_extensions WHERE name = 'vector';"

# Create extension
docker-compose exec postgres psql -U codetect -c "CREATE EXTENSION vector;"
```

**Error: `could not open extension control file`**

Your PostgreSQL image doesn't include pgvector. Use the official pgvector image:

```yaml
# docker-compose.yml
services:
  postgres:
    image: pgvector/pgvector:pg16  # Use this image
```

### Performance Issues

**Slow searches after migration:**

```bash
# Verify index exists
docker-compose exec postgres psql -U codetect -c "\d embeddings"

# Look for: "idx_embeddings_embedding" hnsw (embedding vector_cosine_ops)

# If missing, create index
docker-compose exec postgres psql -U codetect -c "
  CREATE INDEX idx_embeddings_embedding
  ON embeddings
  USING hnsw (embedding vector_cosine_ops);
"
```

**Index creation taking too long:**

For large datasets (100K+ vectors), index creation can take 10-30 minutes. Monitor progress:

```bash
# Check PostgreSQL activity
docker-compose exec postgres psql -U codetect -c "
  SELECT pid, state, query
  FROM pg_stat_activity
  WHERE state != 'idle';
"
```

### Data Issues

**Empty search results:**

```bash
# Check embedding count
docker-compose exec postgres psql -U codetect -c "SELECT COUNT(*) FROM embeddings;"

# If 0, re-run embedding
codetect embed
```

**Dimension mismatch error:**

```bash
# Check actual dimensions in database
docker-compose exec postgres psql -U codetect -c "
  SELECT vector_dims(embedding)
  FROM embeddings
  LIMIT 1;
"

# Must match CODETECT_VECTOR_DIMENSIONS
# If mismatch, drop table and re-embed
docker-compose exec postgres psql -U codetect -c "DROP TABLE IF EXISTS embeddings;"
codetect embed
```

### Docker Issues

**Port conflict:**

```bash
# Change port in docker-compose
export POSTGRES_PORT=5465
docker-compose up -d

# Update DSN
export CODETECT_DB_DSN="postgres://codetect:codetect@localhost:5465/codetect?sslmode=disable"
```

**Volume persistence:**

```bash
# List volumes
docker volume ls | grep codetect

# Backup volume
docker run --rm -v codetect-pgdata:/data -v $(pwd):/backup ubuntu tar czf /backup/pgdata-backup.tar.gz /data

# Restore volume
docker run --rm -v codetect-pgdata:/data -v $(pwd):/backup ubuntu tar xzf /backup/pgdata-backup.tar.gz -C /
```

**Clean restart:**

```bash
# Stop and remove container
docker-compose down

# Remove volume (WARNING: deletes all data)
docker volume rm codetect-pgdata

# Fresh start
docker-compose up -d
codetect embed  # Re-embed
```

### Getting Help

If issues persist:

1. Check PostgreSQL logs: `docker-compose logs postgres`
2. Verify configuration: `echo $CODETECT_DB_DSN`
3. Test connection: `docker-compose exec postgres psql -U codetect`
4. Open an issue: https://github.com/brian-lai/codetect/issues

Include:
- PostgreSQL version: `docker-compose exec postgres psql -U postgres -c "SELECT version();"`
- pgvector version: `docker-compose exec postgres psql -U codetect -c "SELECT extversion FROM pg_extension WHERE extname = 'vector';"`
- Error messages from logs
- Operating system and architecture

## Advanced Configuration

### Custom PostgreSQL Configuration

Mount custom `postgresql.conf`:

```yaml
# docker-compose.yml
services:
  postgres:
    volumes:
      - ./postgresql.conf:/etc/postgresql/postgresql.conf
    command: postgres -c config_file=/etc/postgresql/postgresql.conf
```

### Connection Pooling

For high-concurrency scenarios, use PgBouncer:

```yaml
# docker-compose.yml
services:
  pgbouncer:
    image: pgbouncer/pgbouncer
    environment:
      DATABASES_HOST: postgres
      DATABASES_PORT: 5432
      DATABASES_DBNAME: codetect
      PGBOUNCER_AUTH_TYPE: plain
    ports:
      - "6432:5432"
```

```bash
# Connect through PgBouncer
export CODETECT_DB_DSN="postgres://codetect:codetect@localhost:6432/codetect"
```

### Multiple Projects

Share PostgreSQL across multiple projects using different schemas:

```bash
# Project 1
export CODETECT_DB_DSN="postgres://codetect:codetect@localhost:5432/codetect?search_path=project1"

# Project 2
export CODETECT_DB_DSN="postgres://codetect:codetect@localhost:5432/codetect?search_path=project2"
```

Or use separate databases:

```bash
# Create additional databases
docker-compose exec postgres psql -U postgres -c "CREATE DATABASE project2 OWNER codetect;"
docker-compose exec postgres psql -U codetect -d project2 -c "CREATE EXTENSION vector;"
```

## Next Steps

- [Migration Script Documentation](../scripts/migrate-to-postgres) - Detailed migration guide
- [Configuration Reference](./installation.md#configuration) - All environment variables
- [Performance Tuning](./architecture.md#performance) - Optimization tips
- [Backup and Recovery](./installation.md#backup) - Data protection strategies
