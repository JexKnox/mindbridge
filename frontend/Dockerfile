# mindbridge 前端 Dockerfile
# 多阶段:node 构建产物 -> nginx 托管 + 反代 /api 到 Go 后端。

# syntax=docker/dockerfile:1

FROM node:20-alpine AS builder

WORKDIR /src

# 利用层缓存:仅 lockfile 变化时重装依赖。项目有 pnpm-lock.yaml,用 frozen-lockfile 保证可复现。
COPY package.json pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile

COPY . .
RUN pnpm build

# ---------- 运行阶段 ----------
FROM nginx:1.27-alpine

# 拷构建产物 + nginx 配置。
COPY --from=builder /src/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80
