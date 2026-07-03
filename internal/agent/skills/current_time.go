package skills

import (
	"context"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// currentTimeInput 无参数;InferTool 会生成空参数 schema。
type currentTimeInput struct{}

// newCurrentTimeTool 返回当前时间的工具。供 agent 在需要"现在几点/今天日期"时调用,
// 避免模型凭训练数据猜测当前时间。
func newCurrentTimeTool() (tool.InvokableTool, error) {
	return utils.InferTool(
		"current_time",
		"返回当前的时间与日期(本地时区)。当用户问\"现在几点\"\"今天几号\"等问题时调用。",
		func(ctx context.Context, _ currentTimeInput) (string, error) {
			return time.Now().Format("2006-01-02 15:04:05 (MST)"), nil
		},
	)
}
