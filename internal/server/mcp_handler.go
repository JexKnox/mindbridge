package server

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"

	mindmcp "github.com/JexKnox/mindbridge/internal/mcp"
)

// MCP server CRUD:agent 链路据此连接外部 MCP server 拉取工具。
// 配置文件作初始种子(启动 upsert),DB 作运行时可变来源。

func (h *handlers) listMCPServers(ctx context.Context, c *app.RequestContext) {
	srvs, err := h.deps.MCPRepo.List(ctx)
	if err != nil {
		writeErr(c, 500, err.Error())
		return
	}
	c.JSON(200, srvs)
}

func (h *handlers) createMCPServer(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Name      string `json:"name"`
		Transport string `json:"transport"`
		Endpoint  string `json:"endpoint"`
	}
	if err := c.Bind(&req); err != nil {
		writeErr(c, 400, "invalid body: "+err.Error())
		return
	}
	if req.Name == "" || req.Endpoint == "" {
		writeErr(c, 400, "name and endpoint are required")
		return
	}
	if req.Transport != mindmcp.TransportSSE && req.Transport != mindmcp.TransportHTTP {
		writeErr(c, 400, "transport must be \"sse\" or \"http\"")
		return
	}
	s, err := h.deps.MCPRepo.Create(ctx, &mindmcp.Server{
		Name: req.Name, Transport: req.Transport, Endpoint: req.Endpoint,
	})
	if err != nil {
		writeErr(c, 500, err.Error())
		return
	}
	c.JSON(201, s)
}

func (h *handlers) deleteMCPServer(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	if err := h.deps.MCPRepo.Delete(ctx, id); err != nil {
		writeErr(c, 500, err.Error())
		return
	}
	c.JSON(200, map[string]any{"deleted": id})
}
