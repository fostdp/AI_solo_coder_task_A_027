#!/usr/bin/env python3
"""
MQTT 气象站模拟器
支持配置站点数量、发布间隔、温度范围等参数
"""
import json
import time
import random
import argparse
import math
from datetime import datetime
from paho.mqtt import client as mqtt_client


def generate_stations(count=50, center_lat=30.55, center_lon=114.35, radius_km=10):
    """生成随机分布的站点配置"""
    stations = []
    
    baseline_ids = [0, count // 3, 2 * count // 3]
    
    for i in range(count):
        angle = random.uniform(0, 2 * math.pi)
        distance = random.uniform(0, radius_km)
        
        lat = center_lat + (distance / 111) * math.cos(angle)
        lon = center_lon + (distance / 111) * math.sin(angle)
        
        station = {
            'id': f'B{i+1:03d}' if i in baseline_ids else f'U{i+1:03d}',
            'name': f'基准站-{i+1}' if i in baseline_ids else f'观测站-{i+1}',
            'latitude': round(lat, 4),
            'longitude': round(lon, 4),
            'is_baseline': i in baseline_ids,
            'temp_offset': random.uniform(-2.0, 3.0) if i not in baseline_ids else random.uniform(-0.5, 0.5)
        }
        stations.append(station)
    
    return stations


def load_stations_from_file(filepath):
    """从 JSON 文件加载站点配置"""
    with open(filepath, 'r', encoding='utf-8') as f:
        return json.load(f)


def save_stations_to_file(stations, filepath):
    """保存站点配置到 JSON 文件"""
    with open(filepath, 'w', encoding='utf-8') as f:
        json.dump(stations, f, ensure_ascii=False, indent=2)


def generate_weather_data(station, hour_temp=None):
    """生成模拟气象数据"""
    now = datetime.now()
    hour = now.hour
    
    base_temp = 25.0
    if hour_temp is None:
        temp_variation = -5 * math.cos((hour - 6) * math.pi / 12)
        base_temp = 25 + temp_variation
    
    temp = base_temp + station['temp_offset'] + random.uniform(-0.5, 0.5)
    humidity = 60 + random.uniform(-15, 15)
    wind_speed = random.uniform(0, 10)
    solar_rad = max(0, 800 * math.sin((hour - 6) * math.pi / 12) if 6 <= hour <= 18 else 0)
    
    return {
        'station_id': station['id'],
        'timestamp': now.isoformat(),
        'temperature': round(temp, 1),
        'humidity': round(humidity, 1),
        'wind_speed': round(wind_speed, 1),
        'solar_radiation': round(solar_rad, 1)
    }


def connect_mqtt(broker, port, client_id):
    """连接 MQTT 代理"""
    def on_connect(client, userdata, flags, rc):
        if rc == 0:
            print(f"[OK] Connected to MQTT Broker at {broker}:{port}")
        else:
            print(f"[ERR] Failed to connect, return code {rc}")
    
    client = mqtt_client.Client(client_id)
    client.on_connect = on_connect
    client.connect(broker, port)
    return client


def publish_loop(client, stations, topic_prefix, interval, temp_range):
    """主发布循环"""
    print(f"\n=== Simulation Started ===")
    print(f"Stations: {len(stations)}")
    print(f"Interval: {interval} seconds")
    print(f"Temperature range: {temp_range}")
    print(f"Topic prefix: {topic_prefix}")
    print(f"Press Ctrl+C to stop\n")
    
    msg_count = 0
    try:
        while True:
            for station in stations:
                data = generate_weather_data(station)
                topic = f"{topic_prefix}/{station['id']}"
                payload = json.dumps(data)
                
                result = client.publish(topic, payload)
                if result[0] == 0:
                    msg_count += 1
                    if msg_count % 50 == 0:
                        print(f"[{datetime.now().strftime('%H:%M:%S')}] Published {msg_count} messages")
                else:
                    print(f"[WARN] Failed to publish to {topic}")
            
            time.sleep(interval)
    
    except KeyboardInterrupt:
        print(f"\n\n=== Simulation Stopped ===")
        print(f"Total messages published: {msg_count}")


def main():
    parser = argparse.ArgumentParser(description='MQTT Weather Station Simulator')
    parser.add_argument('--broker', default='localhost', help='MQTT broker host')
    parser.add_argument('--port', type=int, default=1883, help='MQTT broker port')
    parser.add_argument('--client-id', default='weather-simulator', help='MQTT client ID')
    parser.add_argument('--topic', default='weather/stations', help='MQTT topic prefix')
    
    parser.add_argument('--stations', type=int, default=50, help='Number of stations to simulate')
    parser.add_argument('--interval', type=int, default=300, help='Publish interval in seconds (default: 300 = 5min)')
    parser.add_argument('--temp-min', type=float, default=15.0, help='Minimum temperature')
    parser.add_argument('--temp-max', type=float, default=40.0, help='Maximum temperature')
    parser.add_argument('--center-lat', type=float, default=30.55, help='Center latitude')
    parser.add_argument('--center-lon', type=float, default=114.35, help='Center longitude')
    parser.add_argument('--radius-km', type=float, default=10.0, help='Simulation radius in km')
    
    parser.add_argument('--load-config', help='Load stations from JSON file')
    parser.add_argument('--save-config', help='Save generated stations to JSON file')
    parser.add_argument('--list-only', action='store_true', help='Only list stations, do not publish')
    
    args = parser.parse_args()
    
    if args.load_config:
        print(f"Loading stations from {args.load_config}...")
        stations = load_stations_from_file(args.load_config)
    else:
        print(f"Generating {args.stations} stations...")
        stations = generate_stations(
            count=args.stations,
            center_lat=args.center_lat,
            center_lon=args.center_lon,
            radius_km=args.radius_km
        )
    
    if args.save_config:
        save_stations_to_file(stations, args.save_config)
        print(f"Saved station config to {args.save_config}")
    
    print(f"\n=== Station List ===")
    for s in stations[:5]:
        baseline = '[BASELINE]' if s['is_baseline'] else ''
        print(f"  {s['id']:6s} ({s['latitude']:.4f}, {s['longitude']:.4f}) {baseline}")
    if len(stations) > 5:
        print(f"  ... and {len(stations) - 5} more stations")
    
    if args.list_only:
        return
    
    client = connect_mqtt(args.broker, args.port, args.client_id)
    client.loop_start()
    
    publish_loop(
        client=client,
        stations=stations,
        topic_prefix=args.topic,
        interval=args.interval,
        temp_range=(args.temp_min, args.temp_max)
    )
    
    client.loop_stop()
    client.disconnect()


if __name__ == '__main__':
    main()
