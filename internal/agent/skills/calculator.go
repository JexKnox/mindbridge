package skills

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type calculatorInput struct {
	Expression string `json:"expression" jsonschema:"description=要计算的算术表达式,仅支持数字与 + - * / 以及括号"`
}

// newCalculatorTool 安全的算术求值工具。用 go/ast 解析,只允许数字字面量与四则运算,
// 拒绝函数调用、标识符等,杜绝任意代码执行。
func newCalculatorTool() (tool.InvokableTool, error) {
	return utils.InferTool(
		"calculator",
		"对算术表达式求值,仅支持数字与 + - * / 及括号。需要精确数值计算时调用。",
		func(ctx context.Context, in calculatorInput) (string, error) {
			v, err := evalArith(in.Expression)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%g", v), nil
		},
	)
}

// evalArith 解析并求值算术表达式,只允许基本算术运算。
func evalArith(expr string) (float64, error) {
	if expr == "" {
		return 0, fmt.Errorf("空表达式")
	}
	node, err := parser.ParseExpr(expr)
	if err != nil {
		return 0, fmt.Errorf("解析表达式失败: %w", err)
	}
	return evalNode(node)
}

func evalNode(n ast.Node) (float64, error) {
	switch e := n.(type) {
	case *ast.BasicLit:
		switch e.Kind {
		case token.INT, token.FLOAT:
			v, err := strconv.ParseFloat(e.Value, 64)
			if err != nil {
				return 0, err
			}
			return v, nil
		}
		return 0, fmt.Errorf("不支持的字面量 %q", e.Value)
	case *ast.ParenExpr:
		return evalNode(e.X)
	case *ast.BinaryExpr:
		l, err := evalNode(e.X)
		if err != nil {
			return 0, err
		}
		r, err := evalNode(e.Y)
		if err != nil {
			return 0, err
		}
		switch e.Op {
		case token.ADD:
			return l + r, nil
		case token.SUB:
			return l - r, nil
		case token.MUL:
			return l * r, nil
		case token.QUO:
			if r == 0 {
				return 0, fmt.Errorf("除数为零")
			}
			return l / r, nil
		}
		return 0, fmt.Errorf("不支持的运算符 %v", e.Op)
	case *ast.UnaryExpr:
		v, err := evalNode(e.X)
		if err != nil {
			return 0, err
		}
		if e.Op == token.SUB {
			return -v, nil
		}
		return 0, fmt.Errorf("不支持的一元运算符 %v", e.Op)
	default:
		return 0, fmt.Errorf("不支持的表达式节点类型 %T", n)
	}
}
