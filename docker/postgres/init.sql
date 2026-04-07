-- LedgerAlps — PostgreSQL initialization
-- Extensions requises pour la conformité nLPD (chiffrement)

CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Activer le suivi des modifications pour la traçabilité CO art. 957
-- L'audit log est géré au niveau applicatif (voir audit_log table)

-- Paramètres de performance
ALTER SYSTEM SET shared_buffers = '256MB';
ALTER SYSTEM SET effective_cache_size = '1GB';
ALTER SYSTEM SET maintenance_work_mem = '128MB';
ALTER SYSTEM SET checkpoint_completion_target = 0.9;
ALTER SYSTEM SET wal_buffers = '16MB';
ALTER SYSTEM SET default_statistics_target = 100;
