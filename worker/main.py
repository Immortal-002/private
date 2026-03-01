#!/usr/bin/env python3
"""
Telemetry Background Worker
- Analyzes telemetry data
- Computes hourly & daily aggregates (avg CPU, max load)
- Detects threshold violations and creates alerts (with dedup)
- Cleans old telemetry data
- Marks stale agents as offline
- Generates periodic summary logs
"""

import os
import sys
import time
import logging
from datetime import datetime, timedelta

import psycopg2
from psycopg2.extras import RealDictCursor
import redis

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S",
)
logger = logging.getLogger("worker")

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
DB_CONFIG = {
    "host": os.getenv("DB_HOST", "localhost"),
    "port": int(os.getenv("DB_PORT", 5432)),
    "user": os.getenv("DB_USER", "telemetry"),
    "password": os.getenv("DB_PASSWORD", "telemetry_secret_change_me"),
    "dbname": os.getenv("DB_NAME", "telemetry"),
}
REDIS_CONFIG = {
    "host": os.getenv("REDIS_ADDR", "localhost:6379").split(":")[0],
    "port": int(os.getenv("REDIS_ADDR", "localhost:6379").split(":")[-1]),
    "password": os.getenv("REDIS_PASSWORD", "redis_secret_change_me") or None,
    "db": 0,
}
INTERVAL_SECONDS = int(os.getenv("WORKER_INTERVAL", 30))
RETENTION_DAYS = int(os.getenv("TELEMETRY_RETENTION_DAYS", 7))
CPU_THRESHOLD = float(os.getenv("CPU_ALERT_THRESHOLD", 90))
MEMORY_THRESHOLD = float(os.getenv("MEMORY_ALERT_THRESHOLD", 90))
LOAD_THRESHOLD = float(os.getenv("LOAD_ALERT_THRESHOLD", 4.0))
ALERT_COOLDOWN_MINUTES = int(os.getenv("ALERT_COOLDOWN_MINUTES", 10))


def get_db_conn():
    """Create a new database connection."""
    conn = psycopg2.connect(**DB_CONFIG)
    conn.autocommit = False
    return conn


def get_redis():
    """Create a new Redis connection."""
    return redis.Redis(
        host=REDIS_CONFIG["host"],
        port=REDIS_CONFIG["port"],
        password=REDIS_CONFIG["password"],
        db=REDIS_CONFIG["db"],
        decode_responses=True,
    )


# ---------------------------------------------------------------------------
# Aggregation
# ---------------------------------------------------------------------------
def compute_aggregates(conn):
    """Compute hourly aggregates for all agents (upsert)."""
    try:
        with conn.cursor(cursor_factory=RealDictCursor) as cur:
            cur.execute("""
                INSERT INTO telemetry_aggregates (
                    agent_id, aggregation_period, period_start,
                    avg_cpu, max_cpu, avg_memory, max_memory,
                    avg_load, max_load, sample_count
                )
                SELECT
                    agent_id,
                    'hour',
                    date_trunc('hour', recorded_at) AS period_start,
                    ROUND(AVG(cpu_usage)::numeric, 2),
                    ROUND(MAX(cpu_usage)::numeric, 2),
                    ROUND(AVG(memory_usage)::numeric, 2),
                    ROUND(MAX(memory_usage)::numeric, 2),
                    ROUND(AVG(COALESCE(load_avg_1, 0))::numeric, 2),
                    ROUND(MAX(COALESCE(load_avg_1, 0))::numeric, 2),
                    COUNT(*)
                FROM telemetry
                WHERE recorded_at >= NOW() - INTERVAL '2 hours'
                GROUP BY agent_id, date_trunc('hour', recorded_at)
                ON CONFLICT (agent_id, aggregation_period, period_start)
                DO UPDATE SET
                    avg_cpu = EXCLUDED.avg_cpu,
                    max_cpu = EXCLUDED.max_cpu,
                    avg_memory = EXCLUDED.avg_memory,
                    max_memory = EXCLUDED.max_memory,
                    avg_load = EXCLUDED.avg_load,
                    max_load = EXCLUDED.max_load,
                    sample_count = EXCLUDED.sample_count,
                    computed_at = NOW()
            """)
        conn.commit()
        logger.info("Computed hourly aggregates successfully")
    except Exception as e:
        conn.rollback()
        logger.error("Failed to compute aggregates: %s", e)


