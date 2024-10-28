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
	httpClientRequestTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_client_request_total",
			Help: "Total number of client requests",
		},
		[]string{"method", "url"},
	)

	httpClientSuccessTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_client_success_total",
			Help: "Total number of successful http client requests",
		},
		[]string{"method", "url", "status"},
	)

	httpClientErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_client_errors_total",
			Help: "Total number of http client errors"},
		[]string{"method", "url", "status"},
	)

	httpClientDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_client_duration_seconds",
			Help:    "Duration of http client calls",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "url", "status"},
	)
)

func Initialize() {
	prometheus.MustRegister(httpRequestTotal)
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(httpRequestSuccessTotal)
	prometheus.MustRegister(httpRequestErrorsTotal)

	prometheus.MustRegister(httpClientRequestTotal)
	prometheus.MustRegister(httpClientSuccessTotal)
	prometheus.MustRegister(httpClientErrorsTotal)
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

func RecordClientRequest(method, url string) {
	httpClientRequestTotal.WithLabelValues(method, url).Inc()
}

func RecordClientSuccess(method, url, status string) {
	httpClientSuccessTotal.WithLabelValues(method, url, status).Inc()
}

func RecordClientErrors(method, url, status string) {
	httpClientErrorsTotal.WithLabelValues(method, url, status).Inc()
}

func RecordClientDuration(method, url string, duration float64, status string) {
	httpClientDuration.WithLabelValues(method, url, status).Observe(duration)
}
