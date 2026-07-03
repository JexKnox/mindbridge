package adapter

import (
	"context"
	"fmt"
	"time"

	mcpp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	mindmcp "github.com/JexKnox/mindbridge/internal/mcp"
)

// MCPConnectTimeout 是单次连接 + 初始化 MCP server 的超时。
// MCP server 可能不可达,设短超时避免拖垮 agent 请求。
const MCPConnectTimeout = 5 * time.Second

// NewMCPTools 连接一个 MCP server,拉取其暴露的全部工具,转成 eino BaseTool 列表。
// transport 集中在此分发(sse / http);连接失败返回 error,由调用方决定跳过还是中断。
// 调用方负责在用完后调用返回的 close(本实现里 close 持有 client 句柄)。
func NewMCPTools(ctx context.Context, srv *mindmcp.Server) (tools []tool.BaseTool, close func() error, err error) {
	ctx, cancel := context.WithTimeout(ctx, MCPConnectTimeout)
	defer cancel()

	cli, err := newMCPClient(ctx, srv)
	if err != nil {
		return nil, nil, fmt.Errorf("mcp %q: new client: %w", srv.Name, err)
	}
	if err := cli.Start(ctx); err != nil {
		_ = cli.Close()
		return nil, nil, fmt.Errorf("mcp %q: start: %w", srv.Name, err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "mindbridge",
		Version: "0.1.0",
	}
	if _, err := cli.Initialize(ctx, initReq); err != nil {
		_ = cli.Close()
		return nil, nil, fmt.Errorf("mcp %q: initialize: %w", srv.Name, err)
	}

	tools, err = mcpp.GetTools(ctx, &mcpp.Config{Cli: cli})
	if err != nil {
		_ = cli.Close()
		return nil, nil, fmt.Errorf("mcp %q: get tools: %w", srv.Name, err)
	}
	return tools, cli.Close, nil
}

// newMCPClient 按 transport 构造 mcp-go 客户端。新增 transport 在此分支扩展。
func newMCPClient(_ context.Context, srv *mindmcp.Server) (*mcpclient.Client, error) {
	switch srv.Transport {
	case mindmcp.TransportSSE, "":
		return mcpclient.NewSSEMCPClient(srv.Endpoint)
	case mindmcp.TransportHTTP:
		return mcpclient.NewStreamableHttpClient(srv.Endpoint)
	default:
		return nil, fmt.Errorf("unsupported mcp transport %q", srv.Transport)
	}
}
