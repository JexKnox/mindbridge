// Package skills 是内置 Go 工具注册表(agent 的"skill"能力)。
// 每个 skill 是一个用 utils.InferTool 从 Go 函数+结构体推断出的 eino InvokableTool。
// 新增 skill = 加一个文件 + 在 NewSkills 里注册一行,业务层零改动。
package skills

import (
	"github.com/cloudwego/eino/components/tool"
)

// Skills 持有内置工具列表。不可变,启动时装配一次。
type Skills struct {
	tools []tool.InvokableTool
}

// NewSkills 注册全部内置 skill。
func NewSkills() (*Skills, error) {
	var tools []tool.InvokableTool

	if t, err := newCurrentTimeTool(); err == nil {
		tools = append(tools, t)
	} else {
		return nil, err
	}
	if t, err := newCalculatorTool(); err == nil {
		tools = append(tools, t)
	} else {
		return nil, err
	}
	if t, err := newWebFetchTool(); err == nil {
		tools = append(tools, t)
	} else {
		return nil, err
	}
	return &Skills{tools: tools}, nil
}

// List 返回全部内置工具。
func (s *Skills) List() []tool.InvokableTool { return s.tools }
