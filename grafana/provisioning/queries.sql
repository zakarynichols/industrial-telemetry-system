-- Active Pumps (machines sending data in last 5 min)
SELECT count(DISTINCT machine_id) FROM metrics WHERE time > now() - interval '5 minutes'

-- Pumps Sending Data (machines with recent metrics)
SELECT count(DISTINCT machine_id) FROM metrics WHERE time > now() - interval '1 minute'

-- Active Alerts (unacknowledged)
SELECT count(*) FROM alerts WHERE acknowledged = false

-- Metrics per second (last minute)
SELECT count(*)::float / 60 FROM metrics WHERE time > now() - interval '1 minute'

-- Active Rules
SELECT count(*) FROM alert_rules WHERE enabled = true

-- Avg Temperature (last minute)
SELECT avg(value) FROM metrics WHERE metric_name = 'temperature' AND time > now() - interval '1 minute'

-- Avg Vibration (last minute)
SELECT avg(value) FROM metrics WHERE metric_name = 'vibration' AND time > now() - interval '1 minute'

-- Avg Pressure (last minute)
SELECT avg(value) FROM metrics WHERE metric_name = 'pressure' AND time > now() - interval '1 minute'

-- Avg RPM (last minute)
SELECT avg(value) FROM metrics WHERE metric_name = 'rpm' AND time > now() - interval '1 minute'

-- Temperature Time Series (10s buckets)
SELECT
  time_bucket('10 seconds', time) AS time,
  avg(value) AS value
FROM metrics
WHERE $__timeFilter(time)
  AND metric_name = 'temperature'
GROUP BY time_bucket('10 seconds', time)
ORDER BY time

-- Vibration Time Series (10s buckets)
SELECT
  time_bucket('10 seconds', time) AS time,
  avg(value) AS value
FROM metrics
WHERE $__timeFilter(time)
  AND metric_name = 'vibration'
GROUP BY time_bucket('10 seconds', time)
ORDER BY time

-- Pressure Time Series (10s buckets)
SELECT
  time_bucket('10 seconds', time) AS time,
  avg(value) AS value
FROM metrics
WHERE $__timeFilter(time)
  AND metric_name = 'pressure'
GROUP BY time_bucket('10 seconds', time)
ORDER BY time

-- RPM Time Series (10s buckets)
SELECT
  time_bucket('10 seconds', time) AS time,
  avg(value) AS value
FROM metrics
WHERE $__timeFilter(time)
  AND metric_name = 'rpm'
GROUP BY time_bucket('10 seconds', time)
ORDER BY time

-- Machine Status Overview
SELECT 
  m.name,
  max(CASE WHEN mt.metric_name = 'temperature' THEN mt.value END) as temperature,
  max(CASE WHEN mt.metric_name = 'pressure' THEN mt.value END) as pressure,
  max(CASE WHEN mt.metric_name = 'vibration' THEN mt.value END) as vibration,
  max(CASE WHEN mt.metric_name = 'rpm' THEN mt.value END) as rpm,
  max(CASE WHEN mt.metric_name = 'current' THEN mt.value END) as current
FROM machines m
LEFT JOIN metrics mt ON m.id = mt.machine_id AND mt.time > now() - interval '1 minute'
GROUP BY m.id, m.name
ORDER BY m.name

-- Unacknowledged Alerts
SELECT 
  a.created_at,
  m.name as machine,
  a.severity,
  a.message
FROM alerts a
LEFT JOIN machines m ON m.id = a.machine_id
WHERE a.acknowledged = false
ORDER BY a.created_at DESC
LIMIT 20

-- Highest Temperature (worst offenders)
SELECT m.name as pump, round(max(mt.value)::numeric, 1) as max_temp_c
FROM metrics mt
JOIN machines m ON m.id = mt.machine_id
WHERE mt.metric_name = 'temperature' AND mt.time > now() - interval '5 minutes'
GROUP BY m.name
ORDER BY max_temp_c DESC
LIMIT 5

-- Highest Vibration (worst offenders)
SELECT m.name as pump, round(max(mt.value)::numeric, 1) as max_vibration
FROM metrics mt
JOIN machines m ON m.id = mt.machine_id
WHERE mt.metric_name = 'vibration' AND mt.time > now() - interval '5 minutes'
GROUP BY m.name
ORDER BY max_vibration DESC
LIMIT 5

-- Highest Pressure (worst offenders)
SELECT m.name as pump, round(max(mt.value)::numeric, 1) as max_pressure
FROM metrics mt
JOIN machines m ON m.id = mt.machine_id
WHERE mt.metric_name = 'pressure' AND mt.time > now() - interval '5 minutes'
GROUP BY m.name
ORDER BY max_pressure DESC
LIMIT 5

-- Highest Current (worst offenders)
SELECT m.name as pump, round(max(mt.value)::numeric, 1) as max_current
FROM metrics mt
JOIN machines m ON m.id = mt.machine_id
WHERE mt.metric_name = 'current' AND mt.time > now() - interval '5 minutes'
GROUP BY m.name
ORDER BY max_current DESC
LIMIT 5
