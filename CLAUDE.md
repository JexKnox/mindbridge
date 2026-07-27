# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

`mindbridge` —— 基于 Go + eino 的通用 RAG 框架。核心能力:前端上传数据 → 解析/切分/向量化 → 存入向量数据库;对话时选择对应的"知识库"进行检索增强生成。设计目标是"通用",即支持多个知识库的隔离管理与按库检索。

## 技术栈(已定,后续开发遵循)

- **后端**:Go + [eino](https://github.com/cloudwego/eino)(CloudWeGo 的 Go LLM 应用框架)+ eino-ext(扩展组件)+ [Hertz](https://github.com/cloudwego/hertz)(CloudWeGo HTTP 框架,与 eino 同生态)
- **向量数据库**:Qdrant(生产中小规模,单容器部署,Go SDK,eino-ext 原生支持)
- **日志**:[zap](https://github.com/uber-go/zap)(结构化日志,`internal/logging` 统一构建 + chi 请求日志中间件)
- **前端**:React + Vite + TypeScript + Ant Design(UI 库已定:Ant Design)
- **模型接入**:统一通过 eino 的 `ChatModel` / `EmbeddingModel` 接口,支持三类 Provider:
  - OpenAI 兼容 API(火山引擎 Ark、DeepSeek、通义、智谱等大多兼容)
  - 国内模型原生 API
  - 本地模型(Ollama / vLLM)
  - 通过配置切换,业务代码不感知具体 Provider。

## 架构骨架(后续实现以此为准)

### 数据写入路径(离线索引)
```
上传接口 → Document Loader(eino) → Splitter(切分) → EmbeddingModel → Indexer(Qdrant)
```
- 每个"知识库"对应 Qdrant 中的一个 collection(命名如 `kb_{kb_id}`),实现库间隔离。
- 知识库元信息(名称、collection 名、embedding 维度、关联配置)存于关系型 DB(待选型,建议 Postgres)。

### 对话检索路径(在线查询)
```
Chat 接口(带 kb_id) → Retriever(Qdrant,按 kb_id 取 collection) → [可选 Reranker] → ChatModel → 流式响应
```
- 检索范围由请求中的 `kb_id` 决定,据此路由到对应 collection。
- 响应走 SSE/流式,前端逐字渲染。

### 模型抽象
- 业务层只依赖 eino 的 `ChatModel` / `EmbeddingModel` / `Indexer` / `Retriever` 接口,不直接依赖任何具体 Provider/向量库实现。
- 新增 Provider = 新增一个 eino-ext 适配 + 配置项,不改业务代码。

## 命令

> 工程骨架已落地(Go module + 目录分层 + 最小 RAG 闭环)。命令约定如下。

- 后端:`make run`(或 `go run ./cmd/mindbridge`)启动;`go test ./...` 跑全部测试;`go test ./internal/xxx -run TestXxx` 跑单个测试;`make vet` 静态检查
- 依赖:`make tidy` 整理依赖(Makefile 内置 `GOPROXY=https://goproxy.cn,direct`)
- 依赖服务:Qdrant(`docker run -p 6333:6333 qdrant/qdrant`)、Postgres;迁移在程序启动时自动执行
- 前端:`make web-dev`(或 `cd frontend && pnpm dev`)开发,跑在 :5173,/api 反代到 :8080;`make web-build`(或 `pnpm build`)构建到 `frontend/dist`,由 Go 启动时自动托管(同源,无跨域)

## 约定

- 接口层与业务层分离:HTTP/gRPC handler 只做参数解析与响应组装,逻辑沉到 `internal/` 下分层包。
- 知识库是头等概念:任何检索/写入都必须绑定 `kb_id`,杜绝跨库串数据。
- 切换向量库/模型 Provider 时,改动应集中在适配层,不动业务逻辑——这是"通用"目标的硬约束。
- 文档处理(切分策略、chunk 大小)、embedding 维度等需与所选 embedding 模型匹配,且一旦入库不可随意更改(维度变更要重建 collection)。

## 目录结构

```
cmd/mindbridge/       入口:装配 + 启动 HTTP
internal/
  config/             YAML 配置 + ${VAR} 环境变量替换
  logging/            zap 日志器构建 + chi 请求日志中间件
  cache/              进程内缓存:泛型 LRU(容量+TTL)+ KB 版本号(检索缓存失效)
  metrics/            Prometheus 指标(系统/检索/生成三层)+ /metrics 独立端口
  repo/               Postgres 连接池 + 迁移(embed.FS)
  adapter/            ★适配层:chat/embedding/indexer/retriever 工厂 + Qdrant REST 客户端
  kb/                 知识库领域:元信息 + 建库编排(探测维度 → 建 collection → 写元信息)
  doc/                离线索引:Loader → Splitter → Indexer
  rag/                在线查询:Retriever → Chat(流式) + 引用 + 拒答
  server/             Hertz HTTP handler(薄层)+ 前端静态托管(SPA 回退)
frontend/            React + Vite + TS + Ant Design 前端(dist/ 由 Go 托管)
deploy/              docker-compose:PostgreSQL + Qdrant 依赖服务
configs/              配置示例
```

## RAG 最佳实践(开发准则)

以下为已采纳或待落地的 RAG 实践,后续开发据"骨架已落地 / TODO"标注推进。

1. **切分策略是 RAG 质量的第一决定因素**
   - 按 token(非字符)切,`chunk_size=400 / overlap=80` 可配;分隔符优先级 `\n\n`→`\n`→` `。骨架已落地:`internal/doc/splitter.go`。
 - 每个 chunk 的 `Meta` 必须带 `source_file` / `chunk_index`,用于引用追溯与按文件删除。
 - 文档解析支持 `.md`(goldmark 抽纯文本)、`.docx`(纯 Go 解析 zip+XML)、`.txt`(原样);见 `internal/doc/loader.go`。其它扩展名按纯文本兜底。
 - 结构化切分已落地(markdown):按标题分节、chunk 前缀标题路径、代码块/表格整块保留;见 `internal/doc/markdown.go`。TODO:PDF/HTML 结构化切分。

2. **混合检索 + Reranker**
 - 已落地:向量召回(语义)+ 关键词召回(Qdrant sparse 向量,IDF modifier 近似 BM25),Go 侧 RRF 融合;分词为中英混合(CJK bigram + 英文词),见 `internal/adapter/sparse.go` / `retriever.go`。旧 collection 无稀疏向量时自动降级纯向量,上传文档时自动补建稀疏配置(存量点需重传才可被关键词召回)。
 - 已落地:cross-encoder reranker,通用 HTTP `/rerank` API(Jina / Cohere / SiliconFlow / TEI 兼容),配置 `models.reranker`(model 留空禁用);见 `internal/adapter/reranker.go`。重排分写入 `rerank_score`,拒答阈值仍用余弦分。

3. **embedding 维度与模型绑死到 KB**
 - 建库时探测维度写入 KB 元信息 + Qdrant collection;同一 KB 强制同一 embedding 模型。骨架已落地:`internal/kb/manager.go` + `internal/adapter/embedding.go`。
 - 已落地:每个 chunk 的 payload 存 `embedding_model`(pipeline 写入),便于审计与迁移。

4. **查询侧做 query 改写**
 - 已落地:LLM 多路改写(配置 `models.rewrite` 小模型 + `rag.rewrite_n`),原 query + 改写 query 各自召回,跨 query 去重后 RRF 合并;改写失败静默降级仅原 query。见 `internal/rag/query.go` / `service.go`。TODO:HyDE。

5. **生成阶段:流式 + 引用 + 拒答**
   - SSE 流式逐 token;prompt 注入带 `[1][2]` 编号的召回 chunk 要求标注;top1 score < 阈值则提示拒答;SSE 末尾发 `event:citations`。骨架已落地:`internal/rag/service.go` + `internal/server/chat_handler.go`。

6. **评估闭环**
 - 每个知识库建 20–50 条标准 Q&A eval set;换模型/调参前后跑一遍用数字说话。
 - 检索指标已落地:`go run ./cmd/mindbridge-eval -kb <id> -eval evalset.jsonl -k 5` 输出 recall@k / MRR / hit@k,评估集为 JSONL(`{"question":..., "relevant_files":[...]}`)。
 - TODO:生成指标(faithfulness、answer relevance,RAGAS 思路)。

7. **工程坑**
 - Qdrant 按 `kb_{id}` 分 collection(而非单 collection + payload 过滤),删库干净、查询快。已落地。
 - collection 创建时为 `source_file`、`kb_id` 建 payload keyword 索引。已落地:`internal/adapter/qdrant.go`。
 - 重新上传同名文件:先按 `source_file` 删旧 chunk 再写新,避免幽灵数据。已落地:`internal/server/doc_handler.go`。
 - 批量 embedding / Qdrant upsert 分批 + 指数退避重试(`ingest.embed_batch` / `ingest.upsert_batch`)。已落地:`internal/adapter/indexer.go`。

8. **缓存(已落地,`internal/cache`)**
 - 进程内缓存,不引入 Redis(单实例部署够用;多副本时再换实现)。main 构造一份 `cache.RAGCaches`,注入 server 与 agent。三层:
   - **查询 embedding 缓存**:key = `emb:{模型名}:{sha256(文本)}`,2048 条 × 1h。只包在查询侧(`adapter.NewCachedEmbedder` 装饰 retriever 的 embedder),文档摄取侧不缓存(内容不重复,徒耗内存)。
   - **改写结果缓存**:key = `rw:{n}:{sha256(query)}`,512 条 × 1h;改写失败的降级结果不缓存。
   - **检索+重排结果缓存**:缓存「改写→召回→重排」后的最终文档列表 + 重排前最高余弦分(拒答判断用),256 条 × 15min。命中时整段跳过。key 前缀 = `ret:{collection}:v{kb_version}:{检索配置}`(见 `internal/server/chat_handler.go`)。
 - **失效策略**:key 携带 KB 版本号(`cache.Versions`,进程内计数器),上传/删除文档时 `Bump`,旧版本条目查不到、随 LRU 自然淘汰,无需主动清理;调参后 key 前缀变化同理。版本号重启归零,与进程内缓存生命周期一致。
 - **不做最终答案缓存**:回答是流式 + 引用,语义缓存命中错答案的风险大于收益。
 - Rerank 不单独设缓存层:检索结果缓存存的就是重排后结果,一层顶两层。

9. **监控(已落地,`internal/metrics`,Prometheus)**
 - `/metrics` 走独立端口(默认 `:9091`,配置 `metrics.addr`,置空关闭):与业务端口分离,SSE 长连接不影响抓取,内部指标不暴露到业务入口。指标统一 `mindbridge_` 前缀;Go 运行时指标(内存/GC/goroutine)由默认收集器自带。
 - **系统层**:`http_requests_total`(method/path/status,path 用注册路由防高基数)、`http_request_duration_seconds`(P50/P95/P99 用直方图分位)、`qdrant_request_duration_seconds` + `qdrant_request_errors_total`(op=search/query_sparse/upsert/delete)、`model_request_duration_seconds` + `model_request_errors_total`(kind=embedding/rerank/rewrite,错误率上升 = API 可用性下降)。
 - **检索层**:`retrieval_candidates`(channel=dense/sparse/fused,某一路持续为 0 说明该通道故障)、`retrieval_empty_total` / `retrieval_requests_total`(空结果率突升先查索引/过滤,不一定是模型问题)、`retrieval_top1_score`(分布左移 = 检索质量劣化)、`rerank_top1_changed_total` / `rerank_requests_total`(占比过高 = 召回排序差,持续为 0 = 重排冗余)、`cache_hits/misses_total`(cache=embedding/rewrite/retrieval)。
 - **生成层**:`chat_first_token_seconds`(TTFT)、`chat_generation_seconds`、`chat_output_chars`(token 近似,流式响应不带 usage)、`chat_refusals_total` / `chat_answers_total`(拒答率)、`chat_citations`、`chat_stream_errors_total`。
 - 埋点位置:HTTP 中间件在 `internal/server/server.go`;Qdrant 在 `internal/adapter/qdrant.go` 各操作;模型调用在 embedder 装饰器 / reranker / rewriter;检索与生成在 `internal/adapter/retriever.go` + `internal/rag/service.go`。
 - TODO:线上抽样 Recall/MRR/Faithfulness(离线评估已有 `cmd/mindbridge-eval`)、Grafana dashboard、告警规则。

