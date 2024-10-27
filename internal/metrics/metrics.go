package metrics

import (
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	/* Service metrics */
	HTTPRequestTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status", "error_type"},
	)

	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status", "error_type"},
	)

	HTTPRequestSuccessTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_request_success_total",
			Help: "Total number of successful HTTP requests",
		},
		[]string{"method", "path", "status", "error_type"},
	)

	HTTPRequestErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_request_errors_total",
			Help: "Total number of failed HTTP requests",
		},
		[]string{"method", "path", "status", "error_type"},
	)

	/* Downstream metrics */
	HTTPClientErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_client_errors_total",
			Help: "Total number of HTTP client errors"},
		[]string{"method", "url"},
	)

	HTTPClientDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_client_duration_seconds",
			Help:    "Duration of HTTP client calls",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "url"},
	)
)

func Initialize() {
	prometheus.MustRegister(HTTPRequestTotal)
	prometheus.MustRegister(HTTPRequestDuration)
	prometheus.MustRegister(HTTPRequestSuccessTotal)
	prometheus.MustRegister(HTTPRequestErrorsTotal)
	prometheus.MustRegister(HTTPClientErrors)
	prometheus.MustRegister(HTTPClientDuration)
}

type responseWriterWithStatus struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriterWithStatus) WriteHeader(code int) {
	if rw.statusCode != 0 {
		return
	}
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriterWithStatus) Write(b []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	return rw.ResponseWriter.Write(b)
}

func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println("metrics middleware...")

		start := time.Now()

		/* custom responseWriter to capture code */
		rec := &responseWriterWithStatus{ResponseWriter: w, statusCode: 0}

		next.ServeHTTP(rec, r)
		duration := time.Since(start).Seconds()

		HTTPRequestTotal.WithLabelValues(r.Method, r.URL.Path, http.StatusText(rec.statusCode), "none").Inc()

		/* handle errors */
		if rec.statusCode >= 200 && rec.statusCode < 400 {
			HTTPRequestSuccessTotal.WithLabelValues(r.Method, r.URL.Path, http.StatusText(rec.statusCode), "none").Inc()
		} else {
			errorType := classifyError(rec.statusCode)
			HTTPRequestErrorsTotal.WithLabelValues(r.Method, r.URL.Path, http.StatusText(rec.statusCode), errorType).Inc()
		}

		HTTPRequestDuration.WithLabelValues(r.Method, r.URL.Path, http.StatusText(rec.statusCode), "none").Observe(duration)
	})
}

func classifyError(statusCode int) string {
	switch {
	case statusCode == http.StatusNotFound:
		return "not_found"
	case statusCode == http.StatusUnauthorized:
		return "unauthorized"
	case statusCode >= 500:
		return "internal"
	default:
		log.Printf("unknown error type: %d", statusCode)
		return "unknown"
	}
}

func Handler() http.Handler {
	return promhttp.Handler()
}
