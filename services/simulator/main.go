package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

type PumpState struct {
	MachineID      uuid.UUID
	Name           string
	OperatingState string
	RunningTime    float64
	BearingWear    float64
	SealCondition  float64
	MotorHealth    float64
	FailureMode    string
	ThermalEvent   string
	ThermalValue   float64
	LastUpdate     time.Time
	StartupTime    time.Time
}

type Simulator struct {
	apiURL        string
	machineCount  int
	metricsPerSec int
	pumps         []PumpState
	rand          *rand.Rand
}

func main() {
	apiURL := getEnv("API_URL", "http://localhost:8083")
	machineCount := getEnvInt("MACHINE_COUNT", 3)
	metricsPerSec := getEnvInt("METRICS_PER_SECOND", 10)

	seed := time.Now().UnixNano()
	log.Printf("Using seed: %d", seed)

	sim := &Simulator{
		apiURL:        apiURL,
		machineCount:  machineCount,
		metricsPerSec: metricsPerSec,
		rand:          rand.New(rand.NewSource(seed)),
	}

	if err := sim.registerPumps(); err != nil {
		log.Fatalf("Failed to register pumps: %v", err)
	}

	log.Printf("Starting centrifugal pump simulator with %d pumps, %d metrics/sec", sim.machineCount, sim.metricsPerSec)
	sim.run()
}

func (s *Simulator) registerPumps() error {
	pumpTypes := []string{
		"Centrifugal Water Pump",
		"Process Feed Pump",
		"Coolant Circulation Pump",
	}

	locations := []string{
		"Water Treatment Plant - Building A",
		"Chemical Processing Unit - Zone 3",
		"HVAC Cooling Tower - Plant 2",
	}

	for i := 0; i < s.machineCount; i++ {
		machine := PumpState{
			MachineID:      uuid.New(),
			Name:           fmt.Sprintf("PUMP-%s-%03d", getEnv("PLANT_CODE", "WTP"), i+1),
			OperatingState: "stopped",
			RunningTime:    0,
			BearingWear:    s.rand.Float64() * 0.3,
			SealCondition:  1.0 - s.rand.Float64()*0.2,
			MotorHealth:    1.0,
			FailureMode:    "none",
			LastUpdate:     time.Now(),
		}

		reqBody, _ := json.Marshal(map[string]string{
			"name":     machine.Name,
			"type":     pumpTypes[i%len(pumpTypes)],
			"location": locations[i%len(locations)],
		})

		resp, err := http.Post(s.apiURL+"/api/v1/machines", "application/json", bytes.NewReader(reqBody))
		if err != nil {
			return fmt.Errorf("failed to create pump: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			return fmt.Errorf("failed to create pump: status %d", resp.StatusCode)
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
			if id, ok := result["id"].(string); ok {
				machine.MachineID, _ = uuid.Parse(id)
			}
		}

		s.pumps = append(s.pumps, machine)
		log.Printf("Registered pump: %s (%s) - %s", machine.Name, machine.MachineID, pumpTypes[i%len(pumpTypes)])
	}

	return nil
}

func (s *Simulator) run() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	secondCounter := 0

	for range ticker.C {
		secondCounter++

		for i := range s.pumps {
			s.updatePumpState(&s.pumps[i], secondCounter)
		}

		for i := 0; i < s.metricsPerSec; i++ {
			pump := s.pumps[s.rand.Intn(len(s.pumps))]
			metrics := s.generatePumpMetrics(&pump)

			for metricName, value := range metrics {
				s.sendMetric(pump.MachineID, metricName, value)
			}
		}

		if secondCounter%30 == 0 {
			log.Printf("Status: %d pumps | States: ", len(s.pumps))
			for _, p := range s.pumps {
				log.Printf("  %s: %s (%.1fh, wear: %.1f%%)", p.Name, p.OperatingState, p.RunningTime/3600, p.BearingWear*100)
			}
		}

		if secondCounter%5 == 0 {
			for i := range s.pumps {
				s.sendHealthMetrics(&s.pumps[i])
			}
		}
	}
}

