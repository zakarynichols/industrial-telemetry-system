package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS machines (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name VARCHAR(255) NOT NULL,
			type VARCHAR(100),
			location VARCHAR(255),
			metadata JSONB,
			status VARCHAR(50) DEFAULT 'active',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_machines_status ON machines(status)`,

		`CREATE TABLE IF NOT EXISTS metrics (
			time TIMESTAMPTZ NOT NULL,
			machine_id UUID NOT NULL,
			metric_name VARCHAR(100) NOT NULL,
			value DOUBLE PRECISION NOT NULL,
			unit VARCHAR(50),
			quality VARCHAR(20) DEFAULT 'good'
		)`,

		`CREATE TABLE IF NOT EXISTS anomalies (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			machine_id UUID NOT NULL,
			metric_name VARCHAR(100),
			detected_at TIMESTAMPTZ DEFAULT NOW(),
			severity VARCHAR(20) NOT NULL,
			description TEXT,
			raw_data JSONB,
			FOREIGN KEY (machine_id) REFERENCES machines(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_anomalies_machine ON anomalies(machine_id, detected_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_anomalies_severity ON anomalies(severity)`,

		`CREATE TABLE IF NOT EXISTS alerts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			machine_id UUID,
			rule_id UUID,
			severity VARCHAR(20) NOT NULL,
			message TEXT NOT NULL,
			acknowledged BOOLEAN DEFAULT FALSE,
			acknowledged_by VARCHAR(255),
			acknowledged_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_acknowledged ON alerts(acknowledged, created_at DESC)`,

		`CREATE TABLE IF NOT EXISTS alert_rules (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name VARCHAR(255) NOT NULL,
			metric_name VARCHAR(100) NOT NULL,
			condition_type VARCHAR(50) NOT NULL,
			threshold_value DOUBLE PRECISION,
			operator VARCHAR(10),
			severity VARCHAR(20) NOT NULL,
			enabled BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
	}

	for i, sql := range migrations {
		if _, err := pool.Exec(ctx, sql); err != nil {
			return fmt.Errorf("migration %d failed: %w", i, err)
		}
	}

	_, err := pool.Exec(ctx, `SELECT create_hypertable('metrics', 'time', chunk_time_interval => INTERVAL '1 hour', if_not_exists => TRUE)`)
	if err != nil {
		logMigrationError("Failed to create hypertable (may already exist): %v", err)
	}

	if _, err := pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_metrics_machine_time ON metrics(machine_id, time DESC)`); err != nil {
		logMigrationError("Failed to create metrics index: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_metrics_name_time ON metrics(metric_name, time DESC)`); err != nil {
		logMigrationError("Failed to create metrics name index: %v", err)
	}

	return nil
}

func logMigrationError(format string, args ...interface{}) {
	fmt.Printf("WARNING: "+format+"\n", args...)
}
