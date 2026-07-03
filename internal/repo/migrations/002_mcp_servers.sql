-- MCP server 注册表
-- agent 链路可调用外部 MCP server 暴露的工具。来源:配置文件种子 + 运行时 CRUD。
-- transport: "sse" | "http"(stdio 留待后续,需进程生命周期管理)。
CREATE TABLE IF NOT EXISTS mcp_servers (
    id          TEXT         PRIMARY KEY,
    name        TEXT         NOT NULL UNIQUE,
    transport   TEXT         NOT NULL,
    endpoint    TEXT         NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);