def compute_daily_aggregates(conn):
    """Compute daily aggregates for all agents (upsert)."""
    try:
        with conn.cursor(cursor_factory=RealDictCursor) as cur:
            cur.execute("""
                INSERT INTO telemetry_aggregates (
                    agent_id, aggregation_period, period_start,
                    avg_cpu, max_cpu, avg_memory, max_memory,
                    avg_load, max_load, sample_count
                )
                SELECT
                    agent_id,
                    'day',
                    date_trunc('day', recorded_at) AS period_start,
                    ROUND(AVG(cpu_usage)::numeric, 2),
                    ROUND(MAX(cpu_usage)::numeric, 2),
                    ROUND(AVG(memory_usage)::numeric, 2),
                    ROUND(MAX(memory_usage)::numeric, 2),
                    ROUND(AVG(COALESCE(load_avg_1, 0))::numeric, 2),
                    ROUND(MAX(COALESCE(load_avg_1, 0))::numeric, 2),
                    COUNT(*)
                FROM telemetry
                WHERE recorded_at >= NOW() - INTERVAL '2 days'
                GROUP BY agent_id, date_trunc('day', recorded_at)
                ON CONFLICT (agent_id, aggregation_period, period_start)
                DO UPDATE SET
                    avg_cpu = EXCLUDED.avg_cpu,
                    max_cpu = EXCLUDED.max_cpu,
                    avg_memory = EXCLUDED.avg_memory,
                    max_memory = EXCLUDED.max_memory,
                    avg_load = EXCLUDED.avg_load,
                    max_load = EXCLUDED.max_load,
                    sample_count = EXCLUDED.sample_count,
                    computed_at = NOW()
            """)
        conn.commit()
        logger.info("Computed daily aggregates successfully")
    except Exception as e:
        conn.rollback()
        logger.error("Failed to compute daily aggregates: %s", e)


# ---------------------------------------------------------------------------
# Threshold detection (with dedup)
# ---------------------------------------------------------------------------
def _alert_exists_recently(cur, agent_id, alert_type):
    """Check if an alert of this type was created recently (cooldown)."""
    cur.execute("""
        SELECT 1 FROM alerts
        WHERE agent_id = %s AND alert_type = %s
          AND created_at > NOW() - INTERVAL '%s minutes'
        LIMIT 1
    """, (agent_id, alert_type, ALERT_COOLDOWN_MINUTES))
    return cur.fetchone() is not None


def detect_threshold_violations(conn):
    """Detect recent telemetry that exceeds thresholds and create alerts (deduplicated)."""
    try:
        with conn.cursor(cursor_factory=RealDictCursor) as read_cur:
            read_cur.execute("""
                SELECT DISTINCT ON (agent_id)
                    agent_id, cpu_usage, memory_usage, load_avg_1, recorded_at
                FROM telemetry
                WHERE recorded_at >= NOW() - INTERVAL '5 minutes'
                ORDER BY agent_id, recorded_at DESC
            """)
            rows = read_cur.fetchall()

        alerts_created = 0
        with conn.cursor() as cur:
            for row in rows:
                agent_id = row["agent_id"]

                if row["cpu_usage"] is not None and float(row["cpu_usage"]) >= CPU_THRESHOLD:
                    if not _alert_exists_recently(cur, agent_id, "cpu_high"):
                        cur.execute("""
                            INSERT INTO alerts (agent_id, alert_type, severity, message,
                                                actual_value, threshold_value)
                            VALUES (%s, 'cpu_high', 'warning', %s, %s, %s)
                        """, (
                            agent_id,
                            f"CPU usage {row['cpu_usage']}%% exceeds threshold {CPU_THRESHOLD}%%",
                            float(row["cpu_usage"]),
                            CPU_THRESHOLD,
                        ))
                        alerts_created += 1

                if row["memory_usage"] is not None and float(row["memory_usage"]) >= MEMORY_THRESHOLD:
                    if not _alert_exists_recently(cur, agent_id, "memory_high"):
                        cur.execute("""
                            INSERT INTO alerts (agent_id, alert_type, severity, message,
                                                actual_value, threshold_value)
                            VALUES (%s, 'memory_high', 'warning', %s, %s, %s)
                        """, (
                            agent_id,
                            f"Memory usage {row['memory_usage']}%% exceeds threshold {MEMORY_THRESHOLD}%%",
                            float(row["memory_usage"]),
                            MEMORY_THRESHOLD,
                        ))
                        alerts_created += 1

                if row["load_avg_1"] is not None and float(row["load_avg_1"]) >= LOAD_THRESHOLD:
                    if not _alert_exists_recently(cur, agent_id, "load_high"):
                        cur.execute("""
                            INSERT INTO alerts (agent_id, alert_type, severity, message,
                                                actual_value, threshold_value)
                            VALUES (%s, 'load_high', 'critical', %s, %s, %s)
                        """, (
                            agent_id,
                            f"Load average {row['load_avg_1']} exceeds threshold {LOAD_THRESHOLD}",
                            float(row["load_avg_1"]),
                            LOAD_THRESHOLD,
                        ))
                        alerts_created += 1

        conn.commit()
        if alerts_created > 0:
            logger.warning("Created %d new alert(s)", alerts_created)
        elif rows:
            logger.info("Checked %d agent(s) for threshold violations — all OK", len(rows))
    except Exception as e:
        conn.rollback()
        logger.error("Threshold check failed: %s", e)


# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------
def clean_old_telemetry(conn):
    """Remove telemetry older than retention period."""
    try:
        with conn.cursor() as cur:
            cur.execute(
                "DELETE FROM telemetry WHERE recorded_at < NOW() - INTERVAL '1 day' * %s",
                (RETENTION_DAYS,),
            )
            deleted = cur.rowcount
        conn.commit()
        if deleted:
            logger.info("Cleaned %d old telemetry record(s) (retention=%d days)",
                        deleted, RETENTION_DAYS)
    except Exception as e:
        conn.rollback()
        logger.error("Cleanup failed: %s", e)


