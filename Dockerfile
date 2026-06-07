# Dockerfile — 多阶段构建，最终镜像约 15 MB
ARG GO_VERSION=1.21
ARG VERSION=dev

# ── 阶段 1：构建 ───────────────────────────────────────────────────────────
FROM golang:${GO_VERSION}-alpine AS builder

WORKDIR /src

# 先复制 go.mod/go.sum，利用 Docker 层缓存加速依赖下载
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.serviceVersion=${VERSION}" \
    -o /bin/edge-gateway \
    ./cmd/

# ── 阶段 2：最小运行镜像 ───────────────────────────────────────────────────
FROM scratch

# 时区数据
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
# CA 证书（HTTPS 请求需要）
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# 服务二进制
COPY --from=builder /bin/edge-gateway /edge-gateway

# 配置和资源文件
COPY res/ /res/

EXPOSE 59999

ENTRYPOINT ["/edge-gateway", "--configDir=/res", "--configFile=configuration.yaml"]
