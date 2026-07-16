# mindbridge 容器编排

本目录提供两套用法,共用同一份 `docker-compose.yml`:

1. **全栈一键起**(推荐体验):PostgreSQL + Qdrant + Go 后端(app)+ nginx 前端代理,一条命令拉起整套。
2. **仅依赖服务**:只起 PostgreSQL + Qdrant,应用本体由开发者本地 `make run` 启动。

## 前置

- 已安装 Docker 与 Docker Compose(`docker compose` 命令可用)

## 全栈启动(含应用 + nginx)

```bash
# 1. 准备环境变量:填写 LLM 模型密钥
cp deploy/.env.example deploy/.env
#    编辑 deploy/.env,填入 CHAT_BASE_URL / CHAT_API_KEY / CHAT_MODEL 与 embedding 同款

# 2. 一键构建并启动(首次会构建 app / nginx 镜像)
docker compose -f deploy/docker-compose.yml up -d --build

# 3. 访问
open http://localhost:8080      # nginx 托管前端,/api 反代到 app
```

服务拓扑:

```
浏览器 -> :8080 -> nginx(前端静态 + 反代 /api) -> app(Go API :8080)
                                                   ├── postgres(元信息)
                                                   └── qdrant(向量库)
```

- nginx 容器 80 -> 宿主机 8080,托管前端 SPA,`/api/*` 反代到 `app:8080`。
- app 容器只在内网 `mindbridge-net` 暴露 8080,不映射宿主机(避免与 nginx 抢端口)。
- app 读 `configs/config.docker.yaml`(挂载为容器内 `/app/configs/config.yaml`),其中 postgres/qdrant 主机用 compose 服务名,密钥由 `.env` 经 `${VAR}` 注入。

## 仅依赖服务(配合本地 make run)

```bash
cp deploy/.env.example deploy/.env
docker compose -f deploy/docker-compose.yml up -d postgres qdrant

cp configs/config.example.yaml configs/config.yaml   # 按需填 LLM 配置
make run                                              # 应用从宿主机 localhost 连 5432/6333
```

## 常用命令

```bash
# 查看状态(全栈时应见 4 个服务 healthy)
docker compose -f deploy/docker-compose.yml ps

# 查看日志
docker compose -f deploy/docker-compose.yml logs -f app     # 后端
docker compose -f deploy/docker-compose.yml logs -f nginx   # 前端/反代

# 停止(保留数据)
docker compose -f deploy/docker-compose.yml down

# 停止并清除所有数据(慎用,向量库与元信息将丢失)
docker compose -f deploy/docker-compose.yml down -v

# 仅重建应用(改了 Go 代码后)
docker compose -f deploy/docker-compose.yml up -d --build app
```

## 配置对应关系

| 项目 | 全栈(容器内) | 仅依赖(本地 make run) |
|------|--------------|----------------------|
| 应用配置 | `configs/config.docker.yaml`(挂载) | `configs/config.yaml`(本地复制 example) |
| postgres 主机 | `postgres`(服务名) | `localhost` |
| qdrant 主机 | `qdrant`(服务名) | `localhost` |
| 前端 | nginx 容器托管 | Go 内置 `mountSPA` 托管 `frontend/dist` |
| 模型密钥 | `deploy/.env` -> `${VAR}` 注入 | 直接写进 `configs/config.yaml` |

## 版本与持久化

| 服务 | 镜像 | 数据 volume |
|------|------|-------------|
| PostgreSQL | `postgres:16-alpine` | `mindbridge-pgdata` -> `/var/lib/postgresql/data` |
| Qdrant | `qdrant/qdrant:v1.13.2` | `mindbridge-qdrant-data` -> `/qdrant/storage` |
| app | 本地构建 `mindbridge-app` | 无状态(数据都在 pg/qdrant) |
| nginx | 本地构建 `mindbridge-nginx` | 无状态 |

postgres/qdrant 挂在命名 volume 上,`down` 不丢数据,`down -v` 才清除。

## 网络

所有服务接入 bridge 网络 `mindbridge-net`,相互用服务名通信;对外仅暴露:
- `8080`(nginx,全栈模式)
- `5432` / `6333`(postgres/qdrant,供本地 `make run` 使用)
