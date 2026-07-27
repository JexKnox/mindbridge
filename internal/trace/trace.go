// Package trace 实现问答留痕与用户反馈闭环:
// 每次 RAG 问答存一条 trace(query/召回证据/回答/引用),
// 用户反馈按分类挂在 trace 上,LLM Judge 抽样评分写回 trace。
// 三者共同支撑质量统计(拒答率、反馈分类分布、faithfulness 均分)与评测集沉淀。
package trace

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Trace 是一次问答的完整留痕。
type Trace struct {
	ID         string          `json:"id"`
	KBID       string          `json:"kb_id"`
	Query      string          `json:"query"`
	Answer     string          `json:"answer"`
	Citations  json.RawMessage `json:"citations"`
	Contexts   []string        `json:"contexts"`
	TopScore   float64         `json:"top_score"`
	Refused    bool            `json:"refused"`
	DurationMS int64           `json:"duration_ms"`
	CreatedAt  time.Time       `json:"created_at"`
}

// FeedbackCategories 是允许的反馈分类(与前端选项对齐)。
// 分类而非布尔值:分类后的反馈才能进入下一轮评测集。
var FeedbackCategories = map[string]bool{
	"helpful":        true, // 有帮助
	"not_found":      true, // 没找到资料
	"outdated":       true, // 资料过期
	"irrelevant":     true, // 答非所问
	"not_actionable": true, // 步骤不可执行
	"other":          true, // 其他
}

type Repo struct {
	db *sql.DB
}

func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

