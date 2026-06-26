package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	DownloadsEnqueued = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "saveinator_downloads_enqueued_total", Help: "Downloads enqueued"},
		[]string{"platform"},
	)
	RateLimitDropped = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "saveinator_rate_limit_dropped_total", Help: "Rate limited requests"},
		[]string{"scope"},
	)
)

func init() {
	prometheus.MustRegister(DownloadsEnqueued, RateLimitDropped)
}

func Handler() http.Handler {
	return promhttp.Handler()
}
