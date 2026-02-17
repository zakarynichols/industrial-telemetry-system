package processing

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

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

func (s *AlertService) StartBackgroundChecks(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Println("Starting alert auto-resolve background check")
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.resolveAlerts(ctx)
		}
	}
}

func (s *AlertService) resolveAlerts(ctx context.Context) {
	s.mu.RLock()
	rules := make(map[uuid.UUID]AlertRule)
	for _, r := range s.rules {
		rules[r.ID] = r
	}
	s.mu.RUnlock()

	rows, err := s.db.Query(ctx,
		`SELECT a.id, a.machine_id, a.rule_id, m.value 
		 FROM alerts a 
		 JOIN alert_rules r ON a.rule_id = r.id 
		 LEFT JOIN LATERAL (
			 SELECT value FROM metrics 
			 WHERE machine_id = a.machine_id AND metric_name = r.metric_name 
			 ORDER BY time DESC LIMIT 1
		 ) m ON true
		 WHERE a.acknowledged = false`,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var alertID, machineID, ruleID uuid.UUID
		var currentValue *float64
		if err := rows.Scan(&alertID, &machineID, &ruleID, &currentValue); err != nil {
			continue
		}

		rule, ok := rules[ruleID]
		if !ok || currentValue == nil {
			continue
		}

		stillTriggered := false
		switch rule.Operator {
		case ">":
			stillTriggered = *currentValue > rule.ThresholdValue
		case "<":
			stillTriggered = *currentValue < rule.ThresholdValue
		case ">=":
			stillTriggered = *currentValue >= rule.ThresholdValue
		case "<=":
			stillTriggered = *currentValue <= rule.ThresholdValue
		}

		if !stillTriggered {
			_, err := s.db.Exec(ctx,
				"UPDATE alerts SET acknowledged = true, acknowledged_by = 'system', acknowledged_at = NOW() WHERE id = $1",
				alertID,
			)
			if err == nil {
				log.Printf("Auto-resolved alert %s for machine %s: value now %.2f (threshold: %.2f)",
					alertID, machineID, *currentValue, rule.ThresholdValue)
			}
		}
	}
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
	var existingID uuid.UUID
	err := s.db.QueryRow(context.Background(),
		"SELECT id FROM alerts WHERE machine_id = $1 AND rule_id = $2 AND acknowledged = false",
		machineID, rule.ID,
	).Scan(&existingID)

	if err == nil {
		return
	}

	message := rule.Name
	if message == "" {
		message = rule.MetricName
	}
	message += fmt.Sprintf(" - value: %.2f (threshold: %.2f)", value, rule.ThresholdValue)

	_, err = s.db.Exec(context.Background(),
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
