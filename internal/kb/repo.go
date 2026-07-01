package kb

import (
	"context"
	"database/sql"
	"fmt"
)

// Repo 是 KB 元信息的持久化接口(Postgres 实现)。
type Repo struct {
	db *sql.DB
}

func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

// Create 写入一条 KB 元信息。
func (r *Repo) Create(ctx context.Context, kb *KB) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO kb (id, name, collection, embedding_model, embedding_dim)
		VALUES ($1, $2, $3, $4, $5)`,
		kb.ID, kb.Name, kb.Collection, kb.EmbeddingModel, kb.EmbeddingDim)
	if err != nil {
		return fmt.Errorf("kb repo: create: %w", err)
	}
	return nil
}

// Get 按 ID 取一条。
func (r *Repo) Get(ctx context.Context, id string) (*KB, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, collection, embedding_model, embedding_dim, created_at
		FROM kb WHERE id = $1`, id)
	var kb KB
	if err := row.Scan(&kb.ID, &kb.Name, &kb.Collection, &kb.EmbeddingModel, &kb.EmbeddingDim, &kb.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("kb repo: get: %w", err)
	}
	return &kb, nil
}

// List 列出全部。
func (r *Repo) List(ctx context.Context) ([]*KB, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, collection, embedding_model, embedding_dim, created_at
		FROM kb ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("kb repo: list: %w", err)
	}
	defer rows.Close()
	var out []*KB
	for rows.Next() {
		var kb KB
		if err := rows.Scan(&kb.ID, &kb.Name, &kb.Collection, &kb.EmbeddingModel, &kb.EmbeddingDim, &kb.CreatedAt); err != nil {
			return nil, fmt.Errorf("kb repo: scan: %w", err)
		}
		out = append(out, &kb)
	}
	return out, rows.Err()
}

// ErrNotFound 表示 KB 不存在。
var ErrNotFound = fmt.Errorf("kb not found")
