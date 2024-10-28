package metrics

import (
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	/* service metrics */
	httpRequestTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of http requests",
		},
		[]string{"method", "path", "status", "error_type"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of http requests",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status", "error_type"},
	)

	httpRequestSuccessTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_request_success_total",
			Help: "Total number of successful http requests",
		},
		[]string{"method", "path", "status", "error_type"},
	)

	httpRequestErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_request_errors_total",
			Help: "Total number of failed http requests",
		},
		[]string{"method", "path", "status", "error_type"},
	)

	/* downstream metrics */
	httpClientErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_client_errors_total",
			Help: "Total number of http client errors"},
		[]string{"method", "url"},
	)

	httpClientDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_client_duration_seconds",
			Help:    "Duration of http client calls",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "url"},
	)
)

func Initialize() {
	prometheus.MustRegister(httpRequestTotal)
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(httpRequestSuccessTotal)
	prometheus.MustRegister(httpRequestErrorsTotal)
	prometheus.MustRegister(httpClientErrors)
	prometheus.MustRegister(httpClientDuration)
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

		httpRequestTotal.WithLabelValues(r.Method, r.URL.Path, http.StatusText(rec.statusCode), "none").Inc()

		/* handle errors */
		if rec.statusCode >= 200 && rec.statusCode < 400 {
			httpRequestSuccessTotal.WithLabelValues(r.Method, r.URL.Path, http.StatusText(rec.statusCode), "none").Inc()
		} else {
			errorType := classifyError(rec.statusCode)
			httpRequestErrorsTotal.WithLabelValues(r.Method, r.URL.Path, http.StatusText(rec.statusCode), errorType).Inc()
		}

		httpRequestDuration.WithLabelValues(r.Method, r.URL.Path, http.StatusText(rec.statusCode), "none").Observe(duration)
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

func RecordClientErrors(method, url string) {
	httpClientErrors.WithLabelValues(method, url).Inc()
}

func RecordClientDuration(method, url string, duration float64) {
	httpClientDuration.WithLabelValues(method, url).Observe(duration)
}
