package middleware

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests",
	})

	httpRequestDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	})

	httpRequestSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "http_request_size_bytes",
		Help:    "HTTP request size in bytes",
		Buckets: []float64{100, 1000, 10000, 100000, 1000000},
	})

	httpResponseSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "http_response_size_bytes",
		Help:    "HTTP response size in bytes",
		Buckets: []float64{100, 1000, 10000, 100000, 1000000},
	})
)

func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// track request size
		if r.ContentLength >= 0 {
			httpRequestSize.Observe(float64(r.ContentLength))
		}

		// create custom response writer to track response size
		crw := &countingResponseWriter{ResponseWriter: w}

		// call next handler
		next.ServeHTTP(crw, r)

		// track metrics
		httpRequestsTotal.Inc()
		httpResponseSize.Observe(float64(crw.size))
		httpRequestDuration.Observe(time.Since(start).Seconds())
	})
}

type countingResponseWriter struct {
	http.ResponseWriter
	size int
}

func (crw *countingResponseWriter) Write(b []byte) (int, error) {
	size, err := crw.ResponseWriter.Write(b)
	crw.size += size
	return size, err
}
