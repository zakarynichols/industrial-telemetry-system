package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Router struct {
	db *pgxpool.Pool
}

func NewRouter(pool *pgxpool.Pool) *Router {
	return &Router{db: pool}
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.URL.Path {
	case "/health":
		r.health(w, req)
	case "/api/v1/machines":
		r.machines(w, req)
	case "/api/v1/metrics":
		r.metrics(w, req)
	case "/api/v1/metrics/ingest":
		r.ingest(w, req)
	case "/api/v1/alerts":
		r.alerts(w, req)
	case "/api/v1/rules":
		r.rules(w, req)
	default:
		http.NotFound(w, req)
	}
}

func (r *Router) health(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

type Machine struct {
	ID        uuid.UUID              `json:"id"`
	Name      string                 `json:"name"`
	Type      string                 `json:"type,omitempty"`
	Location  string                 `json:"location,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Status    string                 `json:"status"`
	CreatedAt time.Time              `json:"created_at"`
}

func (r *Router) machines(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if req.Method == "GET" {
		rows, err := r.db.Query(req.Context(), "SELECT id, name, type, location, metadata, status, created_at FROM machines ORDER BY created_at DESC")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var machines []Machine
		for rows.Next() {
			var m Machine
			if err := rows.Scan(&m.ID, &m.Name, &m.Type, &m.Location, &m.Metadata, &m.Status, &m.CreatedAt); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			machines = append(machines, m)
		}
		if machines == nil {
			machines = []Machine{}
		}
		json.NewEncoder(w).Encode(machines)
		return
	}

	if req.Method == "POST" {
		var input struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			Location string `json:"location"`
		}
		if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var id uuid.UUID
		err := r.db.QueryRow(req.Context(),
			"INSERT INTO machines (name, type, location) VALUES ($1, $2, $3) RETURNING id",
			input.Name, input.Type, input.Location,
		).Scan(&id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"id": id, "name": input.Name})
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

type Metric struct {
	Time       time.Time `json:"time"`
	MachineID  uuid.UUID `json:"machine_id"`
	MetricName string    `json:"metric_name"`
	Value      float64   `json:"value"`
	Unit       string    `json:"unit,omitempty"`
	Quality    string    `json:"quality"`
}

func (r *Router) metrics(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if req.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	machineID := req.URL.Query().Get("machine_id")
	limit := req.URL.Query().Get("limit")
	if limit == "" {
		limit = "100"
	}

	var rows pgx.Rows
	var err error

	if machineID != "" {
		rows, err = r.db.Query(req.Context(),
			"SELECT time, machine_id, metric_name, value, unit, quality FROM metrics WHERE machine_id = $1 ORDER BY time DESC LIMIT $2",
			machineID, limit,
		)
	} else {
		rows, err = r.db.Query(req.Context(),
			"SELECT time, machine_id, metric_name, value, unit, quality FROM metrics ORDER BY time DESC LIMIT $1",
			limit,
		)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var metrics []Metric
	for rows.Next() {
		var m Metric
		if err := rows.Scan(&m.Time, &m.MachineID, &m.MetricName, &m.Value, &m.Unit, &m.Quality); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		metrics = append(metrics, m)
	}
	if metrics == nil {
		metrics = []Metric{}
	}
	json.NewEncoder(w).Encode(metrics)
}

type IngestRequest struct {
	MachineID  string  `json:"machine_id"`
	MetricName string  `json:"metric_name"`
	Value      float64 `json:"value"`
	Unit       string  `json:"unit,omitempty"`
	Quality    string  `json:"quality,omitempty"`
	Timestamp  string  `json:"timestamp,omitempty"`
}

func (r *Router) ingest(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if req.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var input IngestRequest
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	machineID, err := uuid.Parse(input.MachineID)
	if err != nil {
		http.Error(w, "invalid machine_id", http.StatusBadRequest)
		return
	}

	timestamp := time.Now()
	if input.Timestamp != "" {
		timestamp, _ = time.Parse(time.RFC3339, input.Timestamp)
	}

	quality := input.Quality
	if quality == "" {
		quality = "good"
	}

	_, err = r.db.Exec(req.Context(),
		"INSERT INTO metrics (time, machine_id, metric_name, value, unit, quality) VALUES ($1, $2, $3, $4, $5, $6)",
		timestamp, machineID, input.MetricName, input.Value, input.Unit, quality,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (r *Router) alerts(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if req.Method == "GET" {
		rows, err := r.db.Query(req.Context(),
			"SELECT id, machine_id, severity, message, acknowledged, created_at FROM alerts ORDER BY created_at DESC LIMIT 100")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var alerts []map[string]interface{}
		for rows.Next() {
			var id, machineID uuid.UUID
			var severity, message string
			var acknowledged bool
			var createdAt time.Time
			if err := rows.Scan(&id, &machineID, &severity, &message, &acknowledged, &createdAt); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			alerts = append(alerts, map[string]interface{}{
				"id":           id,
				"machine_id":   machineID,
				"severity":     severity,
				"message":      message,
				"acknowledged": acknowledged,
				"created_at":   createdAt,
			})
		}
		if alerts == nil {
			alerts = []map[string]interface{}{}
		}
		json.NewEncoder(w).Encode(alerts)
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (r *Router) rules(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if req.Method == "GET" {
		rows, err := r.db.Query(req.Context(),
			"SELECT id, name, metric_name, condition_type, threshold_value, operator, severity, enabled FROM alert_rules")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var rules []map[string]interface{}
		for rows.Next() {
			var id uuid.UUID
			var name, metricName, conditionType, operator, severity string
			var thresholdValue *float64
			var enabled bool
			if err := rows.Scan(&id, &name, &metricName, &conditionType, &thresholdValue, &operator, &severity, &enabled); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			rules = append(rules, map[string]interface{}{
				"id":              id,
				"name":            name,
				"metric_name":     metricName,
				"condition_type":  conditionType,
				"threshold_value": thresholdValue,
				"operator":        operator,
				"severity":        severity,
				"enabled":         enabled,
			})
		}
		if rules == nil {
			rules = []map[string]interface{}{}
		}
		json.NewEncoder(w).Encode(rules)
		return
	}

	if req.Method == "POST" {
		var input struct {
			Name           string  `json:"name"`
			MetricName     string  `json:"metric_name"`
			ConditionType  string  `json:"condition_type"`
			ThresholdValue float64 `json:"threshold_value"`
			Operator       string  `json:"operator"`
			Severity       string  `json:"severity"`
		}
		if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var id uuid.UUID
		err := r.db.QueryRow(req.Context(),
			"INSERT INTO alert_rules (name, metric_name, condition_type, threshold_value, operator, severity) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id",
			input.Name, input.MetricName, input.ConditionType, input.ThresholdValue, input.Operator, input.Severity,
		).Scan(&id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"id": id})
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}
