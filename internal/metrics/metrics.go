// Package metrics exposes Prometheus counters/histograms and a /metrics handler.
package metrics

import (
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	gatewayDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "dispatch_gateway_request_duration_seconds",
		Help:    "Gateway /mail/send latency by result.",
		Buckets: durationBuckets,
	}, []string{"result"})
	gatewayRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dispatch_gateway_requests_total",
		Help: "Gateway /mail/send outcomes.",
	}, []string{"result"})

	workerDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "dispatch_worker_process_duration_seconds",
		Help:    "Worker Handle latency by result.",
		Buckets: durationBuckets,
	}, []string{"result"})
	workerProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dispatch_worker_processed_total",
		Help: "Worker message outcomes.",
	}, []string{"result"})
	workerQueuePending = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dispatch_worker_queue_pending",
		Help: "JetStream NumPending on the mail-worker consumer (messages waiting).",
	})

	graphDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "dispatch_graph_request_duration_seconds",
		Help:    "MS Graph HTTP call latency by result.",
		Buckets: durationBuckets,
	}, []string{"result"})
	graphRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dispatch_graph_requests_total",
		Help: "MS Graph HTTP call outcomes.",
	}, []string{"result"})

	durationBuckets = []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}

	handlerOnce sync.Once
	handler     http.Handler
)

// ObserveGateway records one gateway send attempt.
func ObserveGateway(result string, d time.Duration) {
	gatewayRequests.WithLabelValues(result).Inc()
	gatewayDuration.WithLabelValues(result).Observe(d.Seconds())
}

// ObserveWorker records one worker Handle attempt.
func ObserveWorker(result string, d time.Duration) {
	workerProcessed.WithLabelValues(result).Inc()
	workerDuration.WithLabelValues(result).Observe(d.Seconds())
}

// ObserveGraph records one MS Graph HTTP attempt (including retries).
func ObserveGraph(result string, d time.Duration) {
	graphRequests.WithLabelValues(result).Inc()
	graphDuration.WithLabelValues(result).Observe(d.Seconds())
}

// SetWorkerQueuePending sets the JetStream pending-message gauge.
func SetWorkerQueuePending(n uint64) {
	workerQueuePending.Set(float64(n))
}

// Handler returns a Prometheus scrape handler (process + Go collectors, once).
func Handler() http.Handler {
	handlerOnce.Do(func() {
		_ = prometheus.Register(collectors.NewGoCollector())
		_ = prometheus.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
		handler = promhttp.Handler()
	})
	return handler
}

// Mux serves /metrics and a liveness /health for worker and bounce processes.
func Mux() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux
}
