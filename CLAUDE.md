# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

`mindbridge` —— 基于 Go + eino 的通用 RAG 框架。核心能力:前端上传数据 → 解析/切分/向量化 → 存入向量数据库;对话时选择对应的"知识库"进行检索增强生成。设计目标是"通用",即支持多个知识库的隔离管理与按库检索。

## 技术栈(已定,后续开发遵循)

- **后端**:Go + [eino](https://github.com/cloudwego/eino)(CloudWeGo 的 Go LLM 应用框架)+ eino-ext(扩展组件)
- **向量数据库**:Qdrant(生产中小规模,单容器部署,Go SDK,eino-ext 原生支持)
- **前端**:React + Vite + TypeScript(UI 库待定:shadcn/ui 或 Ant Design)
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

> 仓库当前只有 README/LICENSE,尚无 Go module 与前端工程。以下为代码落地后应采用的命令约定,实际命令在工程初始化后据实补充。

- 后端:`go run ./cmd/mindbridge` 启动;`go test ./...` 跑全部测试;`go test ./internal/xxx -run TestXxx` 跑单个测试
- 前端:`pnpm dev` 开发;`pnpm build` 构建;`pnpm test` 测试
- 依赖服务:Qdrant(`docker run -p 6333:6333 qdrant/qdrant`)、关系型 DB、可选 Ollama

## 约定

- 接口层与业务层分离:HTTP/gRPC handler 只做参数解析与响应组装,逻辑沉到 `internal/` 下分层包。
- 知识库是头等概念:任何检索/写入都必须绑定 `kb_id`,杜绝跨库串数据。
- 切换向量库/模型 Provider 时,改动应集中在适配层,不动业务逻辑——这是"通用"目标的硬约束。
- 文档处理(切分策略、chunk 大小)、embedding 维度等需与所选 embedding 模型匹配,且一旦入库不可随意更改(维度变更要重建 collection)。
