// Package metrics 定义 mindbridge 的 Prometheus 指标,按三层组织:
//
//   - 系统层:HTTP 请求量/错误/延迟分位、Qdrant 查询延迟、外部模型 API 延迟与错误(可用性);
//     Go 运行时指标(内存/GC/goroutine)由 client_golang 默认收集器提供。
//   - 检索层:各通道(向量/关键词)候选数、空结果率、top1 分数分布、rerank 前后 top1 变化、缓存命中率。
//   - 生成层:首 token 延迟(TTFT)、完整生成时长、输出字符量、拒答率、引用数、流错误。
//
// 指标经独立端口的 /metrics 暴露(与业务端口分离,不受 SSE 长连接影响),由 Prometheus 抓取。
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

const ns = "mindbridge"

// ── 系统层 ──────────────────────────────────────────────────────────────

var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: ns, Name: "http_requests_total",
		Help: "HTTP 请求总数(含状态码,用于算错误率)",
	}, []string{"method", "path", "status"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: ns, Name: "http_request_duration_seconds",
		Help:    "HTTP 请求耗时(P50/P95/P99 由直方图分位计算;流式接口为完整响应时长)",
		Buckets: []float64{.01, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60},
	}, []string{"method", "path"})

	qdrantDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: ns, Name: "qdrant_request_duration_seconds",
		Help:    "Qdrant 各操作耗时(向量库查询延迟)",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
	}, []string{"op"})

	qdrantErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: ns, Name: "qdrant_request_errors_total",
		Help: "Qdrant 各操作失败数",
	}, []string{"op"})

	modelDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: ns, Name: "model_request_duration_seconds",
		Help:    "外部模型 API 耗时(kind=embedding/rerank/rewrite)",
		Buckets: []float64{.05, .1, .25, .5, 1, 2.5, 5, 10, 30},
	}, []string{"kind"})

	modelErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: ns, Name: "model_request_errors_total",
		Help: "外部模型 API 失败数(错误率上升 = 可用性下降)",
	}, []string{"kind"})
)

// ── 检索层 ──────────────────────────────────────────────────────────────

var (
	retrievalCandidates = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: ns, Name: "retrieval_candidates",
		Help:    "单次检索各通道候选数(channel=dense/sparse/fused)",
		Buckets: []float64{0, 1, 2, 5, 10, 20, 50, 100},
	}, []string{"channel"})

	retrievalRequests = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns, Name: "retrieval_requests_total",
		Help: "检索请求总数",
	})

	retrievalEmpty = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns, Name: "retrieval_empty_total",
		Help: "空结果检索数(空结果率突升可能是索引/过滤问题,不一定是模型问题)",
	})

	retrievalTop1Score = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: ns, Name: "retrieval_top1_score",
		Help:    "top1 余弦分分布(分布左移 = 检索质量劣化)",
		Buckets: prometheus.LinearBuckets(0, 0.1, 11),
	})

	rerankRequests = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns, Name: "rerank_requests_total",
		Help: "rerank 执行次数",
	})

	rerankTop1Changed = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns, Name: "rerank_top1_changed_total",
		Help: "rerank 改变 top1 的次数(占比过高说明召回排序差,过低说明 rerank 冗余)",
	})

	cacheHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: ns, Name: "cache_hits_total",
		Help: "缓存命中数(cache=embedding/rewrite/retrieval)",
	}, []string{"cache"})

	cacheMisses = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: ns, Name: "cache_misses_total",
		Help: "缓存未命中数",
	}, []string{"cache"})
)

// ── 生成层 ──────────────────────────────────────────────────────────────

