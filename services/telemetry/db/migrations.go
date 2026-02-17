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

	if err := seedAlertRules(ctx, pool); err != nil {
		logMigrationError("Failed to seed alert rules: %v", err)
	}

	return nil
}

func seedAlertRules(ctx context.Context, pool *pgxpool.Pool) error {
	rules := []struct {
		name, metricName, conditionType, operator, severity string
		thresholdValue                                      float64
	}{
		// Temperature alerts - motor winding temp
		{"Motor Temperature High", "temperature", "threshold", ">", "warning", 70},
		{"Motor Temperature Critical", "temperature", "threshold", ">", "critical", 80},

		// Vibration alerts - bearing health indicator
		{"Vibration Warning", "vibration", "threshold", ">", "warning", 5.0},
		{"Vibration Critical", "vibration", "threshold", ">", "critical", 7.0},

		// Pressure alerts - discharge pressure
		{"Discharge Pressure High", "pressure", "threshold", ">", "warning", 5.0},
		{"Discharge Pressure Critical", "pressure", "threshold", ">", "critical", 5.5},
		{"Discharge Pressure Low", "pressure", "threshold", "<", "warning", 2.0},

		// Current alerts - motor load
		{"Motor Current High", "current", "threshold", ">", "warning", 140},
		{"Motor Current Critical", "current", "threshold", ">", "critical", 170},

		// RPM alerts - motor speed
		{"RPM Low", "rpm", "threshold", "<", "warning", 1700},
		{"RPM High", "rpm", "threshold", ">", "warning", 1800},

		// Voltage alerts - power quality
		{"Voltage Low", "voltage", "threshold", "<", "warning", 440},
		{"Voltage High", "voltage", "threshold", ">", "critical", 480},
	}

	for _, rule := range rules {
		var exists bool
		err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM alert_rules WHERE name = $1 AND metric_name = $2)", rule.name, rule.metricName).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check rule existence: %w", err)
		}

		if !exists {
			_, err := pool.Exec(ctx,
				"INSERT INTO alert_rules (name, metric_name, condition_type, threshold_value, operator, severity) VALUES ($1, $2, $3, $4, $5, $6)",
				rule.name, rule.metricName, rule.conditionType, rule.thresholdValue, rule.operator, rule.severity,
			)
			if err != nil {
				return fmt.Errorf("failed to insert rule %s: %w", rule.name, err)
			}
			fmt.Printf("Seeded alert rule: %s\n", rule.name)
		}
	}

	return nil
}

func logMigrationError(format string, args ...interface{}) {
	fmt.Printf("WARNING: "+format+"\n", args...)
}
