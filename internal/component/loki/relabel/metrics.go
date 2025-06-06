package relabel

import (
	"github.com/grafana/alloy/internal/util"
	prometheus_client "github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	entriesProcessed prometheus_client.Counter
	entriesOutgoing  prometheus_client.Counter
	cacheHits        prometheus_client.Counter
	cacheMisses      prometheus_client.Counter
	cacheSize        prometheus_client.Gauge
}

// newMetrics creates a new set of metrics. If reg is non-nil, the metrics
// will also be registered.
func newMetrics(reg prometheus_client.Registerer) *metrics {
	var m metrics

	m.entriesProcessed = prometheus_client.NewCounter(prometheus_client.CounterOpts{
		Name: "loki_relabel_entries_processed",
		Help: "Total number of log entries processed",
	})
	m.entriesOutgoing = prometheus_client.NewCounter(prometheus_client.CounterOpts{
		Name: "loki_relabel_entries_written",
		Help: "Total number of log entries forwarded",
	})
	m.cacheMisses = prometheus_client.NewCounter(prometheus_client.CounterOpts{
		Name: "loki_relabel_cache_misses",
		Help: "Total number of cache misses",
	})
	m.cacheHits = prometheus_client.NewCounter(prometheus_client.CounterOpts{
		Name: "loki_relabel_cache_hits",
		Help: "Total number of cache hits",
	})
	m.cacheSize = prometheus_client.NewGauge(prometheus_client.GaugeOpts{
		Name: "loki_relabel_cache_size",
		Help: "Total size of relabel cache",
	})

	if reg != nil {
		m.entriesProcessed = util.MustRegisterOrGet(reg, m.entriesProcessed).(prometheus_client.Counter)
		m.entriesOutgoing = util.MustRegisterOrGet(reg, m.entriesOutgoing).(prometheus_client.Counter)
		m.cacheMisses = util.MustRegisterOrGet(reg, m.cacheMisses).(prometheus_client.Counter)
		m.cacheHits = util.MustRegisterOrGet(reg, m.cacheHits).(prometheus_client.Counter)
		m.cacheSize = util.MustRegisterOrGet(reg, m.cacheSize).(prometheus_client.Gauge)
	}

	return &m
}
