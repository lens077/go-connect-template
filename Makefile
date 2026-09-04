# 默认值
VERSION ?= dev
GOIMAGE ?= golang:1.26.5-alpine3.22
GOOS ?= linux
GOARCH ?= arm64
CGOENABLED ?= 0

# 动态变量
SERVICE = $(shell basename $$PWD)
DOCKER_IMAGE=example/$(SERVICE):$(VERSION)
REPOSITORY = example/$(SERVICE)
REGISTER = docker.io
ARM64=linux/arm64
AMD64=linux/amd64
# 服务注册用的 Consul 地址(集群内域名)。配置不再走 Consul KV。
CONSUL_ADDR=consul.example.com

.PHONY: k8s-dev
k8s-dev:
	kubectl apply -f deploy

# dev 默认走本地文件配置:克隆下来不装配置中心也能直接起服务。
# 需要连 Config Center 时用 make dev-cc。
.PHONY: dev
dev: dev-file

# 从 configs/dev.yml 读整份配置,不走配置中心。
#
# 「不走配置中心」不等于「不接外部组件」:postgres 与 redis 是启动硬依赖,
# 连不上服务直接退出。先起本地依赖:
#   docker compose -f infrastructure/postgres/compose.yaml up -d
#   docker compose -f infrastructure/redis/compose.yaml up -d
# 检索后端可选(连不上只在 /healthz 里显示不健康),要用检索再起所选 adapter:
#   docker compose -f infrastructure/elasticsearch/compose.yaml up -d # +co:elasticsearch
#   docker compose -f infrastructure/meilisearch/compose.yaml up -d # +co:meilisearch
# 凭据已经和 configs/dev.yml 对好,不用改任何配置;建表 DDL 由 postgres 那份
# compose 在库初始化时自动跑,也不用手动灌。
.PHONY: dev-file
dev-file:
	SERVICE_NAME=org-service-v1 \
	CONSUL_ENABLED=false \
	CONFIG_SOURCE=file \
	CONFIG_FILE=configs/dev.yml \
	go run cmd/server/main.go

# +co:begin config-configcenter
# 经 CONFIG_SOURCE_FILE 的 selector 从 Config Center 拉 Bootstrap。
# 首次使用先复制 example,只在被忽略的 source.dev.yaml 里填机器 token:
#   cp configs/source.dev.yaml.example configs/source.dev.yaml
.PHONY: dev-cc
dev-cc:
	SERVICE_NAME=org-service-v1 \
	CONSUL_ENABLED=true \
	CONFIG_SOURCE_FILE=configs/source.dev.yaml \
	go run cmd/server/main.go
# +co:end

.PHONY: test
test:
	go test -short -coverprofile=coverage.out ./...

.PHONY: sqlc
sqlc:
	sqlc generate

.PHONY: api
api:
	# 切换到backend目录运行buf命令，确保proto文件路径在context directory内
	buf generate --template buf.gen.yaml --path api
	buf generate --template buf.gen.ts.yaml --path api

.PHONY: generate
generate:
	# 切换到backend目录运行buf命令，确保proto文件路径在context directory内
	buf generate --template buf.gen.yaml --path api
	buf generate --template buf.gen.ts.yaml --path api

.PHONY: conf
conf:
	# 切换到backend目录运行buf命令，确保proto文件路径在context directory内
	buf generate --template buf.gen.yaml --path internal/conf

.PHONY: docker-build
# 使用 docker 构建镜像
docker-build:
	@echo "构建的微服务: $(SERVICE)"
	@echo "系统: $(GOOS) | CPU架构: $(GOARCH)"
	@echo "镜像名: $(REPOSITORY):$(VERSION)"
	cd ../.. && docker build . \
      -f ./services/$(SERVICE)/Dockerfile \
      --progress=plain \
      -t ecommerce/$(SERVICE):dev \
      --build-arg SERVICE=$(SERVICE) \
      --build-arg CGOENABLED=0 \
      --build-arg GOIMAGE=golang:1.25.8-alpine3.22 \
      --build-arg GOOS=linux \
      --build-arg GOARCH=amd64 \
      --build-arg VERSION=dev \
      --platform linux/amd64

# 使用方式: make docker-push SERVICE=微服务名
.PHONY: docker-push
docker-push:
	@echo "使用方式: make docker-push SERVICE=微服务名"
	@echo "OS: $(GOOS) | ARCH: $(GOARCH)"
	@echo "Docker image: $(REPOSITORY):$(VERSION)"
	docker tag ecommerce/$(SERVICE):$(VERSION) $(REGISTER)/$(REPOSITORY):$(VERSION)
	docker push $(REGISTER)/$(REPOSITORY):$(VERSION)

.PHONY: docker-deploy
docker-deploy:
	@echo "使用方式: make docker-deploy SERVICE=微服务名"
	@echo "SERVICE=$(SERVICE)"
	make docker-build SERVICE=$(SERVICE)
	@echo "SERVICE=$(SERVICE)"
	make docker-push SERVICE=$(SERVICE)

.PHONY: docker-deployx
# 使用 docker 构建多平台架构镜像
docker-deployx:
	@echo "构建的微服务: $(SERVICE)"
	@echo "平台1: $(ARM64)"
	@echo "平台2: $(AMD64)"
	@echo "镜像名: $(REPOSITORY):$(VERSION)"
	cd ../.. && docker buildx build . \
	  -f ./services/$(SERVICE)/Dockerfile \
	  --progress=plain \
	  -t $(REGISTER)/$(REPOSITORY):$(VERSION) \
	  --build-arg SERVICE=$(SERVICE) \
	  --build-arg CGOENABLED=$(CGOENABLED) \
	  --build-arg GOIMAGE=$(GOIMAGE) \
	  --build-arg VERSION=$(VERSION) \
	  --platform $(ARM64),$(AMD64) \
	  --push \
	  --cache-from type=registry,ref=$(REGISTER)/$(REPOSITORY):cache \
	  --cache-to type=registry,ref=$(REGISTER)/$(REPOSITORY):cache,mode=max

.PHONY: docker-run
docker-run:
	docker compose up -d
