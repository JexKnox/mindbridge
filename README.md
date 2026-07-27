# MindBridge

基于 Go + [eino](https://github.com/cloudwego/eino) 的通用 RAG（检索增强生成）框架。

上传文档 → 解析 / 切分 / 向量化 → 写入知识库；对话时按知识库检索并流式生成带引用的回答。设计目标是「通用」：多知识库隔离、模型与向量库可替换，业务代码不感知具体 Provider。

## 功能

- **多知识库**：每个 KB 对应独立 Qdrant collection（`kb_{id}`），检索与写入严格绑定 `kb_id`
- **混合检索**：向量召回 + 关键词（BM25 风格稀疏向量），RRF 融合；可选 cross-encoder 重排与 LLM 多路 query 改写
- **流式问答**：SSE 逐 token 输出，带 `[1][2]` 引用与低分拒答
- **Agent**：ReAct + 内置 skill + MCP 工具调用（与固定 RAG 路径并列）
- **质量闭环**：问答留痕、分类用户反馈、统计页、离线评测门槛、可选 LLM Judge 抽样（faithfulness）
- **工程配套**：进程内缓存、Prometheus 三层指标（`:9091/metrics`）、Docker 一键部署

## 技术栈

| 层 | 选型 |
|----|------|
| 后端 | Go · eino · Hertz · zap |
| 向量库 | Qdrant |
| 元数据 | PostgreSQL |
| 前端 | React · Vite · TypeScript · Ant Design |
| 模型 | OpenAI 兼容 API（火山 Ark / DeepSeek / 通义 / Ollama 等） |

## 快速开始

### 方式一：Docker 全栈（推荐）

```bash
cp deploy/.env.example deploy/.env
# 编辑 deploy/.env，填入 CHAT_* / EMB_* 模型配置

docker compose -f deploy/docker-compose.yml up -d --build
# 打开 http://localhost:8080
```

更细的编排说明见 [deploy/README.md](deploy/README.md)。

### 方式二：本地开发

**前置**：Go 1.22+、Node.js + pnpm、Docker（仅起依赖）。

```bash
# 1. 依赖服务
cp deploy/.env.example deploy/.env
docker compose -f deploy/docker-compose.yml up -d postgres qdrant

# 2. 配置
cp configs/config.example.yaml configs/config.yaml
# 编辑 configs/config.yaml，填入 chat / embedding 的 base_url、api_key、model

# 3. 后端
make run          # http://localhost:8080 ，迁移启动时自动执行

# 4. 前端（另开终端）
make web-install
make web-dev      # http://localhost:5173 ，/api 反代到 :8080
```

生产同源部署：先 `make web-build`，再 `make run`，Go 会托管 `frontend/dist`。

## 配置要点

配置文件支持 `${VAR}` 环境变量替换。复制 `configs/config.example.yaml` 后主要关注：

| 段 | 说明 |
|----|------|
| `models.chat` / `embedding` | 对话与向量模型（必填） |
| `models.reranker` | 重排；`model` 留空禁用 |
| `models.rewrite` | query 改写小模型；留空禁用 |
| `rag.hybrid` | 混合检索开关（默认开） |
| `judge.model` | 线上 LLM Judge；留空禁用 |
| `metrics.addr` | Prometheus 端口，默认 `:9091`，置空 `""` 关闭 |

## 使用流程

1. 打开前端 → **知识库** → 新建（指定 embedding 模型名）
2. 进入知识库 → 上传 `.md` / `.docx` / `.txt` 等文档
3. 在对话面板提问；回答带引用，可提交分类反馈
4. **统计** 页查看问答量、拒答率、反馈分布；可导出负反馈为评测集
5. **Agent** 页可绑定知识库并调用工具 / MCP

## API 一览

| 方法 | 路径 | 说明 |
|------|------|------|
| GET/POST | `/api/kb` | 列表 / 建库 |
| POST | `/api/kb/:id/documents` | 上传文档 |
| POST | `/api/kb/:id/chat` | RAG 对话（SSE） |
| POST | `/api/agent/chat` | Agent 对话（SSE） |
| POST | `/api/feedback` | 用户反馈 |
| GET | `/api/stats/overview` | 质量统计 |
| GET | `/api/stats/export` | 导出评测集 JSONL |
| GET | `/healthz` | 健康检查 |
| GET | `:9091/metrics` | Prometheus 指标 |

SSE 对话末尾会发 `event: citations` 与 `event: trace`（含 `trace_id`，用于反馈）。

## 离线评测

```bash
# 评估集 JSONL，每行：{"question":"...","relevant_files":["a.md"]}
go run ./cmd/mindbridge-eval \
  -config configs/config.yaml \
  -kb <kb_id> \
  -eval evalset.jsonl \
  -k 5 \
  -min-recall 0.8 -min-mrr 0.6   # 可选门槛，不达标非零退出（可接 CI）
```

## 常用命令

```bash
make run          # 启动后端
make test         # 跑测试
make vet          # 静态检查
make tidy         # 整理依赖
make web-dev      # 前端开发
make web-build    # 前端构建
```

## 架构简述

```
上传 → Loader → Splitter → Embedding → Indexer(Qdrant)
对话 → [改写] → 混合检索 → [Rerank] → Chat(流式) → 引用 / 拒答 / 留痕
```

业务层只依赖 eino 接口；Provider / 向量库实现集中在 `internal/adapter`。目录细节与 RAG 实践约定见 [CLAUDE.md](CLAUDE.md)。

## License

见仓库根目录许可证文件（若有）。
