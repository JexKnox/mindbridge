// Package server 是 HTTP 接口层:Hertz handler 只做参数解析与响应组装,
// 业务逻辑沉到 internal/ 下各包。知识库是头等概念,文档/对话接口都带 kb_id。
package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/hertz-contrib/requestid"

	"github.com/JexKnox/mindbridge/internal/adapter"
	"github.com/JexKnox/mindbridge/internal/agent"
	"github.com/JexKnox/mindbridge/internal/cache"
	"github.com/JexKnox/mindbridge/internal/config"
	"github.com/JexKnox/mindbridge/internal/doc"
	"github.com/JexKnox/mindbridge/internal/kb"
	"github.com/JexKnox/mindbridge/internal/logging"
	mindmcp "github.com/JexKnox/mindbridge/internal/mcp"
	"github.com/JexKnox/mindbridge/internal/metrics"
	"github.com/JexKnox/mindbridge/internal/trace"
	"go.uber.org/zap"
)

// Deps 是 handler 的依赖集合,启动时在 main 装配。
type Deps struct {
	Cfg      *config.Config
	Logger   *zap.Logger
	Qdrant   *adapter.QdrantClient
	KB       *kb.Manager
	Pipeline *doc.Pipeline
	Agent    *agent.Service
	MCPRepo  *mindmcp.Repo
	Caches   *cache.RAGCaches
	Traces   *trace.Repo
}

// New 装配 Hertz 路由并返回 *server.Hertz,由 main 调用 h.Spin() 启动。
func New(d Deps) *server.Hertz {
	h := server.Default(server.WithHostPorts(d.Cfg.Server.Addr))

	h.Use(requestid.New())
	h.Use(logging.AccessLog(d.Logger))
	h.Use(metricsMiddleware())

	ctl := &handlers{deps: d}

	// /api/kb:列表与建库。Hertz 的 Group + GET("/kb") 平铺注册,
	// 避免子组 GET("/") 是否匹配无尾斜杠路径的歧义。
	api := h.Group("/api")
	api.GET("/kb", ctl.listKB)
	api.POST("/kb", ctl.createKB)
	api.POST("/kb/:id/documents", ctl.uploadDocument)
	api.POST("/kb/:id/chat", ctl.chat)

	// agent 链路(agentic RAG + 工具调用),与 RAG chat 路径并列、互不影响。
	api.POST("/agent/chat", ctl.agentChat)

	// MCP server 注册表 CRUD:agent 据此连接外部 MCP server。
	api.GET("/mcp/servers", ctl.listMCPServers)
	api.POST("/mcp/servers", ctl.createMCPServer)
	api.DELETE("/mcp/servers/:id", ctl.deleteMCPServer)

	// 质量闭环:用户反馈 + 统计总览 + 评测集导出。
	api.POST("/feedback", ctl.createFeedback)
	api.GET("/stats/overview", ctl.statsOverview)
	api.GET("/stats/export", ctl.exportEval)

	h.GET("/healthz", func(ctx context.Context, c *app.RequestContext) {
		c.Data(200, "text/plain", []byte("ok"))
	})

	// 前端静态托管:产物目录存在时服务,文件不存在回退 index.html(SPA)。
	mountSPA(h, d.Cfg.Server.StaticDir)

	return h
}

// mountSPA 注册前端静态文件服务。staticDir 不存在时静默跳过(纯后端开发场景)。
func mountSPA(h *server.Hertz, staticDir string) {
	if staticDir == "" {
		return
	}
	if info, err := os.Stat(staticDir); err != nil || !info.IsDir() {
		return
	}
	abs, err := filepath.Abs(staticDir)
	if err != nil {
		abs = staticDir
	}
	indexHTML := filepath.Join(abs, "index.html")

	// Static 服务静态文件;NoRoute 兜底:非 /api 路径回退 index.html,支持前端路由刷新/深链。
	h.Static("/", abs)
	h.NoRoute(func(ctx context.Context, c *app.RequestContext) {
		path := string(c.Path())
		if strings.HasPrefix(path, "/api") {
			c.JSON(404, utils.H{"error": "not found"})
			return
		}
		c.File(indexHTML)
	})
}

type handlers struct {
	deps Deps
}

// metricsMiddleware 记录请求量/状态码/耗时。path 用注册路由(如 /api/kb/:id/chat)
// 而非真实 URL,避免 kb_id 等参数造成指标高基数。
func metricsMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		start := time.Now()
		c.Next(ctx)
		path := c.FullPath()
		if path == "" {
			path = "unmatched" // 静态资源/SPA 回退等未注册路由,归并为一类
		}
		metrics.RecordHTTP(string(c.Method()), path, c.Response.StatusCode(), time.Since(start))
	}
}
