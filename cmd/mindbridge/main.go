// Command mindbridge 启动 RAG 服务:加载配置 → 连 Postgres/Qdrant → 装配 → 启动 HTTP。
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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

	handler := server.New(server.Deps{
		Cfg:      cfg,
		Logger:   logger,
		Qdrant:   qdrant,
		KB:       kbMgr,
		Pipeline: pipeline,
	})

	srv := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("mindbridge listening", zap.String("addr", cfg.Server.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("listen", zap.Error(err))
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	logger.Info("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", zap.Error(err))
	}
}
