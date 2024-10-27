package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

/* holds testing metrics registry */
var testRegistry = prometheus.NewRegistry()

func resetTestMetrics() {
	testRegistry.Unregister(HTTPRequestTotal)
	testRegistry.Unregister(HTTPRequestDuration)

	testRegistry.MustRegister(HTTPRequestTotal)
	testRegistry.MustRegister(HTTPRequestDuration)
}

func TestMetricsMiddlewareSuccess(t *testing.T) {
	/* register metrics */
	resetTestMetrics()

	/* metrics handler */
	successHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	/* wrap handler in middleware */
	middleware := MetricsMiddleware(successHandler)

	/* test server */
	server := httptest.NewServer(middleware)
	defer server.Close()

	/* simulate requests */
	for i := 0; i < 3; i++ {
		_, err := http.Get(server.URL)
		if err != nil {
			t.Fatalf("Could not send request: %s", err)
		}
	}

	/* test that middleware recorded three successful calls */
	totalCount := testutil.ToFloat64(HTTPRequestTotal.WithLabelValues("GET", "/", "200", "none"))
	if totalCount != 3 {
		t.Errorf("Expected total count to be 3, got %f", totalCount)
	}

	/* recorded three successful calls */
	totalSuccessCount := testutil.ToFloat64(HTTPRequestSuccessTotal.WithLabelValues("GET", "/", "200", "none"))
	if totalSuccessCount != 3 {
		t.Errorf("Expected total error count to be 3, got %f", totalSuccessCount)
	}
}

func TestMetricsMiddlewareError(t *testing.T) {
	/* register metrics */
	resetTestMetrics()

	/* metrics handler */
	errorHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	/* wrap handler in middleware */
	middleware := MetricsMiddleware(errorHandler)

	/* test server */
	server := httptest.NewServer(middleware)
	defer server.Close()

	/* simulate requests */
	for i := 0; i < 4; i++ {
		_, err := http.Get(server.URL + "/path")
		if err != nil {
			t.Fatalf("Could not send request: %s", err)
		}
	}

	/* recorded four total calls */
	totalCount := testutil.ToFloat64(HTTPRequestTotal.WithLabelValues("GET", "/path", "500", "none"))
	if totalCount != 4 {
		t.Errorf("Expected total count to be 3, got %f", totalCount)
	}

	/* recorded four errors */
	totalErrorCount := testutil.ToFloat64(HTTPRequestErrorsTotal.WithLabelValues("GET", "/path", "500", "internal"))
	if totalErrorCount != 4 {
		t.Errorf("Expected total error count to be 4, got %f", totalErrorCount)
	}

	totalSuccessCount := testutil.ToFloat64(HTTPRequestSuccessTotal.WithLabelValues("GET", "/path", "200", "none"))
	if totalSuccessCount != 0 {
		t.Errorf("Expected no success, got %f", totalSuccessCount)
	}
}
