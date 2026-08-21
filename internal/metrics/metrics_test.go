package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandler_ExposesObservedMetrics(t *testing.T) {
	ObserveGateway("accepted", 12*time.Millisecond)
	ObserveWorker("delivered", 40*time.Millisecond)
	ObserveGraph("success", 8*time.Millisecond)
	SetWorkerQueuePending(3)

	rr := httptest.NewRecorder()
	Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rr.Code)
	}
	body, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	for _, needle := range []string{
		"dispatch_gateway_requests_total",
		"dispatch_worker_processed_total",
		"dispatch_graph_requests_total",
		"dispatch_worker_queue_pending",
	} {
		if !strings.Contains(got, needle) {
			t.Errorf("metrics body missing %q", needle)
		}
	}
}

func TestMux_HealthAndMetrics(t *testing.T) {
	mux := Mux()
	health := httptest.NewRecorder()
	mux.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health: want 200, got %d", health.Code)
	}
	met := httptest.NewRecorder()
	mux.ServeHTTP(met, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if met.Code != http.StatusOK {
		t.Fatalf("metrics: want 200, got %d", met.Code)
	}
}
