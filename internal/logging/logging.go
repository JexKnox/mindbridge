// Package logging 提供 zap 日志器与 Hertz 的接入。
// 业务/适配层只依赖 *zap.Logger(经 Deps 注入),不感知具体配置来源;
// Hertz 框架日志则经 hlog 走官方 hertz-contrib/logger/zap 后端。
package logging

import (
	"context"
	"os"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	hzap "github.com/hertz-contrib/logger/zap"
	"github.com/hertz-contrib/requestid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Config 是日志配置,由 config 包从 YAML 加载。
type Config struct {
	Level        string   `yaml:"level"`         // debug/info/warn/error,默认 info
	Encoding     string   `yaml:"encoding"`      // json 或 console,默认 console(开发友好)
	Outputs      []string `yaml:"outputs"`       // 输出路径,默认 ["stdout"]
	ErrorOutputs []string `yaml:"error_outputs"` // 错误输出路径,默认 ["stderr"]
}

// parseLevel 解析 level 字符串为 zapcore.Level,空串默认 info。
func parseLevel(s string) zapcore.Level {
	var l zapcore.Level
	if err := l.UnmarshalText([]byte(s)); err != nil || s == "" {
		l = zapcore.InfoLevel
	}
	return l
}

func encoderFor(encoding string) zapcore.Encoder {
	if encoding == "json" {
		return zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	}
	enc := zap.NewDevelopmentEncoderConfig()
	enc.EncodeTime = zapcore.ISO8601TimeEncoder
	return zapcore.NewConsoleEncoder(enc)
}

// newWriteSyncer 把配置里的路径列表转成 zapcore.WriteSyncer(stdout/stderr 直接连)。
func newWriteSyncer(paths []string) zapcore.WriteSyncer {
	if len(paths) == 0 {
		return zapcore.AddSync(os.Stdout)
	}
	var ws []zapcore.WriteSyncer
	for _, p := range paths {
		switch p {
		case "stdout":
			ws = append(ws, zapcore.AddSync(os.Stdout))
		case "stderr":
			ws = append(ws, zapcore.AddSync(os.Stderr))
		default:
			if f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
				ws = append(ws, zapcore.AddSync(f))
			}
		}
	}
	if len(ws) == 0 {
		return zapcore.AddSync(os.Stdout)
	}
	return zapcore.NewMultiWriteSyncer(ws...)
}

// New 构建 *zap.Logger,供业务/适配层(Deps)使用。
func New(cfg Config) (*zap.Logger, error) {
	level := parseLevel(cfg.Level)
	encoding := cfg.Encoding
	if encoding == "" {
		encoding = "console"
	}
	zcfg := zap.NewProductionConfig()
	zcfg.Level = zap.NewAtomicLevelAt(level)
	zcfg.Encoding = encoding
	zcfg.OutputPaths = cfg.Outputs
	if len(zcfg.OutputPaths) == 0 {
		zcfg.OutputPaths = []string{"stdout"}
	}
	zcfg.ErrorOutputPaths = cfg.ErrorOutputs
	if len(zcfg.ErrorOutputPaths) == 0 {
		zcfg.ErrorOutputPaths = []string{"stderr"}
	}
	zcfg.Sampling = nil
	if encoding == "console" {
		zcfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}
	return zcfg.Build()
}

// SetHlog 用 hertz-contrib/logger/zap 构建 hlog 后端并设为 Hertz 全局 logger。
// 之后 Hertz 框架日志(启动、错误等)经 zap 输出,与业务日志同源。
func SetHlog(cfg Config) {
	level := parseLevel(cfg.Level)
	encoding := cfg.Encoding
	if encoding == "" {
		encoding = "console"
	}
	logger := hzap.NewLogger(
		hzap.WithCoreLevel(zap.NewAtomicLevelAt(level)),
		hzap.WithCoreEnc(encoderFor(encoding)),
		hzap.WithCoreWs(newWriteSyncer(cfg.Outputs)),
	)
	hlog.SetLogger(logger)
}

// AccessLog 是 Hertz 请求日志中间件:按响应状态码分级(5xx Error/4xx Warn/其余 Info),
// 字段含 method/path/status/bytes/duration/request_id。
func AccessLog(logger *zap.Logger) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		start := time.Now()
		c.Next(ctx)

		status := c.Response.StatusCode()
		fields := []zap.Field{
			zap.String("method", string(c.Method())),
			zap.String("path", string(c.Path())),
			zap.Int("status", status),
			zap.Int("bytes", len(c.Response.BodyBytes())),
			zap.Duration("duration", time.Since(start)),
			zap.String("request_id", requestid.Get(c)),
		}
		switch {
		case status >= 500:
			logger.Error("http request", fields...)
		case status >= 400:
			logger.Warn("http request", fields...)
		default:
			logger.Info("http request", fields...)
		}
	}
}
