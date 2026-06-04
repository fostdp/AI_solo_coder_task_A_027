CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE IF NOT EXISTS stations (
    id VARCHAR(20) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    is_baseline BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS weather_data (
    time TIMESTAMPTZ NOT NULL,
    station_id VARCHAR(20) NOT NULL REFERENCES stations(id),
    temperature DOUBLE PRECISION NOT NULL,
    humidity DOUBLE PRECISION NOT NULL,
    wind_speed DOUBLE PRECISION NOT NULL,
    solar_radiation DOUBLE PRECISION NOT NULL
);

SELECT create_hypertable(
    'weather_data',
    'time',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE
);

ALTER TABLE weather_data SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'station_id',
    timescaledb.compress_orderby = 'time DESC'
);

SELECT add_compression_policy(
    'weather_data',
    compress_after => INTERVAL '7 days',
    if_not_exists => TRUE
);

SELECT add_retention_policy(
    'weather_data',
    drop_after => INTERVAL '90 days',
    if_not_exists => TRUE
);

CREATE INDEX idx_weather_data_station_time ON weather_data (station_id, time DESC);

CREATE TABLE IF NOT EXISTS heat_island_intensity (
    time TIMESTAMPTZ NOT NULL,
    urban_avg_temp DOUBLE PRECISION NOT NULL,
    baseline_avg_temp DOUBLE PRECISION NOT NULL,
    intensity DOUBLE PRECISION NOT NULL
);

SELECT create_hypertable(
    'heat_island_intensity',
    'time',
    chunk_time_interval => INTERVAL '1 week',
    if_not_exists => TRUE
);

CREATE TABLE IF NOT EXISTS station_heat_index (
    time TIMESTAMPTZ NOT NULL,
    station_id VARCHAR(20) NOT NULL REFERENCES stations(id),
    heat_index DOUBLE PRECISION NOT NULL
);

SELECT create_hypertable(
    'station_heat_index',
    'time',
    chunk_time_interval => INTERVAL '1 week',
    if_not_exists => TRUE
);

CREATE INDEX idx_station_heat_index_station_time ON station_heat_index (station_id, time DESC);

CREATE TABLE IF NOT EXISTS alerts (
    id SERIAL PRIMARY KEY,
    time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    alert_type VARCHAR(50) NOT NULL,
    level VARCHAR(20) NOT NULL,
    station_id VARCHAR(20),
    message TEXT NOT NULL,
    notified BOOLEAN NOT NULL DEFAULT FALSE
);