var (
	answers = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns, Name: "chat_answers_total",
		Help: "RAG 回答总数",
	})

	refusals = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns, Name: "chat_refusals_total",
		Help: "拒答数(top1 分数低于阈值走拒答分支;拒答率 = refusals/answers)",
	})

	firstTokenSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: ns, Name: "chat_first_token_seconds",
		Help:    "首 token 延迟 TTFT(从发起生成到收到第一个 token)",
		Buckets: []float64{.1, .25, .5, 1, 2, 3, 5, 10, 20},
	})

	generationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: ns, Name: "chat_generation_seconds",
		Help:    "完整生成时长(流开始到流结束)",
		Buckets: []float64{.5, 1, 2.5, 5, 10, 20, 30, 60, 120},
	})

	outputChars = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: ns, Name: "chat_output_chars",
		Help:    "单次回答输出字符数(token 数的近似替代,provider 流式响应不带 usage)",
		Buckets: prometheus.ExponentialBuckets(16, 2, 10),
	})

	citations = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: ns, Name: "chat_citations",
		Help:    "单次回答注入的引用条数",
		Buckets: prometheus.LinearBuckets(0, 1, 11),
	})

	streamErrors = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns, Name: "chat_stream_errors_total",
		Help: "生成流中断/失败数",
	})
)

// ── 埋点辅助(供各层调用,避免业务代码直接摸 prometheus 类型) ─────────

// ObserveQdrant 记录一次 Qdrant 操作的耗时与错误。用法:defer metrics.ObserveQdrant(op, time.Now())(err)。
func ObserveQdrant(op string, start time.Time) func(err error) {
	return func(err error) {
		qdrantDuration.WithLabelValues(op).Observe(time.Since(start).Seconds())
		if err != nil {
			qdrantErrors.WithLabelValues(op).Inc()
		}
	}
}

// ObserveModel 记录一次外部模型 API 调用(kind=embedding/rerank/rewrite)。
func ObserveModel(kind string, start time.Time, err error) {
	modelDuration.WithLabelValues(kind).Observe(time.Since(start).Seconds())
	if err != nil {
		modelErrors.WithLabelValues(kind).Inc()
	}
}

// ObserveCandidates 记录某检索通道的候选数。
func ObserveCandidates(channel string, n int) {
	retrievalCandidates.WithLabelValues(channel).Observe(float64(n))
}

// ObserveRetrieval 记录一次检索的结果规模与 top1 分数。
func ObserveRetrieval(resultCount int, top1Score float64) {
	retrievalRequests.Inc()
	if resultCount == 0 {
		retrievalEmpty.Inc()
		return
	}
	retrievalTop1Score.Observe(top1Score)
}

// ObserveRerank 记录一次 rerank 及其是否改变了 top1。
func ObserveRerank(top1Changed bool) {
	rerankRequests.Inc()
	if top1Changed {
		rerankTop1Changed.Inc()
	}
}

// Cache 记录一次缓存查询(cache=embedding/rewrite/retrieval)。
func Cache(name string, hit bool) {
	if hit {
		cacheHits.WithLabelValues(name).Inc()
		return
	}
	cacheMisses.WithLabelValues(name).Inc()
}

// ObserveAnswer 记录一次 RAG 回答的生成侧指标。ttft<=0 表示没收到任何 token。
func ObserveAnswer(refused bool, citationCount int, ttft, total time.Duration, chars int, streamErr error) {
	answers.Inc()
	if refused {
		refusals.Inc()
	}
	citations.Observe(float64(citationCount))
	if ttft > 0 {
		firstTokenSeconds.Observe(ttft.Seconds())
	}
	generationSeconds.Observe(total.Seconds())
	outputChars.Observe(float64(chars))
	if streamErr != nil {
		streamErrors.Inc()
	}
}

// ── HTTP 中间件与 /metrics 服务 ─────────────────────────────────────────

// RecordHTTP 记录一次 HTTP 请求(由 server 的中间件调用,path 用注册路由避免高基数)。
func RecordHTTP(method, path string, status int, dur time.Duration) {
	s := strconv.Itoa(status)
	httpRequests.WithLabelValues(method, path, s).Inc()
	httpDuration.WithLabelValues(method, path).Observe(dur.Seconds())
}

// Serve 在独立端口起 /metrics(addr 为空则不启用)。与业务端口分离:
// SSE 长连接不影响抓取,也避免把内部指标暴露到业务入口。
func Serve(addr string, logger *zap.Logger) {
	if addr == "" {
		return
	}
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		logger.Info("metrics listening", zap.String("addr", addr))
		if err := http.ListenAndServe(addr, mux); err != nil {
			logger.Warn("metrics server exited", zap.Error(err))
		}
	}()
}
