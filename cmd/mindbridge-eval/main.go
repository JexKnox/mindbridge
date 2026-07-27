// Command mindbridge-eval 对指定知识库跑检索评估:读取 JSONL 评估集,
// 输出 recall@k / MRR / hit@k 检索指标。换模型/调参前后各跑一遍,用数字说话。
//
// 评估集格式(每行一个 JSON 对象):
//
//	{"question": "如何部署?", "relevant_files": ["deploy.md", "docker.md"]}
//
// relevant_files 为该问题的标准答案所在源文件名(与上传文件名一致)。
//
// 用法:
//
//	go run ./cmd/mindbridge-eval -config configs/config.yaml -kb <kb_id> -eval evalset.jsonl -k 5
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/JexKnox/mindbridge/internal/adapter"
	"github.com/JexKnox/mindbridge/internal/config"
	"github.com/JexKnox/mindbridge/internal/kb"
	"github.com/JexKnox/mindbridge/internal/repo"
)

type evalCase struct {
	Question      string   `json:"question"`
	RelevantFiles []string `json:"relevant_files"`
}

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "config file path")
	kbID := flag.String("kb", "", "knowledge base id (required)")
	evalPath := flag.String("eval", "", "eval set path, JSONL (required)")
	topK := flag.Int("k", 5, "evaluate metrics at top-k")
	flag.Parse()
	if *kbID == "" || *evalPath == "" {
		flag.Usage()
		os.Exit(2)
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	cases, err := loadCases(*evalPath)
	if err != nil {
		log.Fatalf("load eval set: %v", err)
	}
	if len(cases) == 0 {
		log.Fatal("eval set is empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	db, err := repo.Open(ctx, cfg.Postgres.DSN)
	cancel()
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}
	defer db.Close()

	k, err := kb.NewRepo(db).Get(context.Background(), *kbID)
	if err != nil {
		log.Fatalf("get kb %s: %v", *kbID, err)
	}

	embCfg := cfg.Models.Embedding
	embCfg.Model = k.EmbeddingModel
	emb, err := adapter.NewEmbedder(context.Background(), embCfg)
	if err != nil {
		log.Fatalf("build embedder: %v", err)
	}
	qdrant := adapter.NewQdrantClient(cfg.Qdrant.Host, cfg.Qdrant.Port, cfg.Qdrant.APIKey)
	ret := adapter.NewRetriever(qdrant, emb, k.Collection, *topK, cfg.RAG.HybridEnabled())

	fmt.Printf("kb=%s (%s)  cases=%d  k=%d  hybrid=%v\n\n",
		k.Name, k.ID, len(cases), *topK, cfg.RAG.HybridEnabled())

	var sumRecall, sumMRR float64
	hits := 0
	for i, c := range cases {
		docs, err := ret.Retrieve(context.Background(), c.Question)
		if err != nil {
			log.Fatalf("case %d retrieve: %v", i+1, err)
		}

		relevant := map[string]bool{}
		for _, f := range c.RelevantFiles {
			relevant[f] = true
		}
		foundFiles := map[string]bool{}
		firstHitRank := 0
		for rank, d := range docs {
			src, _ := d.MetaData["source_file"].(string)
			if relevant[src] {
				foundFiles[src] = true
				if firstHitRank == 0 {
					firstHitRank = rank + 1
				}
			}
		}

		recall := float64(len(foundFiles)) / float64(len(relevant))
		mrr := 0.0
		if firstHitRank > 0 {
			mrr = 1.0 / float64(firstHitRank)
			hits++
		}
		sumRecall += recall
		sumMRR += mrr
		fmt.Printf("[%2d] recall@%d=%.2f  rr=%.2f  %s\n", i+1, *topK, recall, mrr, truncate(c.Question, 40))
	}

	n := float64(len(cases))
	fmt.Printf("\n===== 汇总 =====\n")
	fmt.Printf("recall@%d = %.4f\n", *topK, sumRecall/n)
	fmt.Printf("MRR      = %.4f\n", sumMRR/n)
	fmt.Printf("hit@%d    = %.4f (%d/%d)\n", *topK, float64(hits)/n, hits, len(cases))
}

func loadCases(path string) ([]evalCase, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cases []evalCase
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var c evalCase
		if err := json.Unmarshal(line, &c); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if c.Question == "" || len(c.RelevantFiles) == 0 {
			return nil, fmt.Errorf("line %d: question and relevant_files are required", lineNo)
		}
		cases = append(cases, c)
	}
	return cases, sc.Err()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
