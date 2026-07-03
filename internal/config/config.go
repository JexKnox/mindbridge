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
}

type RAGConfig struct {
	RetrieveTopK    int     `yaml:"retrieve_top_k"`
	RerankTopK      int     `yaml:"rerank_top_k"`
	RefuseThreshold float64 `yaml:"refuse_threshold"`
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
	if c.Agent.MaxStep <= 0 {
		c.Agent.MaxStep = 24
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
}
