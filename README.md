# go-connect-template

一个基于 Go Connect 框架的微服务模板，用于快速构建现代化的云原生应用。

## 技术栈

- **语言**: Go 1.26+
- **RPC 框架**: Connect (connectrpc.com)
- **依赖注入**: Uber FX
- **数据库**: PostgreSQL (pgx/v5)
- **缓存**: Redis
- **搜索**: Elasticsearch
- **服务发现**: Consul
- **可观测性**: OpenTelemetry
- **日志**: Zap
- **配置**: Viper

## 项目结构

```
.
├── cmd/server/main.go          # 服务入口
├── api/                        # Protobuf API 定义
├── constants/                  # 常量定义
├── configs/                    # 配置文件
├── deploy/                     # 部署配置 (Kubernetes)
├── internal/
│   ├── biz/                    # 业务逻辑层
│   ├── conf/v1/               # 配置定义 (Protobuf)
│   ├── data/                   # 数据访问层
│   ├── pkg/                    # 工具包
│   │   ├── config/             # 配置管理
│   │   ├── log/                # 日志封装
│   │   ├── meta/               # 元信息
│   │   ├── otel/               # OpenTelemetry
│   │   └── registry/           # 服务注册
│   ├── server/                 # HTTP 服务器
│   └── service/                # 业务服务层
├── third_party/                # 第三方 Protobuf 定义
├── buf.yaml                    # Buf 配置
├── buf.gen.yaml               # Buf 生成配置
├── compose.yaml               # Docker Compose
├── Dockerfile                 # Docker 镜像构建
├── go.mod                     # Go 依赖
├── Makefile                   # 构建脚本
└── sqlc.yaml                  # SQLC 配置
```

## 功能特性

- ✅ **Connect RPC**: 高性能 RPC 框架，支持 gRPC、gRPC-Web 和 Connect 协议
- ✅ **依赖注入**: 使用 FX 实现声明式依赖管理
- ✅ **多数据源**: PostgreSQL + Redis + Elasticsearch
- ✅ **服务发现**: Consul 集成，支持健康检查和自动注销
- ✅ **可观测性**: 完整的 OTel 追踪、指标和日志支持
- ✅ **配置管理**: 支持本地文件和 Consul KV 配置中心
- ✅ **健康检查**: 数据库、缓存、ES 的健康检查端点
- ✅ **中间件**: 请求日志、CORS、错误处理
- ✅ **优雅关闭**: 7 秒超时的优雅关闭流程

## 快速开始

### 本地开发

```bash
# 启动开发环境
make dev

# 或直接运行
SERVICE_NAME=org-service-v1 \
CONSUL_ENABLED=true \
CONSUL_ADDR=consul.example.com \
CONSUL_PATH=ecommerce/user/dev.yml \
CONSUL_SCHEME=https \
CONSUL_INSECURE_SKIP_VERIFY=true \
go run cmd/server/main.go
```

### 构建命令

```bash
# 运行测试
make test

# 生成 API 代码
make api

# 生成配置代码
make conf

# 生成 SQL 代码
make sqlc

# 构建 Docker 镜像
make docker-build

# 推送 Docker 镜像
make docker-push

# 部署到 Kubernetes
make k8s-dev
```

## 配置说明

配置支持从以下来源加载（优先级从高到低）：

1. 命令行参数
2. 环境变量
3. Consul KV（如果启用）
4. 本地配置文件 (`configs/`)

### 环境变量

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `SERVICE_NAME` | 服务名称 | org-service |
| `SERVICE_VERSION` | 服务版本 | v1 |
| `DEPLOYMENT_MODE` | 部署环境 | dev |
| `CONSUL_ENABLED` | 是否启用 Consul | false |
| `CONSUL_ADDR` | Consul 地址 | consul.example.com |
| `CONSUL_PATH` | 配置路径 | ecommerce/user/dev.yml |

## API 端点

- **健康检查**: `GET /healthz`
- **RPC 服务**: `POST /api.v1.ServiceName/MethodName`
- **Connect 调试**: `GET /connect-debug`

## 可观测性

### OpenTelemetry 配置

支持通过配置文件启用：

- **Trace**: 通过 OTLP HTTP 导出到 Collector
- **Metric**: 通过 OTLP HTTP 导出指标
- **Logging**: 通过 OTLP HTTP 导出日志

### 日志结构

日志输出为 JSON 格式，包含以下字段：

- `timestamp`: 时间戳
- `level`: 日志级别 (DEBUG/INFO/WARN/ERROR)
- `service`: 服务名称
- `trace_id`: 追踪 ID
- `span_id`: 跨度 ID
- `message`: 日志消息
- `fields`: 自定义字段

## 服务注册

服务启动时自动注册到 Consul，包含：

- 服务名称和版本
- 健康检查端点 (`/healthz`)
- TTL 健康检查（10秒）
- 服务标签

## 数据库配置

支持 PostgreSQL SSL 连接：

- **disable**: 禁用 SSL
- **require**: 要求 SSL，但不验证证书
- **verify-ca**: 验证 CA 证书
- **verify-full**: 验证 CA 证书和域名

## 贡献

欢迎提交 Issue 和 Pull Request！

## 许可证

MIT License