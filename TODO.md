# 改造记录与待办

本轮目标:参照 `ecommerce/backend/services/cart` 把模板补齐到最新形态,同时**不破坏它作为模板库的属性** ——
每一步之后 `go build ./...` 与 `go vet ./...` 都必须通过。

裁剪逻辑全部外置到 `.co/`(以 `.` 开头,Go 工具链整个忽略),模板本体始终是一份
「所有 feature 源码都保留且能编译」的参考实现；互斥 adapter 在原始模板中按默认项装配。CLI 侧的改动见 `../go-connect-template-cli/TODO.md`。

数据库本轮只做 PostgreSQL,mysql / sqlite 按要求延后。

---

## 已完成(按执行顺序)

### 1. 可插拔配置源

- [x] 配置源接口、文件源与解析实现已迁入 `go-connect-kit/config`；模板只实例化 `Bootstrap` 并映射 restart-required 段
- [x] 新增 `source_consul.go`,把原 `consul_config.go` 的 Consul 客户端构造搬进来,**删除** `consul_config.go`
- [x] 新增 `source_file.go`(模板独有,零外部依赖):`CONFIG_SOURCE=file` + `CONFIG_FILE`,克隆下来不装 Consul 也能起服务
- [x] 新增 `source_configcenter.go`:从 config-service 按 `namespace/environment/key` 拉配置
- [x] 改 `config.go`:`Init` 改为「选源 → 解码 → 加锁落盘」,新增 `SourceName()`
- [x] `constants/`:补 `EnvConfigSource` / `EnvConfigFile` / `EnvConfigCenter*` / `ConfigSource{File,Consul,ConfigCenter}` / `DefaultConfigSource` / `ConfigFileFormat`
- [x] 刻意**不做**「主源失败自动降级备源」——静默降级会让服务拿着一份早已废弃的配置正常跑起来,比启动失败难查
- [x] `availableSources()` 用行标记维护:裁掉某个 `source_*.go` 后错误信息里的候选列表随之变短,不会指向不存在的选项

### 2. 数据层按驱动拆分

- [x] 从 `internal/data/data.go` 拆出 `db_postgres.go`(含 TLS / verify-ca 逻辑与事务),`data.go` 只留 `Data`、`NewData`、`CheckCache`、`dbErrHandler`
- [x] 同样拆出 `cache_redis.go`、`auth_casdoor.go`、`search_elasticsearch.go` —— 一个 feature 一个文件,裁剪时整文件删掉即可
- [x] 数据库错误映射统一由 `go-connect-kit/dbutil` 提供，模板不再保存实现副本

### 3. 从 cart 回流的修复

- [x] Consul 守护循环、心跳与注销实现已迁入 `go-connect-kit/registry`；模板只映射注册 Options
- [x] OTel 资源、导出管道、运行时指标及数据库/Redis 埋点辅助已迁入 `go-connect-kit/otel`
- [x] 新增 `internal/pkg/money/numeric.go`、`internal/pkg/minio.go`(+ 测试)
- [x] `conf.proto` 新增 `Store.Minio`(`default_domain` + `buckets` map),重新 `buf generate` 出 `conf.pb.go`

### 4. sqlc 全链路

- [x] 新增 `internal/data/migrations/`(`00001_products.sql`)、`internal/data/queries/`、`internal/data/models/`
- [x] `sqlc.yaml`:`analyzer.database: false`,离线靠 `migrations/` 推断类型,没起数据库时 `make sqlc` 也能跑

### 5. 配置文件与构建脚本

- [x] 新增 `configs/pre.yml`、`configs/.gitignore`
- [x] `Makefile`:`dev`(= `dev-file`)/ `dev-file` / `dev-cc` 两个本地目标
- [x] `api` / `conf` 目标都带 `--path` —— 不是可省的优化:`third_party/google/protobuf/` 下那份 WKT 副本与 buf 内置的同名,不加 `--path` 会直接报 `name conflict over google.protobuf.Any`

