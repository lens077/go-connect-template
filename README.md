# go-connect-template

一个基于 Go Connect 框架的微服务模板，用于快速构建现代化的云原生应用。

本仓库同时是 [`co-cli`](https://github.com/lens077/go-connect-template-cli) 的模板源：它**始终保持所有能力都打开且
`go build ./...` 通过**的状态，`co` 只按 `.co/manifest.yaml` 做减法（删文件、删 `+co:` 标记行、
删 `go.mod` 依赖）。所以这里编译得过，裁剪出来的服务就编译得过。

```bash
go install github.com/lens077/go-connect-template-cli@latest   # 需要 co >= v0.2.0(manifest v3)
co new cart --module github.com/acme/shop --yes
```

## 技术栈

- **语言**: Go 1.26+
- **RPC 框架**: Connect (connectrpc.com)
- **依赖注入**: Uber FX
- **数据库**: PostgreSQL (pgx/v5 + sqlc)
- **缓存**: Redis
<!-- +co:begin elasticsearch -->
- **搜索**: Elasticsearch v9
<!-- +co:end -->
<!-- +co:begin meilisearch -->
- **搜索**: Meilisearch
<!-- +co:end -->
- **身份认证**: Casdoor
- **服务发现**: Consul
- **可观测性**: OpenTelemetry
- **日志**: Zap
- **参数校验**: protovalidate（`connectrpc.com/validate` 拦截器）
- **共享基础设施**: `go-connect-kit` 提供配置、日志、OTel、Consul、数据库错误映射、环境变量与构建元信息
- **配置源**: kit 提供 file source；`control-tower/sdk/configsource` 适配 Config Center

> 数据库目前只提供 PostgreSQL，mysql / sqlite 见 `TODO.md`。

## 项目结构

```
.
├── .co/                        # co-cli 的裁剪契约（. 开头，Go 工具链忽略，不参与编译）
│   ├── manifest.yaml           # feature → 文件 / go.mod 依赖 / 布局 / 锚点 / hook
│   └── scaffold/               # 生成新资源与 monorepo overlay 的模板
├── cmd/server/main.go          # 服务入口
├── api/                        # Protobuf API 定义
│   └── search/v1/              # 示例资源（co new 默认删掉，--keep-example 可留）
├── constants/                  # 常量与环境变量名
├── configs/                    # dev.yml / pre.yml / source.dev.yaml.example（勿提交真实凭据）
├── deploy/                     # 部署配置 (Kubernetes)
├── infrastructure/             # 本地依赖的 compose（postgres / redis / 检索后端 / consul）
├── internal/
│   ├── biz/                    # 业务逻辑层
│   ├── conf/v1/                # 配置结构（由 conf.proto 生成）
│   ├── data/                   # 数据访问层，一个 feature 一个文件
│   │   ├── db_postgres.go      #   PostgreSQL（含 TLS / verify-ca）
│   │   ├── cache_redis.go      #   Redis
│   │   ├── search_catalog.go       #   项目自有 interface 与 DTO
│   │   ├── search_elasticsearch.go #   检索 adapter <!-- +co:elasticsearch -->
│   │   ├── search_meilisearch.go   #   检索 adapter <!-- +co:meilisearch -->
│   │   ├── auth_casdoor.go
│   │   ├── migrations/             #   建表 DDL，000NN_ 前缀决定 sqlc 的读取顺序
│   │   ├── queries/            #   sqlc 查询
│   │   └── models/             #   sqlc 生成物
│   ├── pkg/                    # 项目自有工具与 kit adapter
│   │   ├── config/             #   Bootstrap 泛型实例化 + restart-required 投影
│   │   ├── log/                #   confv1.Log → kit/log Options
│   │   ├── otel/               #   confv1.Observability → kit/otel Options
│   │   ├── registry/           #   confv1.Discovery → kit/registry Options
│   │   ├── minio.go            #   对象存储 URL 拼接（只存 key，域名放配置）
│   │   └── money/              #   NUMERIC ↔ decimal 转换
│   ├── server/                 # HTTP 服务器 + 健康检查
│   └── service/                # Connect handler
├── third_party/                # 第三方 Protobuf 定义
├── buf.yaml / buf.gen.yaml     # Buf 配置与生成配置
├── compose.yaml                # Docker Compose
├── Dockerfile                  # Docker 镜像构建
├── Makefile                    # 构建脚本
├── sqlc.yaml                   # SQLC 配置
└── TODO.md                     # 改造记录与待办
```

## 功能特性

- ✅ **Connect RPC**: 高性能 RPC 框架，支持 gRPC、gRPC-Web 和 Connect 协议
- ✅ **依赖注入**: 使用 FX 实现声明式依赖管理
- ✅ **多数据源**: PostgreSQL + Redis + 一个生成期选定的检索 adapter
- ✅ **服务发现**: Consul 集成，支持健康检查和自动注销
- ✅ **可观测性**: 完整的 OTel 追踪、指标和日志支持
- ✅ **配置管理**: kit 的严格加载与 LKG 热更新，本地文件 / Config Center selector 二选一
- ✅ **健康检查**: 数据库、缓存和所选检索后端的健康检查端点
- ✅ **参数校验**: protobuf 里声明约束，拦截器统一拒绝非法请求
- ✅ **中间件**: 请求日志、CORS、错误处理
- ✅ **优雅关闭**: 7 秒超时的优雅关闭流程

## 快速开始

### 本地开发

外部组件在 `infrastructure/` 下各有一份 compose，凭据与 `configs/dev.yml` 已经对好，起来就能用：

```bash
# postgres 与 redis 是启动硬依赖：连不上服务不会起来
docker compose -f infrastructure/postgres/compose.yaml up -d
docker compose -f infrastructure/redis/compose.yaml up -d
```

检索后端是可降级依赖：连不上只让检索功能不可用，不阻塞启动。

<!-- +co:begin elasticsearch -->
```bash
docker compose -f infrastructure/elasticsearch/compose.yaml up -d
```
<!-- +co:end -->
<!-- +co:begin meilisearch -->
```bash
docker compose -f infrastructure/meilisearch/compose.yaml up -d
```
<!-- +co:end -->

<!-- +co:begin elasticsearch,meilisearch -->
模板同时保留两个检索 adapter，`co new --search elasticsearch|meilisearch` 是**生成期选择**，
不是运行时开关。生成物只保留所选 adapter 的 Go 文件、依赖、compose 与文档段落。
<!-- +co:end -->
仓储层统一通过项目自有的 `SearchCatalog` interface 调用检索后端，不接触厂商 SDK 类型。

迁移使用 goose 版本文件，存放在 `internal/data/migrations/`；示例种子存放在
`internal/data/seeds/`。生产服务必须通过迁移命令升级，不能依赖容器首次启动脚本。

<!-- +co:begin elasticsearch -->
> Elasticsearch 镜像的大版本必须跟 `go.mod` 里的客户端对齐（现在是 `go-elasticsearch/v9` → Elasticsearch 9.x）。
> v9 客户端会发 `compatible-with=9` 的 Accept 头，8.x 服务端不认，连 Ping 都返回 400。
<!-- +co:end -->
<!-- +co:begin meilisearch -->
> 本地 Meilisearch 以 `MEILI_ENV=development` 启动、不设 master key，`search.catalog.api_key` 留空即可。
> 生产环境必须设 master key，并通过 Config Center / Secret 注入 `api_key`。
<!-- +co:end -->

Consul 的 compose 在 `infrastructure/consul/`，但 `make dev` 用不到它：配置从本地文件读，
`CONSUL_ENABLED=false` 也不做服务注册。要验注册时再起。

```bash
# 默认走本地文件配置，不接配置中心
make dev

# 等价于
SERVICE_NAME=org-service-v1 \
CONFIG_SOURCE=file \
CONFIG_FILE=configs/dev.yml \
go run cmd/server/main.go

# 经 selector 从 Config Center 拉 Bootstrap（首次使用）
cp configs/source.dev.yaml.example configs/source.dev.yaml
# 只在已被 gitignore 的 source.dev.yaml 里填 service_token
make dev-cc
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

# 部署到 Kubernetes（先创建 example-config-source Secret）
make k8s-dev
```

> `api` / `conf` 两个目标都带 `--path`，这不是可省的优化：`third_party/google/protobuf/` 下那份
> WKT 副本与 buf 内置的同名，裸跑 `buf generate` 会直接报 `name conflict over google.protobuf.Any`。

## 配置说明

整份 `Bootstrap` 配置从**一个**数据源取回。生产路径由 `CONFIG_SOURCE_FILE` 指向一份本地 selector，`type` 必须是 `config_center`；本地测试显式设 `CONFIG_SOURCE=file`。`make dev` 已设置 file 所需变量；直接运行二进制时必须明确选择来源，不做静默默认。

| 路径 | 说明 | 需要的环境变量 |
|---|---|---|
| `CONFIG_SOURCE_FILE`（生产） | Config Center SDK，selector 的 `type` 必须是 `config_center` | Secret 挂载的 selector 内含 address / namespace / environment / key / service_token |
| `CONFIG_SOURCE=file`（本地） | 读本地 YAML，不接配置中心 | 必须设置 `CONFIG_FILE`；`make dev` 使用 `configs/dev.yml` |

`CONFIG_SOURCE=configcenter` 与 Consul KV 作为配置源均已退役：前者请改挂 selector 文件，后者 Consul 只保留服务注册/发现。

刻意**不做**「主源失败自动降级到备源」：配置来源必须是确定的。静默降级会让服务拿着一份你以为早已废弃的配置正常跑起来，比直接启动失败难排查得多。

远端配置支持 Watch 热更新。`*Bootstrap` 是启动快照；需要随时读到最新值的消费者注入 `*Live`。`server` / `discovery` / `observability` / `search` 变更会打警告，需要滚动重启才生效。

### 环境变量

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `SERVICE_NAME` | 服务名称 | org-service |
| `SERVICE_VERSION` | 服务版本 | v1 |
| `DEPLOYMENT_MODE` | 部署环境 | dev |
| `CONFIG_SOURCE_FILE` | Config Center selector 路径 | 无 |
| `CONFIG_SOURCE` | 仅本地 `file`；`configcenter` 已废弃 | 无 |
| `CONFIG_FILE` | `file` 源的路径 | 无 |
| `CONSUL_ENABLED` | 是否向 Consul 注册服务 | false |
| `CONSUL_ADDR` | Consul 地址（只用于注册发现） | consul.example.com |

> `CONSUL_ENABLED` 只管**服务注册**，与配置来源无关。

`configs/source.dev.yaml.example` 只放占位值。真实 selector 用同目录下被忽略的
`source.dev.yaml`，部署时通过 Secret 挂载；`service_token` 不得入库。

## API 端点

- **健康检查**: `GET /healthz` — 逐项返回各基础设施状态，任一不健康时返回 503
- **RPC 服务**: `POST /<proto package>.<Service>/<Method>`（Connect / gRPC / gRPC-Web 三协议同端口）

服务同端口支持 HTTP/1.1 与 h2c（明文 HTTP/2）。

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

`internal/data/migrations/` 下的建表 DDL **文件名带 `000NN_` 前缀**：sqlc 按文件名排序读整个目录，
序号就是建表顺序，后建的表引用先建的表时靠它保证外键建得起来。`co resource add` 会自动接着往下排。

<!-- +co:begin elasticsearch,meilisearch -->
## 作为模板库维护

改这个仓库时有两条硬约束：

1. **任何改动之后 `go build ./...`、`go vet ./...` 与 `go test ./...` 都要通过。** 模板源码同时保留所有 feature
   和候选 adapter，默认装配其中一个；模板自身通过后，还要让 CLI 生成矩阵验证各互斥产物（矩阵会对每个产物跑 `go test`）。
2. **挪文件、加依赖要同步改 `.co/manifest.yaml`。** CLI 里不写死任何路径，两边的契约只有这一份。
   **测试文件也一样**：`internal/data/*_test.go` 各自登记在所属 feature 的 `files` 下，随 adapter 一起去留；
   test-only 依赖（如 miniredis）登记在该 feature 的 `requires` 下。

### 生成物自带的测试

- `cmd/server/main_test.go`：`fx.ValidateApp` 校验依赖图闭合。**不带标记，所有生成物都有。**
  fx 的注入在运行时用反射解析，`go build`/`go vet` 看不见「某构造函数要的参数没人 provide」——
  曾有一次这类错误让所有生成服务在 fx 构建阶段就起不来，而 CI 全绿。
- `internal/data/*_test.go`：各 adapter 的契约测试，用 httptest / miniredis 假服务，不依赖外部进程。
  随 feature 裁剪：选 Elasticsearch 的产物里没有 Meilisearch 的测试，反之亦然。

### `+co:` 标记

标记是各语言里合法的注释，所以模板照常编译。CLI 按启用的 feature 删掉对应的行或块：

```go
var Module = fx.Provide(
    NewData,
    NewRedisClient,   // +co:redis
    NewSearchCatalog, // +co:elasticsearch|meilisearch
    // +co:anchor data-providers
)

// +co:begin minio
func NewMinioClient(...) { ... }
// +co:end
```

- **行标记** `// +co:<feature-expression>` —— feature 表达式成立时只摘掉标记本身，否则整行删掉。
  逗号是「与」，竖线是「或」；`elasticsearch|meilisearch` 表示任一项启用。标记必须是行尾最后一个 token。
- **块标记** `// +co:begin <feature-expression>` … `// +co:end` —— 可嵌套，两行标记本身也会消失。
- **锚点** `// +co:anchor <name>` —— 插入点，裁剪后**保留**，留给 `co resource add`。

YAML / Makefile / Dockerfile / `.gitignore` 用 `#`，SQL 用 `--`，语义完全相同。
Markdown 用 HTML 注释 `<!-- +co:... -->`：块标记独占一行，行标记放行尾，必须以 `-->` 闭合。
渲染时注释不可见，模板 README 照常阅读；生成物只保留所选 adapter 的段落。

`.proto` 刻意**不参与**裁剪：protoc 会移动注释，`begin`/`end` 的配对关系在生成物里会断掉。
因此互斥实现共用 vendor-neutral `Search.Catalog` 配置，不把任一厂商的 message 留在另一种生成物中。

### 本地验证 CLI 的裁剪结果

改完模板不必先 push：

```bash
co new demo --template-dir ../go-connect-template --module github.com/acme/demo --search meilisearch --yes
cd demo && go build ./...
```
<!-- +co:end -->

## 待办

见 [`TODO.md`](TODO.md)——改造记录、已知问题与未做项。

## 贡献

欢迎提交 Issue 和 Pull Request！

## 许可证

MIT License
