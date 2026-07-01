# 同类项目调研:Go 后端 RAG 框架

> 调研日期:2026-07-01
> 范围:GitHub 上后端使用 Go 语言的 RAG / 知识库类项目,用于对照 mindbridge 的技术选型与 roadmap。

## 调研背景

mindbridge 的技术栈为 **Go + eino + Qdrant + Postgres**,核心目标是"通用"——支持多个知识库的隔离管理与按库检索。本文档汇总同类开源项目,重点关注:

- 是否使用 eino 框架(技术栈一致度)
- 是否支持多知识库隔离
- 是否实现了 mindbridge roadmap 中的 TODO 项(混合检索、reranker、chunk 编辑、评估闭环、MCP 等)

---

## 一、最对口(技术栈一致:eino + Go + 知识库)

### [wangle201210/go-rag](https://github.com/wangle201210/go-rag) ⭐322

**几乎就是 mindbridge 的"加强版"**。基于 eino + GoFrame(gf) + Vue 实现知识库 RAG,技术选型与 mindbridge 高度一致。

- 存储层:同时支持 ES8 与 Qdrant 存储向量
- 功能列表:
  - md / pdf / html 文档解析、网页解析
  - 文档检索、长文档自动切割(chunk)
  - **多知识库支持**
  - **chunk 编辑**
  - **自动生成 QA 对**
  - **多路召回**
  - MCP 集成(可接入 DeepChat 等)
- 提供 Docker Compose 一键启动,另有 Mac/Windows GUI 版本 [wachat](https://github.com/wangle201210/wachat)

**参考价值**:已实现 mindbridge CLAUDE.md 中多条 TODO,可直接对照其实现思路。

### [cloudwego/eino-examples](https://github.com/cloudwego/eino-examples) ⭐691

eino 官方示例集。mindbridge 使用的就是 eino 框架,这里是最权威的用法参考:检索、流式生成、reranker、agent 编排等标准写法。

### [Wood-Q/AwesomeEino](https://github.com/Wood-Q/AwesomeEino) ⭐149

eino 入门实战教程(中文)。适合补充 eino 框架本身的使用方法与最佳实践。

---

## 二、高星通用 Go RAG 平台(架构参考)

### [Tencent/WeKnora](https://github.com/Tencent/WeKnora) ⭐17.6k

腾讯开源的 LLM 知识平台:把原始文档转化为可查询的 RAG、自主推理 agent、自维护 Wiki。工业级架构,适合参考大厂如何组织多知识库 + agent 协同。

### [the-open-agent/openagent](https://github.com/the-open-agent/openagent) ⭐5.3k

Go 编写的下一代个人 AI 助手,涵盖 RAG + agent loop,支持 computer-use / browser-use / coding agent。偏 agent 方向。

### [weibaohui/openDeepWiki](https://github.com/weibaohui/openDeepWiki) ⭐94

完全 AI 驱动的 DeepWiki,使用 Go + Eino 技术栈开发。文档处理与知识沉淀思路可参考。

---

## 三、偏小但思路接近

### [liaotxcn/Weave](https://github.com/liaotxcn/Weave) ⭐361

高效、安全、稳定的应用开发平台,深度集成 AI 能力,易于扩展。

### [hexagon-codes/hexclaw](https://github.com/hexagon-codes/hexclaw) ⭐11

企业级 AI Agent 后台:多模型对话、RAG 知识库、Tool Call 代理、Skill 系统、MCP 协议、代码沙箱、飞书/钉钉/Telegram 多平台 IM 接入。功能覆盖面广。

### [int2t05/OpsMind](https://github.com/int2t05/OpsMind) ⭐7

私有部署的 AI 运维数字员工:自建 RAG 引擎(BM25 + 向量混合检索 + 重排序)、知识库管理、申告工单全流程。Go + Next.js 全栈,Docker Compose 一键部署。

**参考价值**:其混合检索 + 重排序实现与 mindbridge roadmap 中"混合检索 + Reranker"几乎完全对应。

### [wmulabs/eywa](https://github.com/wmulabs/eywa) ⭐7

生产级 Go 对话 agent 框架:LLM 抽象层 + RAG + memory + 调度 + 可观测性内置。工程化程度高,适合参考生产级 RAG 的可观测与调度设计。

---

## 四、对照 mindbridge roadmap

下表将上述项目已实现的能力与 mindbridge CLAUDE.md 中的 TODO 项对照:

| mindbridge TODO | go-rag | OpsMind | hexclaw | eywa |
|---|:---:|:---:|:---:|:---:|
| BM25 混合检索 + RRF 融合 | ✅ 多路召回 | ✅ BM25+向量 | — | — |
| Reranker 接入 | — | ✅ 重排序 | — | — |
| 结构化切分(PDF/HTML/表格) | ✅ pdf/html | — | — | — |
| chunk 编辑 | ✅ | — | — | — |
| 评估闭环(eval set + RAGAS) | ✅ 自动 QA 对 | — | — | — |
| MCP 协议 | ✅ | — | ✅ | — |
| 多知识库隔离 | ✅ | ✅ | ✅ | — |

---

## 五、结论与下一步

**首推 [wangle201210/go-rag](https://github.com/wangle201210/go-rag)** 作为 mindbridge 的主要参考项目,理由:

1. 技术栈一致(eino + Go + Qdrant + 多知识库)
2. 已实现 mindbridge 多条 roadmap TODO,可直接对照源码学习
3. 维护活跃(最近更新于 2026-06-30)

建议重点阅读其以下实现,对照 mindbridge 自身代码:

- **混合检索 + RRF 融合** → 对照 `internal/rag/service.go` 的占位截断
- **chunk 切分策略(PDF/HTML 结构化)** → 对照 `internal/doc/splitter.go` 的 token 切分
- **自动 QA 生成 / eval** → 对照 mindbridge 未实现的评估闭环
- **多路召回编排** → 对照 `internal/rag/query.go` 的查询改写

次要参考:

- **混合检索 + reranker 实现** → [int2t05/OpsMind](https://github.com/int2t05/OpsMind)
- **生产级可观测 / 调度** → [wmulabs/eywa](https://github.com/wmulabs/eywa)
- **eino 标准用法** → [cloudwego/eino-examples](https://github.com/cloudwego/eino-examples)

---

## 数据来源

- GitHub Search API(`language:go + RAG + knowledge base` / `eino + language:go` / `language:go + RAG + chatbot`,按 star 降序)
- 各仓库 README
- star 数与更新时间为 2026-07-01 快照,后续会有变化
