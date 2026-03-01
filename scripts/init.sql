-- ============================================================================
-- Distributed System Telemetry & Remote Command Platform
-- Database Schema
-- ============================================================================

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ============================================================================
-- Admin users table (for JWT authentication)
-- ============================================================================
CREATE TABLE IF NOT EXISTS admin_users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(100) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================================
-- Agents table
-- ============================================================================
CREATE TABLE IF NOT EXISTS agents (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id VARCHAR(128) UNIQUE NOT NULL,
    hostname VARCHAR(255) NOT NULL,
    ip_address VARCHAR(45),
    os_type VARCHAR(100),
    status VARCHAR(20) DEFAULT 'online' CHECK (status IN ('online', 'offline')),
    last_heartbeat TIMESTAMPTZ DEFAULT NOW(),
    registered_at TIMESTAMPTZ DEFAULT NOW(),
    metadata JSONB DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_agents_agent_id ON agents(agent_id);
CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
CREATE INDEX IF NOT EXISTS idx_agents_last_heartbeat ON agents(last_heartbeat);

-- ============================================================================
-- Telemetry table
-- ============================================================================
CREATE TABLE IF NOT EXISTS telemetry (
    id BIGSERIAL PRIMARY KEY,
    agent_id VARCHAR(128) NOT NULL REFERENCES agents(agent_id) ON DELETE CASCADE,
    cpu_usage DECIMAL(5,2) NOT NULL,
    memory_usage DECIMAL(5,2) NOT NULL,
    memory_total BIGINT,
    memory_used BIGINT,
    disk_usage DECIMAL(5,2),
    disk_total BIGINT,
    disk_used BIGINT,
    uptime_seconds BIGINT,
    load_avg_1 DECIMAL(6,2),
    load_avg_5 DECIMAL(6,2),
    load_avg_15 DECIMAL(6,2),
    recorded_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_telemetry_agent_id ON telemetry(agent_id);
CREATE INDEX IF NOT EXISTS idx_telemetry_recorded_at ON telemetry(recorded_at);
CREATE INDEX IF NOT EXISTS idx_telemetry_agent_recorded ON telemetry(agent_id, recorded_at DESC);

-- ============================================================================
-- Commands table
-- Status lifecycle: pending → sent → (success | failed)
-- ============================================================================
CREATE TABLE IF NOT EXISTS commands (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id VARCHAR(128) NOT NULL REFERENCES agents(agent_id) ON DELETE CASCADE,
    command_type VARCHAR(50) NOT NULL CHECK (command_type IN ('restart_agent', 'collect_logs', 'simulate_load', 'ping')),
    payload JSONB DEFAULT '{}',
    status VARCHAR(20) DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'success', 'failed')),
    result TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    executed_at TIMESTAMPTZ,
    created_by UUID REFERENCES admin_users(id)
);

CREATE INDEX IF NOT EXISTS idx_commands_agent_id ON commands(agent_id);
CREATE INDEX IF NOT EXISTS idx_commands_status ON commands(status);
CREATE INDEX IF NOT EXISTS idx_commands_agent_pending ON commands(agent_id, status) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_commands_created_at ON commands(created_at);

-- ============================================================================
-- Aggregated stats table (populated by worker)
-- ============================================================================
CREATE TABLE IF NOT EXISTS telemetry_aggregates (
    id BIGSERIAL PRIMARY KEY,
    agent_id VARCHAR(128) NOT NULL,
    aggregation_period VARCHAR(20) NOT NULL CHECK (aggregation_period IN ('hour', 'day')),
    period_start TIMESTAMPTZ NOT NULL,
    avg_cpu DECIMAL(5,2),
    max_cpu DECIMAL(5,2),
    avg_memory DECIMAL(5,2),
    max_memory DECIMAL(5,2),
    avg_load DECIMAL(6,2),
    max_load DECIMAL(6,2),
    sample_count INTEGER,
    computed_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_aggregates_agent_period
    ON telemetry_aggregates(agent_id, aggregation_period, period_start);

-- ============================================================================
-- Alerts table (populated by worker)
-- ============================================================================
CREATE TABLE IF NOT EXISTS alerts (
    id BIGSERIAL PRIMARY KEY,
    agent_id VARCHAR(128) NOT NULL,
    alert_type VARCHAR(50) NOT NULL CHECK (alert_type IN ('cpu_high', 'memory_high', 'load_high')),
    severity VARCHAR(20) NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    message TEXT NOT NULL,
    threshold_value DECIMAL(10,2),
    actual_value DECIMAL(10,2),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    acknowledged BOOLEAN DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_alerts_agent_id ON alerts(agent_id);
CREATE INDEX IF NOT EXISTS idx_alerts_created_at ON alerts(created_at);
CREATE INDEX IF NOT EXISTS idx_alerts_severity ON alerts(severity);
CREATE INDEX IF NOT EXISTS idx_alerts_unacked ON alerts(acknowledged) WHERE acknowledged = FALSE;

-- ============================================================================
-- Updated_at trigger for admin_users
-- ============================================================================
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS admin_users_updated_at ON admin_users;
CREATE TRIGGER admin_users_updated_at
    BEFORE UPDATE ON admin_users
    FOR EACH ROW EXECUTE PROCEDURE update_updated_at();