func (s *Simulator) updatePumpState(pump *PumpState, secondCounter int) {
	pump.LastUpdate = time.Now()

	switch pump.OperatingState {
	case "stopped":
		if secondCounter%30 == 0 && s.rand.Float64() > 0.5 {
			pump.OperatingState = "starting"
			pump.StartupTime = time.Now()
			log.Printf("[%s] Starting pump", pump.Name)
		}

	case "starting":
		if time.Since(pump.StartupTime) > 5*time.Second {
			pump.OperatingState = "running"
			log.Printf("[%s] Pump running", pump.Name)
		}

	case "running":
		pump.RunningTime += 1

		if s.rand.Float64() < 0.001 {
			pump.OperatingState = "stopping"
			log.Printf("[%s] Stopping pump", pump.Name)
		}

		if s.rand.Float64() < 0.0001 {
			modes := []string{"bearing_wear", "cavitation", "motor_overload", "seal_wear", "discharge_blockage"}
			pump.FailureMode = modes[s.rand.Intn(len(modes))]
			log.Printf("[%s] FAILURE MODE TRIGGERED: %s", pump.Name, pump.FailureMode)
		}

	case "stopping":
		if s.rand.Float64() > 0.5 {
			pump.OperatingState = "stopped"
			log.Printf("[%s] Pump stopped", pump.Name)
		}
	}

	if pump.OperatingState == "running" {
		pump.BearingWear += s.rand.Float64() * 0.00001
		if pump.BearingWear > 1.0 {
			pump.BearingWear = 1.0
		}

		if pump.FailureMode == "seal_wear" {
			pump.SealCondition -= s.rand.Float64() * 0.001
			if pump.SealCondition < 0 {
				pump.SealCondition = 0
			}
		}
	}

	if pump.FailureMode != "none" && s.rand.Float64() < 0.001 {
		pump.FailureMode = "none"
		log.Printf("[%s] Failure condition cleared", pump.Name)
	}

	if pump.OperatingState == "running" {
		if pump.ThermalEvent == "" && s.rand.Float64() < 0.02 {
			thermalLevels := []string{"warning", "critical"}
			pump.ThermalEvent = thermalLevels[s.rand.Intn(len(thermalLevels))]
			pump.ThermalValue = 0
			log.Printf("[%s] THERMAL EVENT STARTED: %s", pump.Name, pump.ThermalEvent)
		}

		if pump.ThermalEvent != "" {
			pump.ThermalValue += s.rand.Float64() * 3

			if pump.ThermalEvent == "warning" {
				if pump.ThermalValue > 10 || s.rand.Float64() < 0.01 {
					pump.ThermalEvent = ""
					pump.ThermalValue = 0
					log.Printf("[%s] Thermal event resolved", pump.Name)
				}
			} else if pump.ThermalEvent == "critical" {
				if pump.ThermalValue > 8 || s.rand.Float64() < 0.008 {
					pump.ThermalEvent = ""
					pump.ThermalValue = 0
					log.Printf("[%s] Thermal event resolved", pump.Name)
				}
			}
		}
	}
}

