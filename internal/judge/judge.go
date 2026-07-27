// Package judge 实现线上抽样评测:后台任务定期抽取问答留痕,
// 用稳定的 LLM Judge 检查回答是否被检索证据支撑(faithfulness,0~1),写回 trace。
// 成本靠抽样率控制;未抽中/评审失败的 trace 也标记 judged,避免重复拉取。
package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"github.com/JexKnox/mindbridge/internal/metrics"
	"github.com/JexKnox/mindbridge/internal/trace"
)

// Judge 是后台抽样评审器。
type Judge struct {
	traces     *trace.Repo
	chat       model.ChatModel
	sampleRate float64
	interval   time.Duration
	batch      int
	logger     *zap.Logger
}

// New 构造 Judge。sampleRate 为 (0,1] 的抽样率;interval 为轮询间隔;batch 为单轮最多处理条数。
func New(traces *trace.Repo, chat model.ChatModel, sampleRate float64, interval time.Duration, batch int, logger *zap.Logger) *Judge {
	if sampleRate <= 0 || sampleRate > 1 {
		sampleRate = 0.2
	}
	if interval <= 0 {
		interval = time.Minute
	}
	if batch <= 0 {
		batch = 8
	}
	return &Judge{traces: traces, chat: chat, sampleRate: sampleRate, interval: interval, batch: batch, logger: logger}
}

// Run 阻塞轮询,应在独立 goroutine 中调用;ctx 取消时退出。
func (j *Judge) Run(ctx context.Context) {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			j.tick(ctx)
		}
	}
}

func (j *Judge) tick(ctx context.Context) {
	items, err := j.traces.ListUnjudged(ctx, j.batch)
	if err != nil {
		j.logger.Warn("judge: list unjudged failed", zap.Error(err))
		return
	}
	for _, t := range items {
		// 抽样:未抽中的标记 skipped,不再重复拉取。
		if rand.Float64() > j.sampleRate {
			_ = j.traces.SetJudge(ctx, t.ID, nil, "skipped")
			continue
		}
		score, verdict, err := j.evaluate(ctx, t)
		if err != nil {
			j.logger.Warn("judge: evaluate failed", zap.String("trace", t.ID), zap.Error(err))
			// 失败也标记,避免坏样本堵塞队列;verdict 记录失败原因。
			_ = j.traces.SetJudge(ctx, t.ID, nil, "judge_error: "+truncate(err.Error(), 200))
			continue
		}
		if err := j.traces.SetJudge(ctx, t.ID, &score, verdict); err != nil {
			j.logger.Warn("judge: save result failed", zap.String("trace", t.ID), zap.Error(err))
		}
	}
}

const judgePrompt = `你是 RAG 回答质量评审员。给定用户问题、检索到的参考资料和系统的回答,评估回答的忠实度(faithfulness):回答中的事实是否都能被参考资料支撑,是否存在编造。
只输出一个 JSON 对象,格式:{"score": 0.0到1.0的数值, "verdict": "一句话中文判定"}
评分标准:1.0 = 全部事实有据;0.5 = 部分有据部分无据;0.0 = 大量编造或与资料矛盾。`

// evaluate 调 LLM 对单条 trace 打分。
func (j *Judge) evaluate(ctx context.Context, t trace.UnjudgedTrace) (float64, string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "【用户问题】\n%s\n\n【参考资料】\n", t.Query)
	for i, c := range t.Contexts {
		fmt.Fprintf(&b, "[%d] %s\n\n", i+1, truncate(c, 1500))
	}
	fmt.Fprintf(&b, "【系统回答】\n%s", truncate(t.Answer, 4000))

	msgs := []*schema.Message{
		schema.SystemMessage(judgePrompt),
		schema.UserMessage(b.String()),
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	start := time.Now()
	resp, err := j.chat.Generate(cctx, msgs)
	metrics.ObserveModel("judge", start, err)
	if err != nil {
		return 0, "", fmt.Errorf("generate: %w", err)
	}
	return parseVerdict(resp.Content)
}

// parseVerdict 从模型输出中提取 JSON(容忍前后多余文本/代码围栏)。
func parseVerdict(content string) (float64, string, error) {
	s := strings.Index(content, "{")
	e := strings.LastIndex(content, "}")
	if s < 0 || e <= s {
		return 0, "", fmt.Errorf("no JSON object in judge output: %q", truncate(content, 200))
	}
	var out struct {
		Score   float64 `json:"score"`
		Verdict string  `json:"verdict"`
	}
	if err := json.Unmarshal([]byte(content[s:e+1]), &out); err != nil {
		return 0, "", fmt.Errorf("decode judge output: %w", err)
	}
	if out.Score < 0 {
		out.Score = 0
	}
	if out.Score > 1 {
		out.Score = 1
	}
	return out.Score, out.Verdict, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