def clean_old_alerts(conn):
    """Remove acknowledged alerts older than 30 days."""
    try:
        with conn.cursor() as cur:
            cur.execute(
                "DELETE FROM alerts WHERE acknowledged = TRUE AND created_at < NOW() - INTERVAL '30 days'"
            )
            deleted = cur.rowcount
        conn.commit()
        if deleted:
            logger.info("Cleaned %d old acknowledged alert(s)", deleted)
    except Exception as e:
        conn.rollback()
        logger.error("Alert cleanup failed: %s", e)


# ---------------------------------------------------------------------------
# Agent status management
# ---------------------------------------------------------------------------
def update_agent_status(conn, rdb):
    """Mark agents as offline if no heartbeat in 2 minutes."""
    try:
        with conn.cursor(cursor_factory=RealDictCursor) as cur:
            cur.execute("""
                UPDATE agents SET status = 'offline'
                WHERE last_heartbeat < NOW() - INTERVAL '2 minutes' AND status = 'online'
                RETURNING agent_id
            """)
            offline = [r["agent_id"] for r in cur.fetchall()]
        conn.commit()

        for aid in offline:
            try:
                rdb.set(f"agent:{aid}:status", "offline", ex=300)
            except Exception:
                pass

        if offline:
            logger.info("Marked %d agent(s) as offline: %s", len(offline), offline)
    except Exception as e:
        conn.rollback()
        logger.warning("Failed to update agent status: %s", e)


# ---------------------------------------------------------------------------
# Summary logging
# ---------------------------------------------------------------------------
def log_summary(conn):
    """Log a quick summary of the system state."""
    try:
        with conn.cursor(cursor_factory=RealDictCursor) as cur:
            cur.execute("SELECT COUNT(*) AS total FROM agents")
            total = cur.fetchone()["total"]
            cur.execute("SELECT COUNT(*) AS online FROM agents WHERE status = 'online'")
            online = cur.fetchone()["online"]
            cur.execute("SELECT COUNT(*) AS cnt FROM telemetry WHERE recorded_at > NOW() - INTERVAL '1 hour'")
            recent = cur.fetchone()["cnt"]
            cur.execute("SELECT COUNT(*) AS cnt FROM alerts WHERE acknowledged = FALSE")
            unack = cur.fetchone()["cnt"]
        logger.info(
            "Summary: agents=%d (online=%d) | telemetry_last_hour=%d | unacked_alerts=%d",
            total, online, recent, unack,
        )
    except Exception as e:
        logger.warning("Failed to log summary: %s", e)


# ---------------------------------------------------------------------------
# Worker cycle
# ---------------------------------------------------------------------------
def run_cycle(conn, rdb, cycle_count):
    """Run one worker cycle."""
    compute_aggregates(conn)
    detect_threshold_violations(conn)
    update_agent_status(conn, rdb)

    # Less frequent tasks
    if cycle_count % 10 == 0:  # Every ~5 minutes
        compute_daily_aggregates(conn)
        clean_old_telemetry(conn)
        clean_old_alerts(conn)
        log_summary(conn)


def main():
    logger.info("=== Telemetry Worker starting ===")
    logger.info("  Interval: %ds", INTERVAL_SECONDS)
    logger.info("  DB: %s:%s/%s", DB_CONFIG["host"], DB_CONFIG["port"], DB_CONFIG["dbname"])
    logger.info("  Redis: %s:%s", REDIS_CONFIG["host"], REDIS_CONFIG["port"])
    logger.info("  Retention: %d days", RETENTION_DAYS)
    logger.info("  Thresholds: CPU>%.0f%% MEM>%.0f%% LOAD>%.1f",
                CPU_THRESHOLD, MEMORY_THRESHOLD, LOAD_THRESHOLD)

    conn = None
    rdb = None
    cycle_count = 0

    # Wait a bit for services to be ready
    time.sleep(5)

    while True:
        try:
            if conn is None or conn.closed:
                logger.info("Connecting to database...")
                conn = get_db_conn()
                logger.info("Database connected")
            if rdb is None:
                logger.info("Connecting to Redis...")
                rdb = get_redis()
                rdb.ping()
                logger.info("Redis connected")

            run_cycle(conn, rdb, cycle_count)
            cycle_count += 1

        except psycopg2.OperationalError as e:
            logger.error("Database connection error: %s", e)
            if conn:
                try:
                    conn.close()
                except Exception:
                    pass
                conn = None
        except redis.ConnectionError as e:
            logger.error("Redis connection error: %s", e)
            rdb = None
        except Exception as e:
            logger.error("Unexpected error in cycle: %s", e, exc_info=True)
            if conn:
                try:
                    conn.rollback()
                except Exception:
                    pass

        time.sleep(INTERVAL_SECONDS)


if __name__ == "__main__":
    main()
