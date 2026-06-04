# 城市气候监测与热岛效应分析系统

基于 MQTT + TimescaleDB + Go + Leaflet 的城市热岛效应实时监测系统。

## 📐 系统架构

```
┌─────────────────────────────────────────────────────────────────┐
│                        气象站设备层                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐       │
│  │ 站点 U001│  │ 站点 U002│  │ 站点 U003│  │ 站点 B001│       │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘       │
└───────┼──────────────┼──────────────┼──────────────┼───────────┘
        │              │              │              │
        └──────────────┴──────┬───────┴──────────────┘
                              │ MQTT
                        ┌─────▼─────┐
                        │  EMQX     │  MQTT Broker
                        └─────┬─────┘
                              │
                    ┌─────────▼─────────┐
                    │  mqtthub.Hub      │  消息总线
                    │  (连接管理/分发)  │
                    └─────────┬─────────┘
                              │
          ┌───────────────────┼───────────────────┐
          │                   │                   │
  ┌───────▼───────┐   ┌───────▼───────┐   ┌───────▼───────┐
  │ datacleaner   │   │ heatisland    │   │ alarmengine   │
  │ (校验+批量入库)│   │ (热岛计算)     │   │ (告警+推送)    │
  └───────┬───────┘   └───────┬───────┘   └───────┬───────┘
          │                   │                   │
          └───────────────────┼───────────────────┘
                              │
                    ┌─────────▼─────────┐
                    │  TimescaleDB      │  时序数据库
                    │  (压缩+物化视图)  │
                    └─────────┬─────────┘
                              │
                    ┌─────────▼─────────┐
                    │  Go HTTP Server   │  REST API
                    │  (Gzip+Cache)     │
                    └─────────┬─────────┘
                              │
                    ┌─────────▼─────────┐
                    │  Leaflet 前端     │
                    │  (热力图/趋势图)  │
                    └───────────────────┘
```

## 📁 项目结构

```
.
├── backend/                    # Go 后端
│   ├── main.go                # 应用入口
│   ├── go.mod                 # 依赖
│   ├── Dockerfile             # 多阶段构建
│   ├── api/                   # HTTP API
│   │   └── handler.go
│   ├── db/                    # 数据库连接
│   │   └── db.go
│   └── pkg/                   # 业务模块
│       ├── mqtthub/           # MQTT 消息总线
│       │   └── hub.go
│       ├── datacleaner/       # 数据清洗管道
│       │   └── cleaner.go
│       ├── heatisland/        # 热岛强度计算
│       │   └── calculator.go
│       └── alarmengine/       # 告警引擎
│           └── engine.go
├── frontend/                   # 前端
│   ├── index.html             # 主页面
│   ├── heatmap.js             # 热力图组件
│   └── trendchart.js          # 趋势图组件
├── db/                         # 数据库脚本
│   └── init.sql               # 初始化 + 压缩策略
├── mqtt_simulator/             # MQTT 模拟器
│   ├── simulator.py           # 模拟器主程序
│   ├── requirements.txt
│   └── Dockerfile
├── docker-compose.yml          # 编排配置
├── .env.example               # 环境变量示例
└── README.md
```

## 🚀 快速开始

### 环境要求

- Docker 20.10+
- Docker Compose 2.0+
- 至少 4GB 可用内存

### 1. 配置环境变量

```bash
cp .env.example .env
# 编辑 .env 根据需要调整
```

### 2. 启动核心服务

```bash
# 启动数据库 + MQTT Broker + 后端
docker-compose up -d timescaledb emqx backend

# 查看服务状态
docker-compose ps

# 查看日志
docker-compose logs -f backend
```

### 3. 启动模拟器（可选）

```bash
# 启动模拟器（默认 50 站点，5 分钟间隔）
docker-compose --profile simulator up -d

# 或使用自定义配置
cd mqtt_simulator
pip install -r requirements.txt

# 查看所有选项
python simulator.py --help

# 示例：10 个站点，1 分钟间隔
python simulator.py --stations 10 --interval 60

# 示例：从配置文件加载
python simulator.py --load-config my_stations.json
```

### 4. 访问应用

| 服务 | 地址 | 说明 |
|------|------|------|
| 前端/API | http://localhost:8080 | 主应用 |
| 健康检查 | http://localhost:8080/health | 服务状态 |
| pprof | http://localhost:6060/debug/pprof | 性能分析 |
| EMQX Dashboard | http://localhost:18083 | MQTT 控制台 (admin/public) |
| PostgreSQL | localhost:5432 | 数据库端口 |

## ⚙️ 配置说明