func (s *Simulator) generatePumpMetrics(pump *PumpState) map[string]float64 {
	metrics := make(map[string]float64)

	baseRPM := 1750.0
	nominalPressure := 4.0
	nominalCurrent := 120.0
	nominalTemp := 55.0

	switch pump.OperatingState {
	case "stopped":
		metrics["rpm"] = 0
		metrics["pressure"] = 0.5 + s.gaussianNoise(0.1)
		metrics["current"] = 2 + s.gaussianNoise(0.5)
		metrics["temperature"] = s.gaussianNoise(2) + 25
		metrics["vibration"] = 0
		metrics["voltage"] = s.gaussianNoise(5) + 460

	case "starting":
		elapsed := time.Since(pump.StartupTime).Seconds()
		rampUp := math.Min(elapsed/5.0, 1.0)

		metrics["rpm"] = baseRPM * rampUp
		metrics["pressure"] = nominalPressure * rampUp
		metrics["current"] = nominalCurrent*(0.8+0.2*rampUp) + s.gaussianNoise(10)
		metrics["temperature"] = nominalTemp*rampUp + s.gaussianNoise(3)
		metrics["vibration"] = 2 * rampUp
		metrics["voltage"] = s.gaussianNoise(5) + 460

	case "running":
		rpm := baseRPM + s.gaussianNoise(10)
		metrics["rpm"] = rpm

		pressureBase := nominalPressure
		vibrationBase := 2.0 + pump.BearingWear*5.0
		tempBase := nominalTemp + pump.BearingWear*20
		currentBase := nominalCurrent

		switch pump.FailureMode {
		case "bearing_wear":
			vibrationBase += 3.0
			tempBase += 10.0
		case "cavitation":
			pressureBase -= 1.5
			vibrationBase += 2.5
			currentBase -= 10
		case "motor_overload":
			currentBase += 50
			tempBase += 15
		case "seal_wear":
			pressureBase -= 0.3
			tempBase += 5.0
		case "discharge_blockage":
			pressureBase += 2.0
			currentBase -= 30
		}

		metrics["pressure"] = pressureBase + s.gaussianNoise(0.15)
		metrics["vibration"] = vibrationBase + s.gaussianNoise(0.3)

		tempWithThermal := tempBase
		if pump.ThermalEvent == "warning" {
			tempWithThermal += 15 + pump.ThermalValue
		} else if pump.ThermalEvent == "critical" {
			tempWithThermal += 25 + pump.ThermalValue
		}
		metrics["temperature"] = tempWithThermal + s.gaussianNoise(2)

		metrics["current"] = currentBase + s.gaussianNoise(5)
		metrics["voltage"] = s.gaussianNoise(10) + 460

		metrics["vibration"] = math.Max(0, metrics["vibration"])
		metrics["temperature"] = math.Max(20, metrics["temperature"])
		metrics["pressure"] = math.Max(0, metrics["pressure"])

	case "stopping":
		metrics["rpm"] = s.gaussianNoise(20)
		metrics["pressure"] = nominalPressure * 0.5
		metrics["current"] = s.gaussianNoise(3) + 5
		metrics["temperature"] = nominalTemp * 0.8
		metrics["vibration"] = s.gaussianNoise(0.5)
		metrics["voltage"] = s.gaussianNoise(5) + 460
	}

	metrics["rpm"] = math.Max(0, metrics["rpm"])

	return metrics
}

func (s *Simulator) gaussianNoise(sigma float64) float64 {
	u1 := s.rand.Float64()
	u2 := s.rand.Float64()
	if u1 == 0 {
		u1 = 0.000001
	}
	return sigma * math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
}

func (s *Simulator) sendMetric(machineID uuid.UUID, metricName string, value float64) {
	units := map[string]string{
		"temperature":    "celsius",
		"pressure":       "bar",
		"vibration":      "mm/s",
		"rpm":            "rpm",
		"voltage":        "volts",
		"current":        "amps",
		"bearing_wear":   "percent",
		"seal_condition": "percent",
		"motor_health":   "percent",
		"running_time":   "hours",
	}

	unit := units[metricName]
	if unit == "" {
		unit = "unknown"
	}

	payload := map[string]interface{}{
		"machine_id":  machineID.String(),
		"metric_name": metricName,
		"value":       math.Round(value*100) / 100,
		"unit":        unit,
		"quality":     "good",
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(s.apiURL+"/api/v1/metrics/ingest", "application/json", bytes.NewReader(body))
	if err != nil {
		return
	}
	defer resp.Body.Close()
}

func (s *Simulator) sendHealthMetrics(pump *PumpState) {
	s.sendMetric(pump.MachineID, "bearing_wear", pump.BearingWear*100)
	s.sendMetric(pump.MachineID, "seal_condition", pump.SealCondition*100)
	s.sendMetric(pump.MachineID, "motor_health", pump.MotorHealth*100)
	s.sendMetric(pump.MachineID, "running_time", pump.RunningTime/3600)

	stateValue := 0.0
	switch pump.OperatingState {
	case "running":
		stateValue = 3.0
	case "starting":
		stateValue = 2.0
	case "stopping":
		stateValue = 1.0
	case "stopped":
		stateValue = 0.0
	}
	s.sendMetric(pump.MachineID, "operating_state", stateValue)

	failureValue := 0.0
	switch pump.FailureMode {
	case "bearing_wear":
		failureValue = 1.0
	case "cavitation":
		failureValue = 2.0
	case "motor_overload":
		failureValue = 3.0
	case "seal_wear":
		failureValue = 4.0
	case "discharge_blockage":
		failureValue = 5.0
	}
	s.sendMetric(pump.MachineID, "failure_mode", failureValue)
}

func getEnv(key string, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var intVal int
		if _, err := fmt.Sscanf(value, "%d", &intVal); err == nil {
			return intVal
		}
	}
	return defaultValue
}
