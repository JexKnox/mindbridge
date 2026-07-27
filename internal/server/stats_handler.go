package server

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
)

// createFeedback 记录用户对某次回答的分类反馈(用户反馈闭环)。
// 分类而非布尔值:没找到资料 / 资料过期 / 答非所问 / 步骤不可执行 / 有帮助 / 其他。
func (h *handlers) createFeedback(ctx context.Context, c *app.RequestContext) {
	var body struct {
		TraceID  string `json:"trace_id"`
		Category string `json:"category"`
		Comment  string `json:"comment"`
	}
	if err := c.Bind(&body); err != nil {
		writeErr(c, 400, "invalid body: "+err.Error())
		return
	}
	if body.TraceID == "" || body.Category == "" {
		writeErr(c, 400, "trace_id and category are required")
		return
	}
	if err := h.deps.Traces.AddFeedback(ctx, body.TraceID, body.Category, body.Comment); err != nil {
		writeErr(c, 400, err.Error())
		return
	}
	c.JSON(200, utils.H{"ok": true})
}

// statsOverview 返回最近 N 天的质量统计:问答量、拒答率、top1 均分、
// Judge 抽样均分、反馈分类分布、每日问答量。?days=30&kb_id= 可选。
func (h *handlers) statsOverview(ctx context.Context, c *app.RequestContext) {
	days, _ := strconv.Atoi(c.Query("days"))
	o, err := h.deps.Traces.GetOverview(ctx, days, c.Query("kb_id"))
	if err != nil {
		writeErr(c, 500, err.Error())
		return
	}
	c.JSON(200, o)
}

// exportEval 把分类反馈对应的问题导出为评测集 JSONL(与 cmd/mindbridge-eval 格式对齐,
// relevant_files 留空待人工标注)。?category= 指定分类,缺省导出全部负反馈;?days=30。
func (h *handlers) exportEval(ctx context.Context, c *app.RequestContext) {
	days, _ := strconv.Atoi(c.Query("days"))
	items, err := h.deps.Traces.ExportByCategory(ctx, days, c.Query("category"))
	if err != nil {
		writeErr(c, 500, err.Error())
		return
	}
	var out []byte
	for _, item := range items {
		line, err := json.Marshal(item)
		if err != nil {
			continue
		}
		out = append(out, line...)
		out = append(out, '\n')
	}
	c.Response.Header.Set("Content-Disposition", `attachment; filename="evalset.jsonl"`)
	c.Data(200, "application/x-ndjson", out)
}
