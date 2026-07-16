# mindbridge Go 后端 Dockerfile
# 多阶段:builder 编译二进制,最终 alpine 运行。
# 镜像只跑 API:前端静态由 nginx 容器托管并反代 /api 到本服务。
# (本地 make run 时,Go 仍可自行托管 frontend/dist;两条路径不冲突。)

# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS builder

# 国内网络优先走 goproxy.cn,加速模块拉取。
ENV GOPROXY=https://goproxy.cn,direct \
    CGO_ENABLED=0

WORKDIR /src

# 先拷依赖清单,利用层缓存:仅 go.mod/go.sum 变化时才重拉依赖。
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# 拷源码并编译。-trimpath 去绝对路径,-ldflags 去掉调试信息缩体积。
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/mindbridge ./cmd/mindbridge

# ---------- 运行阶段 ----------
FROM alpine:3.20

# ca-certificates:https 调用模型/MCP 需要;tini:正确转发信号(优雅停机)。
RUN apk add --no-cache ca-certificates tini

WORKDIR /app
COPY --from=builder /out/mindbridge /app/mindbridge

# 配置默认指向 /app/configs/config.yaml,由 compose 挂载或构建时放入。
# 迁移在程序启动时自动执行,无需额外步骤。
EXPOSE 8080

# tini 作 PID 1,确保 h.Spin() 能收到 SIGTERM 优雅停机。
ENTRYPOINT ["/sbin/tini", "--"]
CMD ["/app/mindbridge", "-config", "/app/configs/config.yaml"]
