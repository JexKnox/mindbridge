// Package mcp 持久化外部 MCP server 的注册信息。
// 来源:配置文件种子(启动时 upsert)+ 运行时 CRUD(前端增删)。
// agent 链路据此连接各 MCP server,拉取其暴露的工具。
package mcp

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Transport 表示 MCP server 的传输方式。
const (
	TransportSSE  = "sse"
	TransportHTTP = "http" // Streamable HTTP
)

// Server 是一条 MCP server 注册记录。
type Server struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Transport string    `json:"transport"` // sse | http
	Endpoint  string    `json:"endpoint"`
	CreatedAt time.Time `json:"created_at"`
}

// Repo 是 mcp_servers 表的持久化访问层。
type Repo struct {
	db *sql.DB
}

func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

// List 列出全部 MCP server(按创建时间升序,保证种子在前)。
func (r *Repo) List(ctx context.Context) ([]*Server, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, transport, endpoint, created_at
		FROM mcp_servers ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("mcp repo: list: %w", err)
	}
	defer rows.Close()
	var out []*Server
	for rows.Next() {
		var s Server
		if err := rows.Scan(&s.ID, &s.Name, &s.Transport, &s.Endpoint, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("mcp repo: scan: %w", err)
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}

// Create 写入一条;name 唯一冲突时返回错误。
func (r *Repo) Create(ctx context.Context, s *Server) (*Server, error) {
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	if err := r.db.QueryRowContext(ctx, `
		INSERT INTO mcp_servers (id, name, transport, endpoint)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, transport, endpoint, created_at`,
		s.ID, s.Name, s.Transport, s.Endpoint,
	).Scan(&s.ID, &s.Name, &s.Transport, &s.Endpoint, &s.CreatedAt); err != nil {
		return nil, fmt.Errorf("mcp repo: create: %w", err)
	}
	return s, nil
}

// Delete 按 ID 删除。
func (r *Repo) Delete(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM mcp_servers WHERE id = $1`, id); err != nil {
		return fmt.Errorf("mcp repo: delete: %w", err)
	}
	return nil
}

// Seed 把配置文件里的 MCP server 以 name 为唯一键 upsert 进 DB。
// 让配置文件作初始种子、DB 作运行时可变来源:首次启动写入,后续用户改动不被覆盖。
func (r *Repo) Seed(ctx context.Context, servers []Server) error {
	for _, s := range servers {
		if s.Name == "" || s.Endpoint == "" {
			continue
		}
		if _, err := r.db.ExecContext(ctx, `
			INSERT INTO mcp_servers (id, name, transport, endpoint)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (name) DO NOTHING`,
			uuid.NewString(), s.Name, s.Transport, s.Endpoint,
		); err != nil {
			return fmt.Errorf("mcp repo: seed %q: %w", s.Name, err)
		}
	}
	return nil
}
