// Package logging 提供 zap 日志器的构建与 chi 请求日志中间件。
// 业务/适配层只依赖 *zap.Logger,不感知具体配置来源;切换日志实现时改动集中在此包。
package logging

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
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

// New 按 cfg 构建 zap.Logger。
func New(cfg Config) (*zap.Logger, error) {
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil || cfg.Level == "" {
		level = zapcore.InfoLevel
	}

	encoding := cfg.Encoding
	if encoding == "" {
		encoding = "console"
	}
	outputs := cfg.Outputs
	if len(outputs) == 0 {
		outputs = []string{"stdout"}
	}
	errOutputs := cfg.ErrorOutputs
	if len(errOutputs) == 0 {
		errOutputs = []string{"stderr"}
	}

	zcfg := zap.NewProductionConfig()
	zcfg.Level = zap.NewAtomicLevelAt(level)
	zcfg.Encoding = encoding
	zcfg.OutputPaths = outputs
	zcfg.ErrorOutputPaths = errOutputs
	zcfg.Sampling = nil // 不采样,避免开发态丢日志
	if encoding == "console" {
		// console 编码用可读时间格式,而非 epoch 毫秒
		zcfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}
	return zcfg.Build()
}

// ZapLogger 是基于 zap 的 chi 请求日志中间件,替代 middleware.Logger。
// 按响应状态码分级:5xx Error、4xx Warn、其余 Info。
func ZapLogger(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			fields := []zap.Field{
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", ww.Status()),
				zap.Int("bytes", ww.BytesWritten()),
				zap.Duration("duration", time.Since(start)),
				zap.String("request_id", middleware.GetReqID(r.Context())),
			}
			switch {
			case ww.Status() >= 500:
				logger.Error("http request", fields...)
			case ww.Status() >= 400:
				logger.Warn("http request", fields...)
			default:
				logger.Info("http request", fields...)
			}
		})
	}
}
