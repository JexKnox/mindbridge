// Package config 加载并解析 mindbridge 配置。
// 配置来源:YAML 文件(默认 configs/config.yaml),其中 ${VAR} 占位会从环境变量替换。
package config

import (
	"fmt"
	"os"
	"regexp"

	"github.com/JexKnox/mindbridge/internal/logging"
	"gopkg.in/yaml.v3"
)

// Config 是全局配置根。
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Postgres PostgresConfig `yaml:"postgres"`
	Qdrant   QdrantConfig   `yaml:"qdrant"`
	Models   ModelsConfig   `yaml:"models"`
	Ingest   IngestConfig   `yaml:"ingest"`
	RAG      RAGConfig      `yaml:"rag"`
	Agent    AgentConfig    `yaml:"agent"`
	Logging  logging.Config `yaml:"logging"`
	Metrics  MetricsConfig  `yaml:"metrics"`
}

// MetricsConfig 控制 Prometheus 指标暴露。Addr 为独立监听地址(与业务端口分离),
// 置空字符串可关闭;默认 ":9091"。
type MetricsConfig struct {
	Addr *string `yaml:"addr"`
}

// ListenAddr 返回 /metrics 监听地址;yaml 未配置时默认 ":9091",显式配置空串表示关闭。
func (m MetricsConfig) ListenAddr() string {
	if m.Addr == nil {
		return ":9091"
	}
	return *m.Addr
}

type ServerConfig struct {
	Addr      string `yaml:"addr"`
	StaticDir string `yaml:"static_dir"` // 前端构建产物目录,默认 frontend/dist;不存在则跳过静态托管
}

type PostgresConfig struct {
	DSN string `yaml:"dsn"`
}

type QdrantConfig struct {
	Host   string `yaml:"host"`
	Port   int    `yaml:"port"`
	APIKey string `yaml:"api_key"`
}

type ModelsConfig struct {
	Chat      ModelConfig `yaml:"chat"`
	Embedding ModelConfig `yaml:"embedding"`
	// Reranker 为 cross-encoder 重排服务(通用 /rerank API:Jina/Cohere/SiliconFlow/TEI 兼容)。
	// model 留空则禁用重排,退化为按召回融合序截断。
	Reranker ModelConfig `yaml:"reranker"`
	// Rewrite 为 query 改写用的小模型(建议便宜快的);model 留空则禁用改写。
	Rewrite ModelConfig `yaml:"rewrite"`
}

// ModelConfig 描述一个模型 Provider 的连接信息。
// provider 当前仅 "openai"(OpenAI 兼容),新增 Provider 时扩展此处与 adapter。
type ModelConfig struct {
	Provider string `yaml:"provider"`
	BaseURL  string `yaml:"base_url"`
	APIKey   string `yaml:"api_key"`
	Model    string `yaml:"model"`
}

type IngestConfig struct {
	ChunkSize    int      `yaml:"chunk_size"`
	ChunkOverlap int      `yaml:"chunk_overlap"`
	Separators   []string `yaml:"separators"`
	EmbedBatch   int      `yaml:"embed_batch"`  // 单次 embedding 请求文本数,默认 64
	UpsertBatch  int      `yaml:"upsert_batch"` // 单次 Qdrant upsert 点数,默认 128
}

type RAGConfig struct {
	RetrieveTopK    int     `yaml:"retrieve_top_k"`
	RerankTopK      int     `yaml:"rerank_top_k"`
	RefuseThreshold float64 `yaml:"refuse_threshold"`
	Hybrid          *bool   `yaml:"hybrid"`    // BM25 混合检索开关,默认开;旧 collection 自动降级纯向量
	RewriteN        int     `yaml:"rewrite_n"` // query 改写数量(不含原 query),默认 2;需配置 models.rewrite
}

// HybridEnabled 返回混合检索是否开启(默认开)。
func (r RAGConfig) HybridEnabled() bool {
	return r.Hybrid == nil || *r.Hybrid
}

// AgentConfig 描述 agent 链路(检索→决策→调工具→生成)的配置。
// 与 rag 配置并列:rag 是固定"检索→生成",agent 是 agentic RAG + 工具调用。
type AgentConfig struct {
	MaxStep    int               `yaml:"max_step"`    // ReAct 最大步数,默认 24
	MCPServers []MCPServerConfig `yaml:"mcp_servers"` // 配置文件种子,启动时 upsert 进 DB
}

// MCPServerConfig 描述一个外部 MCP server 的连接信息。
// transport 当前支持 "sse" / "http"(stdio 留待后续)。
type MCPServerConfig struct {
	Name      string `yaml:"name"`
	Transport string `yaml:"transport"`
	Endpoint  string `yaml:"endpoint"`
}

// Load 从 path 读取 YAML 配置并替换其中的 ${VAR} 环境变量占位。
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	raw = expandEnv(raw)

	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	c.applyDefaults()
	return &c, nil
}

var envRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// expandEnv 把 ${VAR} 替换成 os.Getenv("VAR");未设置则替换为空串。
func expandEnv(raw []byte) []byte {
	return envRe.ReplaceAllFunc(raw, func(m []byte) []byte {
		name := envRe.FindSubmatch(m)[1]
		return []byte(os.Getenv(string(name)))
	})
}

func (c *Config) applyDefaults() {
	if c.Server.Addr == "" {
		c.Server.Addr = ":8080"
	}
	if c.Server.StaticDir == "" {
		c.Server.StaticDir = "frontend/dist"
	}
	if c.Ingest.ChunkSize <= 0 {
		c.Ingest.ChunkSize = 400
	}
	if c.Ingest.ChunkOverlap < 0 {
		c.Ingest.ChunkOverlap = 0
	}
	if len(c.Ingest.Separators) == 0 {
		c.Ingest.Separators = []string{"\n\n", "\n", " "}
	}
	if c.RAG.RetrieveTopK <= 0 {
		c.RAG.RetrieveTopK = 20
	}
	if c.RAG.RerankTopK <= 0 {
		c.RAG.RerankTopK = 6
	}
	if c.RAG.RefuseThreshold <= 0 {
		c.RAG.RefuseThreshold = 0.3
	}
	if c.RAG.RewriteN <= 0 {
		c.RAG.RewriteN = 2
	}
	if c.Ingest.EmbedBatch <= 0 {
		c.Ingest.EmbedBatch = 64
	}
	if c.Ingest.UpsertBatch <= 0 {
		c.Ingest.UpsertBatch = 128
	}
	if c.Agent.MaxStep <= 0 {
		c.Agent.MaxStep = 24
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
}