### 6. `.co/` 契约目录

- [x] `.co/manifest.yaml`:feature → 文件 / `go.mod` require / 依赖关系、groups、layouts、anchors、hooks、tools
- [x] `.co/scaffold/resource/*.tmpl`:生成一套资源的六个模板(proto / schema / queries / biz / data / service)
- [x] `.co/scaffold/monorepo/`:`Makefile.tmpl`、`Dockerfile.tmpl`
- [x] `.co/scaffold/README.md`

### 7. `+co:` 标记铺设

- [x] 各 `fx.Module` 的 provider 列表、`server/health.go` 的检查项、`server/server.go`、`configs/*.yml`、`Makefile`、`go.mod` 都加上行 / 块标记
- [x] 留下 8 个锚点供 `co resource add` 插入(见 `manifest.yaml` 的 `anchors`)
- [x] `.proto` 刻意**不参与**裁剪:protoc 会移动注释,`begin`/`end` 的配对关系在生成物里会断掉。互斥实现必须共用 vendor-neutral 配置,不能把另一厂商的 message 留进生成代码

### 8. 凭据清理

- [x] `configs/dev.yml` 全部替换为 example 值
- [x] `pg_ca_pem.crt` 从版本库移除
- [x] `sqlc.yaml` 保留 `database.uri` 字段(sqlc 需要它),值改成 example 串,并在注释里说明可用 `${DATABASE_URL}` 展开把真实凭据挪到环境变量

### 9. 配置中心契约（已由第 11 条取代）

- [x] ~~从 `ecommerce` 复制 `api/config/v1` 进模板~~ —— 现改用 config-center SDK 自带契约，不再复制 proto

### 11. Consul KV → Config Center SDK

对齐 `ecommerce/backend/services/cart`：

- [x] 删除 `source_consul.go` 与 `CONFIG_SOURCE=consul` / `CONSUL_PATH`。Consul 只保留服务注册/发现
- [x] 删除手写 `source_configcenter.go` 与复制的 `api/config/`
- [x] `internal/pkg/config/config.go` 经 `control-tower/sdk/configsource.NewKitSource` 读取 selector，`type` 必须是 `config_center`
- [x] `Live` 与 watch/LKG 实现迁入 kit；Config Center 支持热更新，file 源仍是启动读一次
- [x] `CONFIG_SOURCE=configcenter` 快速失败，提示改用 `CONFIG_SOURCE_FILE`
- [x] `make dev-cc` 改挂被忽略的 `configs/source.dev.yaml`;仓库只留 `.example`,避免机器 token 入库
- [x] 部署/compose 改挂 `CONFIG_SOURCE_FILE`;Kubernetes 补 0400 Secret volume 与非 root `fsGroup: 1000`
- [x] manifest：去掉 `config-consul` feature 与 `shared_proto: config`；`config-configcenter` 默认启用，
      裁剪时同时 DropRequire SDK 与 selector 专属 Make/compose/deploy 段

### 10. Elasticsearch v9 API 迁移

- [x] `NewTypedClient` / `Config` 已被官方标记 `// Deprecated:`,改用函数式选项 `elasticsearch.NewTyped(opts...)`
- [x] v9 **没有** `elasticsearch.WithTransport`,自定义 `http.RoundTripper` 要经
      `WithTransportOptions(elastictransport.WithTransport(rt))`;`elastic-transport-go/v8` 因此从间接依赖变成直接依赖,`manifest.yaml` 里跟着一起删
