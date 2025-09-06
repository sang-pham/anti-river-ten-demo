-- Create DEMO schema if not exists
CREATE SCHEMA IF NOT EXISTS "DEMO";

-- Create table to store parsed logsql.log records
CREATE TABLE IF NOT EXISTS "DEMO"."SQL_LOG" (
    id            BIGSERIAL PRIMARY KEY,
    db_name       TEXT NOT NULL,
    sql_query     TEXT NOT NULL,
    exec_time_ms  BIGINT NOT NULL,
    exec_count    BIGINT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Helpful indexes for filtering and sorting
CREATE INDEX IF NOT EXISTS idx_sql_log_db_name ON "DEMO"."SQL_LOG"(db_name);
CREATE INDEX IF NOT EXISTS idx_sql_log_db_exec_time ON "DEMO"."SQL_LOG"(db_name, exec_time_ms DESC);
-- New indexes to optimize time-window filtering and per-DB scans
CREATE INDEX IF NOT EXISTS idx_sql_log_created_at ON "DEMO"."SQL_LOG"(created_at);
CREATE INDEX IF NOT EXISTS idx_sql_log_db_created_at ON "DEMO"."SQL_LOG"(db_name, created_at);