INSERT INTO stations (id, name, latitude, longitude, is_baseline) VALUES
('B001', '郊区基准站-东', 30.55, 114.38, TRUE),
('B002', '郊区基准站-南', 30.48, 114.42, TRUE),
('B003', '郊区基准站-西', 30.52, 114.30, TRUE),
('U001', '江汉路观测站', 30.58, 114.36, FALSE),
('U002', '解放大道观测站', 30.59, 114.37, FALSE),
('U003', '中山公园观测站', 30.575, 114.355, FALSE),
('U004', '武广商圈观测站', 30.582, 114.362, FALSE),
('U005', '汉口站观测站', 30.60, 114.38, FALSE),
('U006', '光谷广场观测站', 30.505, 114.40, FALSE),
('U007', '珞喻路观测站', 30.515, 114.395, FALSE),
('U008', '街道口观测站', 30.52, 114.385, FALSE),
('U009', '中南路观测站', 30.525, 114.37, FALSE),
('U010', '洪山广场观测站', 30.53, 114.365, FALSE),
('U011', '武昌站观测站', 30.54, 114.355, FALSE),
('U012', '徐东大街观测站', 30.555, 114.375, FALSE),
('U013', '岳家嘴观测站', 30.56, 114.385, FALSE),
('U014', '楚河汉街观测站', 30.545, 114.362, FALSE),
('U015', '水果湖观测站', 30.538, 114.355, FALSE),
('U016', '东湖高新观测站', 30.49, 114.42, FALSE),
('U017', '关山大道观测站', 30.495, 114.41, FALSE),
('U018', '南湖观测站', 30.50, 114.39, FALSE),
('U019', '白沙洲观测站', 30.51, 114.34, FALSE),
('U020', '汉阳大道观测站', 30.545, 114.33, FALSE),
('U021', '钟家村观测站', 30.55, 114.34, FALSE),
('U022', '墨水湖观测站', 30.535, 114.335, FALSE),
('U023', '王家湾观测站', 30.54, 114.325, FALSE),
('U024', '沌口观测站', 30.52, 114.30, FALSE),
('U025', '吴家山观测站', 30.56, 114.28, FALSE),
('U026', '常青路观测站', 30.57, 114.34, FALSE),
('U027', '范湖观测站', 30.575, 114.345, FALSE),
('U028', '西北湖观测站', 30.578, 114.35, FALSE),
('U029', '唐家墩观测站', 30.585, 114.355, FALSE),
('U030', '百步亭观测站', 30.595, 114.37, FALSE),
('U031', '后湖观测站', 30.605, 114.385, FALSE),
('U032', '二七路观测站', 30.598, 114.365, FALSE),
('U033', '青山广场观测站', 30.565, 114.395, FALSE),
('U034', '红钢城观测站', 30.57, 114.40, FALSE),
('U035', '和平大道观测站', 30.555, 114.39, FALSE),
('U036', '工业大道观测站', 30.568, 114.405, FALSE),
('U037', '钢花观测站', 30.572, 114.398, FALSE),
('U038', '杨园观测站', 30.578, 114.395, FALSE),
('U039', '友谊大道观测站', 30.565, 114.385, FALSE),
('U040', '建设大道观测站', 30.58, 114.36, FALSE),
('U041', '新华路观测站', 30.577, 114.348, FALSE),
('U042', '青年路观测站', 30.573, 114.34, FALSE),
('U043', '航空路观测站', 30.57, 114.335, FALSE),
('U044', '宝丰路观测站', 30.568, 114.33, FALSE),
('U045', '古田观测站', 30.565, 114.315, FALSE),
('U046', '宗关观测站', 30.558, 114.325, FALSE),
('U047', '硚口路观测站', 30.56, 114.335, FALSE),
('U048', '武胜路观测站', 30.563, 114.342, FALSE),
('U049', '江汉桥观测站', 30.555, 114.345, FALSE),
('U050', '月湖桥观测站', 30.55, 114.338, FALSE);

CREATE MATERIALIZED VIEW weather_30min_agg
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('30 minutes', time) AS bucket_start,
    station_id,
    AVG(temperature) AS avg_temp,
    MIN(temperature) AS min_temp,
    MAX(temperature) AS max_temp,
    AVG(humidity) AS avg_humidity,
    AVG(wind_speed) AS avg_wind_speed,
    AVG(solar_radiation) AS avg_solar_radiation,
    COUNT(*) AS record_count
FROM weather_data
GROUP BY bucket_start, station_id
WITH DATA;

CREATE UNIQUE INDEX idx_weather_30min_agg_station_time ON weather_30min_agg (station_id, bucket_start DESC);
CREATE INDEX idx_weather_30min_agg_bucket ON weather_30min_agg (bucket_start DESC);

SELECT add_continuous_aggregate_policy(
    'weather_30min_agg',
    start_offset => INTERVAL '1 hour',
    end_offset => INTERVAL '30 minutes',
    schedule_interval => INTERVAL '30 minutes',
    if_not_exists => TRUE
);

SELECT add_retention_policy(
    'weather_30min_agg',
    drop_after => INTERVAL '1 year',
    if_not_exists => TRUE
);

CREATE OR REPLACE PROCEDURE refresh_weather_agg()
LANGUAGE plpgsql
AS $$
BEGIN
    CALL refresh_continuous_aggregate('weather_30min_agg', NULL, NULL);
END;
$$;
