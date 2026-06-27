package metrics

import (
	"net/http"
	"strconv"
	"time"
)

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func HTTPMiddleware(route string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		status := strconv.Itoa(sw.status)
		HTTPRequestsTotal.WithLabelValues(r.Method, route, status).Inc()
		HTTPRequestLatencySeconds.WithLabelValues(r.Method, route).Observe(time.Since(started).Seconds())
	})
}
