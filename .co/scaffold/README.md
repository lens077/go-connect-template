# .co/scaffold

`co new` / `co resource add` 用的 `text/template` 模板。

这些文件不参与模板仓库自身的编译（`.co/` 以点开头，Go 工具链整个忽略），
所以它们的正确性只能靠 `co-cli` 的生成矩阵测试保证：生成到临时目录后跑
`go build ./...`。改动这里的任何一个 `.tmpl`，都要跑那组测试。

## 目录

```
resource/     一个资源（proto + biz + data + service + SQL）的全套模板
monorepo/     monorepo 布局专用的覆盖文件
```

## 模板变量

| 变量 | 示例 | 说明 |
|---|---|---|
| `.Module` | `github.com/acme/shop` | 目标 module 路径 |
| `.Name` | `cart` | 资源名，小写下划线，单数 |
| `.Pascal` | `Cart` | 大驼峰单数 |
| `.PascalPlural` | `Carts` | 大驼峰复数，用于 List 方法名 |
| `.Camel` | `cart` | 小驼峰，用于局部变量 |
| `.Table` | `carts` | 数据表名，小写复数 |
| `.ProtoPackage` | `cart.v1` | proto package |
| `.APIDir` | `api/cart/v1` | proto 与生成物所在目录 |
| `.GoPkgAlias` | `cartv1` | 生成的 Go 包名 |
| `.ConnectPkg` | `cartv1connect` | connect 生成的 handler 包名 |
| `.ServiceName` | `cart-service` | 服务注册名（`SERVICE_NAME`） |
| `.Features` | `map[string]bool` | 已启用的 feature，如 `.Features.minio` |

仅 `monorepo/` 下的模板会用到这几个：

| 变量 | 示例 | 说明 |
|---|---|---|
| `.ServiceModule` | `github.com/acme/shop/backend/services/cart` | 服务包的导入前缀。monorepo 下服务没有自己的 `go.mod`，它是 `.Module` + `/services/` + `.Name` |
| `.DockerRegistry` | `ccr.ccs.tencentyun.com` | 镜像仓库地址 |
| `.DockerNamespace` | `sumery` | 镜像命名空间 |
| `.ConsulAddr` | `consul.app.com` | 服务注册用的 Consul 地址（集群内域名） |
| `.ConsulKVPrefix` | `ecommerce` | Consul KV 里配置的路径前缀 |

后四项是部署环境相关的，`co` 只填一个默认值，生成的 `Makefile` 里都写成 `?=`，
可用环境变量覆盖，不必改文件。

## 生成顺序

模板之间有编译期依赖，必须按这个顺序落盘并执行：

1. `proto.tmpl` → `{{.APIDir}}/{{.Name}}.proto`
2. `buf generate` — 产出 `*.pb.go` 与 `{{.ConnectPkg}}/`，service 层依赖它
3. `schema.sql.tmpl` / `queries.sql.tmpl` → `internal/data/{schema,queries}/`
4. `sqlc generate` — 产出 `internal/data/models/`，data 层依赖它
5. `biz.go.tmpl` / `data.go.tmpl` / `service.go.tmpl`
6. 按 `manifest.yaml` 的 `anchors` 把 provider 插进各 `Module`
7. `gofmt -w .` && `go mod tidy`

跳过第 2 或第 4 步，生成的代码引用不到 `v1.Create*Request` / `models.*Params`，
`go build` 会直接失败。`co doctor` 就是用来提前发现 buf/sqlc 缺失的。
