package mqtt

import (
	"encoding/json"
	"log"
	"net"
	"strings"

	"github.com/google/uuid"

	"telemetry/processing"
)

type Server struct {
	alertService *processing.AlertService
	clients      map[string]net.Conn
}

func StartServer(alertService *processing.AlertService) error {
	s := &Server{
		alertService: alertService,
		clients:      make(map[string]net.Conn),
	}

	ln, err := net.Listen("tcp", ":1883")
	if err != nil {
		return err
	}
	log.Println("MQTT server listening on :1883")

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}
		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, 1024)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return
		}

		data := string(buf[:n])
		if strings.Contains(data, "CONNECT") {
			conn.Write([]byte{0x20, 0x02, 0x00, 0x00})
		}

		if strings.Contains(data, "PUBLISH") {
			s.processPublish(data)
		}
	}
}

func (s *Server) processPublish(data string) {
	parts := strings.Split(data, "/")
	if len(parts) < 4 {
		return
	}

	topic := parts[1]
	payloadStart := strings.Index(data, "{")
	if payloadStart == -1 {
		return
	}

	var msg map[string]interface{}
	if err := json.Unmarshal([]byte(data[payloadStart:]), &msg); err != nil {
		return
	}

	machineIDStr, _ := msg["machine_id"].(string)
	metricName, _ := msg["metric_name"].(string)
	value, _ := msg["value"].(float64)

	if machineIDStr == "" || metricName == "" {
		return
	}

	machineID, err := uuid.Parse(machineIDStr)
	if err != nil {
		return
	}

	log.Printf("MQTT: %s/%s = %.2f", topic, metricName, value)
	s.alertService.CheckMetric(machineID, metricName, value)
}
