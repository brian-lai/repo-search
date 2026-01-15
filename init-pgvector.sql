-- Initialize pgvector extension for codetect
-- This script runs automatically when the Docker container starts

CREATE EXTENSION IF NOT EXISTS vector;

-- Verify extension is loaded
SELECT extname, extversion FROM pg_extension WHERE extname = 'vector';