// Insert 写入一条问答留痕。
func (r *Repo) Insert(ctx context.Context, t *Trace) error {
	cits := t.Citations
	if len(cits) == 0 {
		cits = json.RawMessage("[]")
	}
	ctxsJSON, err := json.Marshal(t.Contexts)
	if err != nil {
		return fmt.Errorf("trace: marshal contexts: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO chat_traces (id, kb_id, query, answer, citations, contexts, top_score, refused, duration_ms)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		t.ID, t.KBID, t.Query, t.Answer, string(cits), string(ctxsJSON), t.TopScore, t.Refused, t.DurationMS)
	if err != nil {
		return fmt.Errorf("trace: insert: %w", err)
	}
	return nil
}

// AddFeedback 给 trace 追加一条分类反馈。category 必须在 FeedbackCategories 内。
func (r *Repo) AddFeedback(ctx context.Context, traceID, category, comment string) error {
	if !FeedbackCategories[category] {
		return fmt.Errorf("trace: invalid feedback category %q", category)
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO chat_feedback (id, trace_id, category, comment) VALUES ($1,$2,$3,$4)`,
		uuid.NewString(), traceID, category, comment)
	if err != nil {
		return fmt.Errorf("trace: add feedback: %w", err)
	}
	return nil
}

// ── LLM Judge 抽样评分 ─────────────────────────────────────────────────

// UnjudgedTrace 是待评审 trace 的评审所需字段。
type UnjudgedTrace struct {
	ID       string
	Query    string
	Answer   string
	Contexts []string
}

// ListUnjudged 取最多 limit 条未评审且未拒答的 trace(拒答无需评 faithfulness)。
func (r *Repo) ListUnjudged(ctx context.Context, limit int) ([]UnjudgedTrace, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, query, answer, contexts FROM chat_traces
		WHERE judged_at IS NULL AND refused = FALSE AND answer <> ''
		ORDER BY created_at ASC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("trace: list unjudged: %w", err)
	}
	defer rows.Close()

	var out []UnjudgedTrace
	for rows.Next() {
		var t UnjudgedTrace
		var ctxsRaw []byte
		if err := rows.Scan(&t.ID, &t.Query, &t.Answer, &ctxsRaw); err != nil {
			return nil, fmt.Errorf("trace: scan unjudged: %w", err)
		}
		_ = json.Unmarshal(ctxsRaw, &t.Contexts)
		out = append(out, t)
	}
	return out, rows.Err()
}

// SetJudge 写回评审结果。score 为 nil 表示未抽中/评审失败,只记 verdict 防止重复拉取。
func (r *Repo) SetJudge(ctx context.Context, id string, score *float64, verdict string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE chat_traces SET judge_score = $2, judge_verdict = $3, judged_at = now() WHERE id = $1`,
		id, score, verdict)
	if err != nil {
		return fmt.Errorf("trace: set judge: %w", err)
	}
	return nil
}

// ── 统计 ────────────────────────────────────────────────────────────────

// CategoryCount 是某反馈分类的计数。
type CategoryCount struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

// DayCount 是某日问答量。
type DayCount struct {
	Day     string `json:"day"`
	Answers int    `json:"answers"`
	Refused int    `json:"refused"`
}

// Overview 是统计总览。
type Overview struct {
	Days          int             `json:"days"`
	Answers       int             `json:"answers"`
	Refusals      int             `json:"refusals"`
	AvgTopScore   float64         `json:"avg_top_score"`
	JudgedCount   int             `json:"judged_count"`
	AvgJudgeScore float64         `json:"avg_judge_score"`
	Feedback      []CategoryCount `json:"feedback"`
	Daily         []DayCount      `json:"daily"`
}

// GetOverview 聚合最近 days 天的问答/拒答/Judge/反馈统计。kbID 为空表示全部知识库。
func (r *Repo) GetOverview(ctx context.Context, days int, kbID string) (*Overview, error) {
	if days <= 0 {
		days = 30
	}
	since := time.Now().AddDate(0, 0, -days)
	o := &Overview{Days: days, Feedback: []CategoryCount{}, Daily: []DayCount{}}

	err := r.db.QueryRowContext(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE refused),
		       coalesce(avg(top_score) FILTER (WHERE NOT refused), 0),
		       count(*) FILTER (WHERE judge_score IS NOT NULL),
		       coalesce(avg(judge_score), 0)
		FROM chat_traces
		WHERE created_at >= $1 AND ($2 = '' OR kb_id = $2)`, since, kbID).
		Scan(&o.Answers, &o.Refusals, &o.AvgTopScore, &o.JudgedCount, &o.AvgJudgeScore)
	if err != nil {
		return nil, fmt.Errorf("trace: overview totals: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT f.category, count(*)
		FROM chat_feedback f JOIN chat_traces t ON t.id = f.trace_id
		WHERE f.created_at >= $1 AND ($2 = '' OR t.kb_id = $2)
		GROUP BY f.category ORDER BY count(*) DESC`, since, kbID)
	if err != nil {
		return nil, fmt.Errorf("trace: overview feedback: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c CategoryCount
		if err := rows.Scan(&c.Category, &c.Count); err != nil {
			return nil, err
		}
		o.Feedback = append(o.Feedback, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	dayRows, err := r.db.QueryContext(ctx, `
		SELECT to_char(date_trunc('day', created_at), 'YYYY-MM-DD'),
		       count(*),
		       count(*) FILTER (WHERE refused)
		FROM chat_traces
		WHERE created_at >= $1 AND ($2 = '' OR kb_id = $2)
		GROUP BY 1 ORDER BY 1`, since, kbID)
	if err != nil {
		return nil, fmt.Errorf("trace: overview daily: %w", err)
	}
	defer dayRows.Close()
	for dayRows.Next() {
		var d DayCount
		if err := dayRows.Scan(&d.Day, &d.Answers, &d.Refused); err != nil {
			return nil, err
		}
		o.Daily = append(o.Daily, d)
	}
	return o, dayRows.Err()
}

// ExportItem 是评测集导出条目,与 cmd/mindbridge-eval 的 JSONL 格式对齐;
// relevant_files 需人工标注后方可用于评测。
type ExportItem struct {
	Question      string   `json:"question"`
	RelevantFiles []string `json:"relevant_files"`
	Category      string   `json:"category"`
	KBID          string   `json:"kb_id"`
}

// ExportByCategory 导出某分类反馈对应的问题(去重),沉淀为下一轮评测集素材。
// category 为空表示导出所有负反馈(helpful 之外的分类)。
func (r *Repo) ExportByCategory(ctx context.Context, days int, category string) ([]ExportItem, error) {
	if days <= 0 {
		days = 30
	}
	since := time.Now().AddDate(0, 0, -days)
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT ON (t.query) t.query, f.category, t.kb_id
		FROM chat_feedback f JOIN chat_traces t ON t.id = f.trace_id
		WHERE f.created_at >= $1
		  AND (($2 <> '' AND f.category = $2) OR ($2 = '' AND f.category <> 'helpful'))
		ORDER BY t.query, f.created_at DESC`, since, category)
	if err != nil {
		return nil, fmt.Errorf("trace: export: %w", err)
	}
	defer rows.Close()

	var out []ExportItem
	for rows.Next() {
		item := ExportItem{RelevantFiles: []string{}}
		if err := rows.Scan(&item.Question, &item.Category, &item.KBID); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
