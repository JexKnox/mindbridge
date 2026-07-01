# mindbridge 依赖服务

本目录提供 RAG 框架依赖的两个服务:**PostgreSQL**(知识库元信息)与 **Qdrant**(向量数据库)。
应用本体(mindbridge)不进 compose,由开发者本地 `make run` 启动,通过映射到宿主机的端口连接这两个服务。

## 前置

- 已安装 Docker 与 Docker Compose(`docker compose` 命令可用)

## 启动

```bash
# 1. 准备环境变量(默认值已与 config.example.yaml 对齐,通常无需修改)
cp deploy/.env.example deploy/.env

# 2. 启动依赖服务
docker compose -f deploy/docker-compose.yml up -d

# 3. 准备应用配置
cp configs/config.example.yaml configs/config.yaml
#    按需填写 CHAT_BASE_URL / CHAT_API_KEY / CHAT_MODEL 等 LLM 配置

# 4. 启动应用
make run
```

## 常用命令

```bash
# 查看状态(应看到两个服务均为 healthy)
docker compose -f deploy/docker-compose.yml ps

# 查看日志
docker compose -f deploy/docker-compose.yml logs -f
# 单个服务
docker compose -f deploy/docker-compose.yml logs -f postgres
docker compose -f deploy/docker-compose.yml logs -f qdrant

# 停止服务(保留数据)
docker compose -f deploy/docker-compose.yml down

# 停止并清除所有数据(慎用,向量库与元信息将丢失)
docker compose -f deploy/docker-compose.yml down -v
```

## 与 `configs/config.yaml` 的对应关系

`deploy/.env.example` 中的默认值与 `configs/config.example.yaml` 完全一致,二者各管一头:

| 依赖服务 | docker-compose / .env 负责 | 应用 config.yaml 读取 |
|---------|---------------------------|----------------------|
| PostgreSQL | 创建 `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB`,映射端口 `5432:5432` | `postgres.dsn` = `postgres://mindbridge:mindbridge@localhost:5432/mindbridge?sslmode=disable` |
| Qdrant | 映射端口 `6333:6333`,本地不开认证 | `qdrant.host` = `localhost`,`qdrant.port` = `6333`,不带 `api_key` |

因此:

- 若保持默认值,**只需** `cp configs/config.example.yaml configs/config.yaml` 即可,应用开箱即连。
- 若修改了 `.env` 中的凭证或端口,**必须同步**修改 `configs/config.yaml` 中对应的 `postgres.dsn` 与 `qdrant.host`/`qdrant.port`。

## 版本与持久化

| 服务 | 镜像(固定版本) | 数据 volume |
|------|----------------|-------------|
| PostgreSQL | `postgres:16-alpine` | `mindbridge-pgdata` → `/var/lib/postgresql/data` |
| Qdrant | `qdrant/qdrant:v1.13.2` | `mindbridge-qdrant-data` → `/qdrant/storage` |

两个服务挂载在命名 volume 上,`docker compose down` 不会丢数据,只有 `down -v` 才会清除。

## 网络

两个服务接入独立的 bridge 网络 `mindbridge-net`,相互隔离;对外仅通过宿主机端口 `5432` / `6333` 暴露,应用从宿主机连接。
