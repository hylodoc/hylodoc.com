package metrics

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/knuthic/knuthic/internal/assert"
	"github.com/knuthic/knuthic/internal/model"
	"github.com/knuthic/knuthic/internal/session"
)

var (
	/* service metrics */
	httpRequest = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_request",
			Help: "hylodoc service request",
		},
		[]string{"method", "path", "status", "error_type"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "hylodoc service request duration",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status", "error_type"},
	)

	httpRequestSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_request_success",
			Help: "hylodoc service request success",
		},
		[]string{"method", "path", "status", "error_type"},
	)

	httpRequestErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_request_error",
			Help: "hylodoc service request error",
		},
		[]string{"method", "path", "status", "error_type"},
	)

	/* downstream metrics */
	httpClientRequest = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_client_request",
			Help: "downstream request",
		},
		[]string{"method", "url"},
	)

	httpClientSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_client_success",
			Help: "downstream request success",
		},
		[]string{"method", "url", "status"},
	)

	httpClientErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_client_error",
			Help: "downstream request error"},
		[]string{"method", "url", "status"},
	)

	httpClientDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_client_duration_seconds",
			Help:    "downstream request duration",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "url", "status"},
	)

	/* custom email error */
	emailBatchRequest = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "email_batch_request",
			Help: "email batch request",
		},
		[]string{"email_type"},
	)
	emailBatchSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "email_batch_success",
			Help: "email batch success",
		},
		[]string{"email_type"},
	)
	emailBatchError = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "email_batch_error",
			Help: "email batch error",
		},
		[]string{"email_type"},
	)
	emailInBatchSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "email_in_batch_success",
			Help: "email in batch success",
		},
		[]string{"postmark_stream"},
	)
	emailInBatchError = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "email_in_batch_error",
			Help: "email in batch error",
		},
		[]string{"postmark_stream"},
	)
)

func Initialize() {
	prometheus.MustRegister(httpRequest)
	prometheus.MustRegister(httpRequestSuccess)
	prometheus.MustRegister(httpRequestErrors)
	prometheus.MustRegister(httpRequestDuration)

	prometheus.MustRegister(httpClientRequest)
	prometheus.MustRegister(httpClientSuccess)
	prometheus.MustRegister(httpClientErrors)
	prometheus.MustRegister(httpClientDuration)

	prometheus.MustRegister(emailBatchRequest)
	prometheus.MustRegister(emailBatchSuccess)
	prometheus.MustRegister(emailBatchError)
	prometheus.MustRegister(emailInBatchSuccess)
	prometheus.MustRegister(emailInBatchError)
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

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		/*
		 * XXX: Do not introduce errors into this middleware until it is
		 * absorbed into something consistent with our handler package,
		 * because doing so would lead to those errors showing to the
		 * user.
		 */

		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		assert.Assert(ok)

		start := time.Now()

		/* custom responseWriter to capture code */
		rec := &responseWriterWithStatus{
			ResponseWriter: w,
			statusCode:     0,
		}

		next.ServeHTTP(rec, r)
		duration := time.Since(start).Seconds()

		httpRequest.WithLabelValues(
			r.Method,
			r.URL.Path,
			http.StatusText(rec.statusCode),
			"none",
		).Inc()

		/* handle errors */
		if rec.statusCode >= 200 && rec.statusCode < 400 {
			httpRequestSuccess.WithLabelValues(
				r.Method,
				r.URL.Path,
				http.StatusText(rec.statusCode),
				"none",
			).Inc()
		} else {
			errorType := classifyError(rec.statusCode, sesh)
			httpRequestErrors.WithLabelValues(
				r.Method,
				r.URL.Path,
				http.StatusText(rec.statusCode),
				errorType,
			).Inc()
		}

		httpRequestDuration.WithLabelValues(
			r.Method,
			r.URL.Path,
			http.StatusText(rec.statusCode),
			"none",
		).Observe(duration)
	})
}

func classifyError(statusCode int, sesh *session.Session) string {
	switch {
	case statusCode == http.StatusNotFound:
		return "not_found"
	case statusCode == http.StatusUnauthorized:
		return "unauthorized"
	case statusCode >= 500:
		return "internal"
	default:
		sesh.Printf("unknown error type: %d", statusCode)
		return "unknown"
	}
}

func Handler() http.Handler {
	return promhttp.Handler()
}

func RecordClientRequest(method, url string) {
	httpClientRequest.WithLabelValues(method, url).Inc()
}

func RecordClientSuccess(method, url, status string) {
	httpClientSuccess.WithLabelValues(method, url, status).Inc()
}

func RecordClientErrors(method, url, status string) {
	httpClientErrors.WithLabelValues(method, url, status).Inc()
}

func RecordClientDuration(method, url string, duration float64, status string) {
	httpClientDuration.WithLabelValues(method, url, status).Observe(duration)
}

func RecordEmailBatchRequest() {
	emailBatchSuccess.WithLabelValues("batch").Inc()
}

func RecordEmailBatchSuccess() {
	emailBatchSuccess.WithLabelValues("batch").Inc()
}

func RecordEmailBatchError() {
	emailBatchError.WithLabelValues("batch").Inc()
}

func RecordEmailInBatchSuccess(stream model.PostmarkStream) {
	emailInBatchSuccess.WithLabelValues(fmt.Sprintf("%s", stream)).Inc()
}

func RecordEmailInBatchError(stream model.PostmarkStream) {
	emailInBatchError.WithLabelValues(fmt.Sprintf("%s", stream)).Inc()
}
