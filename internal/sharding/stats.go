// /internal/sharding/stats.go

package sharding

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	WriteLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "demo_write_latency_seconds",
		Help:    "Write latency seconds",
		Buckets: prometheus.DefBuckets,
	})
	ReadLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "demo_read_latency_seconds",
		Help:    "Read latency seconds",
		Buckets: prometheus.DefBuckets,
	})
	WriteCount = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "demo_write_total",
		Help: "Total writes",
	})
	ReadCount = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "demo_read_total",
		Help: "Total reads",
	})
)

// RegisterMetrics registers the metrics with Prometheus default registry.
// Call once during startup.
func RegisterMetrics() {
	prometheus.MustRegister(WriteLatency, ReadLatency, WriteCount, ReadCount)
}
