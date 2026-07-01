# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

`mindbridge` —— 基于 Go + eino 的通用 RAG 框架。核心能力:前端上传数据 → 解析/切分/向量化 → 存入向量数据库;对话时选择对应的"知识库"进行检索增强生成。设计目标是"通用",即支持多个知识库的隔离管理与按库检索。

## 技术栈(已定,后续开发遵循)

- **后端**:Go + [eino](https://github.com/cloudwego/eino)(CloudWeGo 的 Go LLM 应用框架)+ eino-ext(扩展组件)
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
  repo/               Postgres 连接池 + 迁移(embed.FS)
  adapter/            ★适配层:chat/embedding/indexer/retriever 工厂 + Qdrant REST 客户端
  kb/                 知识库领域:元信息 + 建库编排(探测维度 → 建 collection → 写元信息)
  doc/                离线索引:Loader → Splitter → Indexer
  rag/                在线查询:Retriever → Chat(流式) + 引用 + 拒答
  server/             chi HTTP handler(薄层)+ 前端静态托管(SPA 回退)
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
   - TODO:结构化切分(PDF/HTML 按 header/段落,代码/表格整体保留)。

2. **混合检索 + Reranker**
   - 向量召回(语义)+ 关键词召回(BM25),分数用 RRF 融合;召回 topK 后用 cross-encoder rerank 到 rerankTopK。骨架已落地:纯向量召回 + `rerankTopK` 截断占位。
   - TODO:BM25 混合检索、reranker 接入(bge-reranker / Cohere / Qwen3-reranker,eino 有 reranker 接口)。接口位在 `internal/rag/service.go`。

3. **embedding 维度与模型绑死到 KB**
   - 建库时探测维度写入 KB 元信息 + Qdrant collection;同一 KB 强制同一 embedding 模型。骨架已落地:`internal/kb/manager.go` + `internal/adapter/embedding.go`。
   - TODO:在 `payload` 存 `embedding_model`/`embedding_version` 便于审计与迁移。

4. **查询侧做 query 改写**
   - 不直接拿用户原话检索;HyDE / 多路改写并行召回后去重合并。骨架已落地:直通占位 `internal/rag/query.go`。TODO:接入 LLM 改写(用便宜小模型)。

5. **生成阶段:流式 + 引用 + 拒答**
   - SSE 流式逐 token;prompt 注入带 `[1][2]` 编号的召回 chunk 要求标注;top1 score < 阈值则提示拒答;SSE 末尾发 `event:citations`。骨架已落地:`internal/rag/service.go` + `internal/server/chat_handler.go`。

6. **评估闭环**
   - 每个知识库建 20–50 条标准 Q&A eval set;检索指标(recall@k、MRR)+ 生成指标(faithfulness、answer relevance,RAGAS 思路);换模型/调参前后跑一遍用数字说话。TODO:未实现。

7. **工程坑**
   - Qdrant 按 `kb_{id}` 分 collection(而非单 collection + payload 过滤),删库干净、查询快。已落地。
   - collection 创建时为 `source_file`、`kb_id` 建 payload keyword 索引。已落地:`internal/adapter/qdrant.go`。
   - 重新上传同名文件:先按 `source_file` 删旧 chunk 再写新,避免幽灵数据。已落地:`internal/server/doc_handler.go`。
   - 批量 embedding 注意 Qdrant upsert 限流,分批 + 重试。TODO。

