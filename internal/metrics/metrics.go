// Package metrics is what gitd tells Prometheus.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Metrics are gitd's metrics, in a registry of their own.
type Metrics struct {
	Registry   *prometheus.Registry
	Operations *prometheus.HistogramVec
	Failures   *prometheus.CounterVec
	PoolBytes  *prometheus.GaugeVec
}

// New registers the metrics; running reports how many git processes are alive.
func New(running func() float64) *Metrics {
	measured := &Metrics{
		Registry: prometheus.NewRegistry(),
		Operations: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "gitd_operation_seconds",
			Help:    "How long each operation took, by procedure and outcome.",
			Buckets: []float64{0.05, 0.25, 1, 5, 15, 60, 300, 900, 1800},
		}, []string{"procedure", "code"}),
		Failures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gitd_failures_total",
			Help: "Operations that failed, by reason.",
		}, []string{"reason"}),
		PoolBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gitd_pool_bytes",
			Help: "Disk each pool takes, as of the last maintenance.",
		}, []string{"repository"}),
	}
	measured.Registry.MustRegister(
		measured.Operations, measured.Failures, measured.PoolBytes,
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "gitd_git_processes",
			Help: "git processes alive right now.",
		}, running),
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return measured
}
