package processing

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"telemetry/config"
)

type AlertService struct {
	db    *pgxpool.Pool
	cfg   *config.Config
	mu    sync.RWMutex
	rules []AlertRule
}

type AlertRule struct {
	ID             uuid.UUID
	Name           string
	MetricName     string
	ConditionType  string
	ThresholdValue float64
	Operator       string
	Severity       string
	Enabled        bool
}

func NewAlertService(pool *pgxpool.Pool, cfg *config.Config) *AlertService {
	s := &AlertService{db: pool, cfg: cfg}
	s.loadRules(context.Background())
	return s
}

func (s *AlertService) loadRules(ctx context.Context) {
	rows, err := s.db.Query(ctx, "SELECT id, name, metric_name, condition_type, threshold_value, operator, severity, enabled FROM alert_rules WHERE enabled = true")
	if err != nil {
		log.Printf("Failed to load alert rules: %v", err)
		return
	}
	defer rows.Close()

	var rules []AlertRule
	for rows.Next() {
		var r AlertRule
		if err := rows.Scan(&r.ID, &r.Name, &r.MetricName, &r.ConditionType, &r.ThresholdValue, &r.Operator, &r.Severity, &r.Enabled); err != nil {
			continue
		}
		rules = append(rules, r)
	}
	s.mu.Lock()
	s.rules = rules
	s.mu.Unlock()
	log.Printf("Loaded %d alert rules", len(rules))
}

func (s *AlertService) CheckMetric(machineID uuid.UUID, metricName string, value float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, rule := range s.rules {
		if rule.MetricName != metricName {
			continue
		}

		triggered := false
		switch rule.Operator {
		case ">":
			triggered = value > rule.ThresholdValue
		case "<":
			triggered = value < rule.ThresholdValue
		case ">=":
			triggered = value >= rule.ThresholdValue
		case "<=":
			triggered = value <= rule.ThresholdValue
		}

		if triggered {
			s.createAlert(machineID, rule, value)
		}
	}
}

func (s *AlertService) createAlert(machineID uuid.UUID, rule AlertRule, value float64) {
	message := rule.Name
	if message == "" {
		message = rule.MetricName
	}
	message += fmt.Sprintf(" - value: %.2f (threshold: %.2f)", value, rule.ThresholdValue)

	_, err := s.db.Exec(context.Background(),
		"INSERT INTO alerts (machine_id, rule_id, severity, message) VALUES ($1, $2, $3, $4)",
		machineID, rule.ID, rule.Severity, message,
	)
	if err != nil {
		log.Printf("Failed to create alert: %v", err)
		return
	}

	log.Printf("ALERT [%s] %s for machine %s: %s", rule.Severity, rule.Name, machineID, message)

	go s.sendNotifications(rule.Severity, message)
}

func (s *AlertService) sendNotifications(severity, message string) {
	if s.cfg.SlackWebhook != "" {
		log.Printf("Would send Slack notification: %s", message)
	}
}
