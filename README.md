# Industrial Telemetry System

Real-time monitoring for industrial pumps with anomaly detection and alerting.

## Use Case

**Scenario**: You're a lead maintenance technician at a water treatment plant with 50+ pumps across 3 buildings.

You open Grafana and see:

- **Building A has 2 critical alerts** - click to see which pumps
- **PUMP-007 bearing wear at 78%** - schedule replacement next week
- **PUMP-012 showing cavitation** - inspect intake immediately
- **Zone 3 running 4/6 pumps** - why are 2 stopped?

Each pump tracks real health indicators (bearing wear, seal condition, motor health) not just temperature. Alerts include pump name and location so you know exactly what needs attention.

## Adapter Logic

The telemetry service normalizes incoming data from any source or schema into a unified format.

Real industrial equipment sends data in different formats. A Siemens S7 PLC might send `{ "temp": 45.2 }` via Modbus, while an ABB ACS550 VFD sends `{ "motor_temp_celsius": 46.1 }` over MQTT. An Allen-Bradley CompactLogix might push JSON via HTTP. The adapter normalizes these into a standard schema:

```json
{
  "machine_id": "pump-001",
  "metric_name": "temperature",
  "value": 45.2,
  "unit": "celsius",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

This lets you connect any equipment regardless of its native format, while downstream (dashboards, alerts) work with a consistent schema.

## Tech Stack

- **TimescaleDB** - Time-series database for high-frequency metrics
- **Go** - Telemetry service (HTTP/MQTT ingestion, alert processing)
- **Grafana** - Dashboards
- **Docker** - Single-command deployment

## Quick Start

```bash
docker-compose up -d

# Access Grafana
open http://localhost:3000
# Login: admin / admin123
```

You will also need to add your password to the TimescaleDB data source in Grafana. On the left, click 'Connections' -> 'Data sources' -> 'TimescaleDB'. 

All fields will be pre-populated from our provisioning profile, but you will need to input the password `telemetry_secure_pass_123`.

## Endpoints

| Service | URL |
|---------|-----|
| Grafana | http://localhost:3000 |
| Telemetry API | http://localhost:8083 |
| MQTT Broker | localhost:1883 |
