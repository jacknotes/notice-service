package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// 全局指标注册表与指标。所有采集器只在本包注册一次（init）。
var (
	Registry = prometheus.NewRegistry()

	SendsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "notice_sends_total",
		Help: "通知发送次数，按渠道与状态（success/failed）",
	}, []string{"channel", "status"})

	SendDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "notice_send_duration_seconds",
		Help:    "单次发送耗时（秒）",
		Buckets: prometheus.DefBuckets,
	}, []string{"channel"})

	HTTPRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "HTTP 请求数，按 code/method/path",
	}, []string{"code", "method", "path"})
)

// QueuePendingFunc 由 main 注入：返回待处理队列中的 pending job 数（scrape 时调用）。
var QueuePendingFunc func() float64

func init() {
	Registry.MustRegister(SendsTotal, SendDuration, HTTPRequests)
	Registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	Registry.MustRegister(prometheus.NewGoCollector())
	Registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "notice_queue_pending",
		Help: "待处理发送队列中 pending 的 job 数",
	}, func() float64 {
		if QueuePendingFunc == nil {
			return 0
		}
		return QueuePendingFunc()
	}))
}

// Handler 返回 /metrics 的 HTTP handler（含 Go runtime/process 默认采集）。
func Handler() http.Handler {
	return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{})
}
