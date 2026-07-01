// Package server 是 HTTP 接口层:chi handler 只做参数解析与响应组装,
// 业务逻辑沉到 internal/ 下各包。知识库是头等概念,文档/对话接口都带 kb_id。
package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"github.com/JexKnox/mindbridge/internal/adapter"
	"github.com/JexKnox/mindbridge/internal/config"
	"github.com/JexKnox/mindbridge/internal/doc"
	"github.com/JexKnox/mindbridge/internal/kb"
	"github.com/JexKnox/mindbridge/internal/logging"
)

// Deps 是 handler 的依赖集合,启动时在 main 装配。
type Deps struct {
	Cfg      *config.Config
	Logger   *zap.Logger
	Qdrant   *adapter.QdrantClient
	KB       *kb.Manager
	Pipeline *doc.Pipeline
}

func New(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(logging.ZapLogger(d.Logger)) // 替代 middleware.Logger,统一走 zap
	r.Use(middleware.Recoverer)

	h := &handlers{deps: d}

	r.Route("/api", func(r chi.Router) {
		r.Get("/kb", h.listKB)
		r.Post("/kb", h.createKB)
		r.Post("/kb/{id}/documents", h.uploadDocument)
		r.Post("/kb/{id}/chat", h.chat)
	})

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// 前端静态托管:若产物目录存在,则服务其静态文件;
	// 非文件路径(且不属于 /api、/healthz)回退到 index.html,支持前端路由刷新/深链。
	mountSPA(r, d.Cfg.Server.StaticDir)

	return r
}

// mountSPA 注册前端静态文件服务。staticDir 不存在时静默跳过(纯后端开发场景)。
func mountSPA(r chi.Router, staticDir string) {
	if staticDir == "" {
		return
	}
	if info, err := os.Stat(staticDir); err != nil || !info.IsDir() {
		return
	}
	abs, err := filepath.Abs(staticDir)
	if err != nil {
		abs = staticDir
	}
	fs := http.FileServer(http.Dir(abs))

	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		// 拦截 API 路径,避免误返回 index.html 掩盖 404。
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" {
			http.NotFound(w, r)
			return
		}
		// 文件不存在则回退 index.html(SPA 路由)。
		full := filepath.Join(abs, filepath.Clean(r.URL.Path))
		if _, err := os.Stat(full); os.IsNotExist(err) {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fs.ServeHTTP(w, r2)
			return
		}
		fs.ServeHTTP(w, r)
	})
}

type handlers struct {
	deps Deps
}