### 后端环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `DB_HOST` | `timescaledb` | 数据库主机 |
| `DB_PORT` | `5432` | 数据库端口 |
| `DB_USER` | `postgres` | 数据库用户 |
| `DB_PASSWORD` | `postgres` | 数据库密码 |
| `DB_NAME` | `climate_monitor` | 数据库名 |
| `MQTT_BROKER` | `tcp://emqx:1883` | MQTT 代理地址 |
| `MQTT_TOPIC` | `weather/stations/#` | 订阅主题 |
| `SERVER_PORT` | `8080` | HTTP 端口 |
| `PPROF_ENABLED` | `true` | 启用性能分析 |
| `DINGTALK_WEBHOOK_URL` | `''` | 钉钉告警 Webhook |

### 模拟器参数

```bash
python simulator.py \
  --broker localhost       # MQTT 代理地址
  --port 1883             # MQTT 端口
  --stations 50           # 站点数量
  --interval 300          # 发布间隔（秒）
  --temp-min 15.0         # 最低温度
  --temp-max 40.0         # 最高温度
  --center-lat 30.55      # 中心纬度
  --center-lon 114.35     # 中心经度
  --radius-km 10          # 模拟半径（公里）
  --save-config stations.json  # 保存配置
  --load-config stations.json  # 加载配置
  --list-only             # 仅列出站点
```

## 📊 数据库优化

### 压缩策略

- **weather_data**：7 天后自动压缩
- **压缩率**：~90%（segmentby=station_id, orderby=time）
- **保留策略**：90 天后自动删除

### 持续聚合

- **物化视图**：`weather_30min_agg`
- **刷新间隔**：每 30 分钟自动刷新
- **数据粒度**：30 分钟桶聚合
- **查询加速**：10-100 倍性能提升

### 自动分区

- **weather_data**：按天分区
- **heat_island_intensity**：按周分区
- **索引优化**：复合索引 + 降序排序

## 🔧 运维操作

### 查看服务状态

```bash
docker-compose ps
docker-compose logs -f [service]
```

### 重启服务

```bash
docker-compose restart backend
```

### 升级服务

```bash
git pull
docker-compose build backend
docker-compose up -d backend
```

### 数据备份

```bash
# 备份数据库
docker exec climate-timescaledb pg_dump -U postgres climate_monitor > backup.sql

# 恢复数据库
docker exec -i climate-timescaledb psql -U postgres climate_monitor < backup.sql
```

### 查看物化视图

```sql
-- 查询聚合视图
SELECT * FROM weather_30min_agg
WHERE station_id = 'U001'
ORDER BY bucket_start DESC
LIMIT 10;

-- 手动刷新
CALL refresh_weather_agg();
```

## 🔍 性能监控

### pprof 使用

```bash
# 查看 goroutine
go tool pprof http://localhost:6060/debug/pprof/goroutine?debug=2

# CPU 采样 30 秒
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# 内存分析
go tool pprof http://localhost:6060/debug/pprof/heap
```

### 健康检查

```bash
curl http://localhost:8080/health
# {
#   "status": "ok",
#   "version": "1.0.0",
#   "buildTime": "2024-01-01T00:00:00Z",
#   "time": "2024-01-01T12:00:00+08:00"
# }
```

## 📝 API 接口

### 站点信息

```http
GET /api/stations
GET /api/stations/{id}
```

### 实时数据

```http
GET /api/weather/latest
GET /api/weather/latest/{stationId}
```

### 历史数据

```http
GET /api/weather/history/{stationId}?hours=24
GET /api/weather/aggregate/{stationId}?bucket=30m&hours=24
```

### 热岛指数

```http
GET /api/heat-island/current
GET /api/heat-island/history?hours=24
GET /api/station-heat-index/{stationId}?hours=24
```

### 告警

```http
GET /api/alerts?limit=10
GET /api/alerts/count?hours=24
```

## 🚨 告警规则

| 类型 | 阈值 | 级别 | 说明 |
|------|------|------|------|
| 高温告警 | > 35℃ 持续 1 小时 | WARNING | 单站高温 |
| 高温告警 | > 40℃ 持续 30 分钟 | CRITICAL | 极端高温 |
| 热岛告警 | 强度 > 2℃ 持续 1 小时 | WARNING | 显著热岛 |
| 热岛告警 | 强度 > 5℃ 持续 30 分钟 | CRITICAL | 严重热岛 |

告警通过钉钉 Webhook 推送，支持 3 次指数退避重试。

## 🛠️ 开发指南

### 本地开发

```bash
# 启动依赖
docker-compose up -d timescaledb emqx

# 运行后端
cd backend
go run main.go

# 运行模拟器
cd mqtt_simulator
pip install -r requirements.txt
python simulator.py --stations 10 --interval 10
```

### 构建镜像

```bash
# 后端
docker build -t climate-backend ./backend

# 模拟器
docker build -t climate-simulator ./mqtt_simulator
```

### 运行测试

```bash
cd backend
go test ./...
```

## 📄 许可证

MIT License