- [x] 地址为空时官方客户端会**静默**退回 `ELASTICSEARCH_URL` / `localhost:9200`,改为显式报错
- [x] 补上能真正跑通的示例:`EnsureIndex`(带显式 mapping,标识符字段用 keyword)、`IndexProduct`(`refresh=wait_for`)、`SearchUseCase.Reindex`。原来只有 `Search`,空集群必然返回 `index_not_found_exception`,看着像客户端配错了
- [x] 修 ES 请求日志两个真实 panic:连接失败时 `res` 为 nil(ES 一挂,日志钩子先崩,真正的错误被吞掉);配置访问器改为 nil 安全。实现现位于 `internal/data/search_elasticsearch_log.go`
- [x] 修 `internal/data/search.go`:`hit.Score_` 是指针,排序查询或 `track_scores=false` 时为 nil

---

## 待办

### 需要决定的

- [ ] **mysql / sqlite**:按要求延后。补充方式见 `manifest.yaml` 里 `groups.database` 的注释 ——
      驱动文件要放进模板本体(而不是 `.co/` 下)才会被模板自己的 CI 编译到
- [ ] **`otel` 目前不是可裁剪的 feature**:它被 `server`、`data`、`main.go` 多处引用,做成可裁剪要动的地方比一个 feature 该有的多
- [ ] **monorepo 下 `constants/` 仍是每个服务各带一份**,没有共享 `backend/constants`。共享要先确定谁是所有者,以及跨服务改常量的影响面

### 已知问题

- [ ] **裸 `buf generate` 在本仓库不可用**,必须带 `--path`(原因见上文第 5 条)。这是 `third_party` 里 WKT 副本与 buf 内置 WKT 冲突导致的,根治要么删掉那份副本、要么在 `buf.yaml` 里排除它
- [ ] **`third_party/google/api/annotations.proto` 的 import 是坏的**(`ecommerce` 里同样)。目前没被引用所以不报错

### 未做的小项

- [x] 不迁移 cart 的状态枚举转换：它属于 cart 领域，已留在 cart data；共享 `dbutil` 不接收业务枚举
- [ ] `Makefile` 没有 `k8s-prod` 目标(只有 `k8s-dev`)

### 12. 2026-08 cart 标准再同步

- [x] 配置加载启用未知键拒绝与 protovalidate，热更新失败时保留旧配置
- [x] Config Center SDK 迁移到 `control-tower`，并同步日志级别热更新、OTel 与 Consul 深度健康检查
- [x] 数据库示例改为 goose `migrations/` + 幂等 `seeds/`，业务内容保持为中性商品示例
- [x] 数据库 CRUD 示例的金额改用整数分与精确 NUMERIC 转换，避免生产 cart 代码直接复制领域实现

### 13. 生成期检索 adapter 二选一

- [x] 抽出项目自有的 `SearchCatalog` interface 与 `CatalogProduct`，业务仓储不再依赖厂商 SDK 类型
- [x] 保留 Elasticsearch adapter，并新增 Meilisearch adapter 与本地 compose
- [x] `Search.Catalog` 改为 vendor-neutral 配置；ES 生成代码不包含 Meilisearch message 或类型
- [x] `.co/manifest.yaml` 把两个 adapter 放进同一个互斥组；共享接口文件由两项共同持有
- [x] 启动探测与 `/healthz` 改用中性的 `CheckSearch`，不暗示生成物支持运行时切换

### 14. go-connect-kit 根因闭环

- [x] 删除模板内 `config`、`log`、`otel`、`registry` 的实现，只保留 protobuf-to-options adapter
- [x] 删除模板内 `dbutil`、`env`、`meta` 及通用 health-check helper 副本，消费方直接导入 kit
- [x] Config Center 的 raw SDK adapter 归 `control-tower/sdk/configsource`，kit 不反向依赖 control-tower
- [x] CLI 生成矩阵断言 kit 依赖与裁剪结果；发布前本地编译只在测试临时加入 replace
- [x] 已发布 kit `v0.3.0` 和 control-tower `v0.1.4`；`GOWORK=off` 独立 tidy/build/test 通过并刷新 `go.sum`

这条链路不使用 BSR。BSR 分发 proto，不分发 Go 实现。
