// Command mindbridge 启动 RAG 服务:加载配置 → 连 Postgres/Qdrant → 装配 → 启动 HTTP。
package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/JexKnox/mindbridge/internal/adapter"
	"github.com/JexKnox/mindbridge/internal/config"
	"github.com/JexKnox/mindbridge/internal/doc"
	"github.com/JexKnox/mindbridge/internal/kb"
	"github.com/JexKnox/mindbridge/internal/logging"
	"github.com/JexKnox/mindbridge/internal/repo"
	"github.com/JexKnox/mindbridge/internal/server"
	"go.uber.org/zap"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "config file path")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err) // logger 尚未构建,兜底用标准 log
	}

	// Hertz 框架日志经 hlog 走官方 zap 后端,与业务日志同源。
	logging.SetHlog(cfg.Logging)

	logger, err := logging.New(cfg.Logging)
	if err != nil {
		log.Fatalf("build logger: %v", err)
	}
	defer logger.Sync()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	db, err := repo.Open(ctx, cfg.Postgres.DSN)
	cancel()
	if err != nil {
		logger.Fatal("open postgres", zap.Error(err))
	}
	defer db.Close()

	migCtx, migCancel := context.WithTimeout(context.Background(), 15*time.Second)
	if err := repo.Migrate(migCtx, db); err != nil {
		migCancel()
		logger.Fatal("migrate", zap.Error(err))
	}
	migCancel()

	qdrant := adapter.NewQdrantClient(cfg.Qdrant.Host, cfg.Qdrant.Port, cfg.Qdrant.APIKey)

	kbRepo := kb.NewRepo(db)
	kbMgr := kb.NewManager(kbRepo, qdrant, cfg.Models.Embedding)

	pipeline := doc.NewPipeline(doc.SplitterConfig{
		ChunkSize:    cfg.Ingest.ChunkSize,
		ChunkOverlap: cfg.Ingest.ChunkOverlap,
		Separators:   cfg.Ingest.Separators,
	})

	h := server.New(server.Deps{
		Cfg:      cfg,
		Logger:   logger,
		Qdrant:   qdrant,
		KB:       kbMgr,
		Pipeline: pipeline,
	})

	logger.Info("mindbridge listening", zap.String("addr", cfg.Server.Addr))
	// h.Spin() 内部处理 SIGINT/SIGTERM 优雅停机,无需手写信号监听。
	h.Spin()
}
