package skills

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type webFetchInput struct {
	URL string `json:"url" jsonschema:"description=要抓取的网页 URL,需包含 http(s)://"`
}

const (
	webFetchTimeout  = 10 * time.Second
	webFetchMaxBytes = 64 * 1024 // 限制抓取体积,避免拉超大页面拖垮 agent
)

// newWebFetchTool 抓取指定 URL,剥离 HTML 标签返回纯文本。
// 供 agent 在需要外部实时信息(且未接 MCP 工具)时兜底使用。
func newWebFetchTool() (tool.InvokableTool, error) {
	return utils.InferTool(
		"web_fetch",
		"抓取指定网页 URL 并返回纯文本内容(自动去 HTML 标签)。需要获取某个网页的实时内容时调用。",
		func(ctx context.Context, in webFetchInput) (string, error) {
			return fetchPlainText(ctx, in.URL)
		},
	)
}

func fetchPlainText(ctx context.Context, rawURL string) (string, error) {
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return "", fmt.Errorf("URL 必须以 http:// 或 https:// 开头")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "mindbridge-agent/0.1")

	client := &http.Client{Timeout: webFetchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, webFetchMaxBytes))
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}
	return stripHTML(string(body)), nil
}

var (
	scriptRe = regexp.MustCompile(`(?is)<script.*?</script>`)
	styleRe  = regexp.MustCompile(`(?is)<style.*?</style>`)
	tagRe    = regexp.MustCompile(`<[^>]+>`)
	wsRe     = regexp.MustCompile(`\s+`)
)

// stripHTML 粗粒度去标签:先去 script/style,再去标签,合并空白。
func stripHTML(s string) string {
	s = scriptRe.ReplaceAllString(s, " ")
	s = styleRe.ReplaceAllString(s, " ")
	s = tagRe.ReplaceAllString(s, " ")
	s = wsRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}
