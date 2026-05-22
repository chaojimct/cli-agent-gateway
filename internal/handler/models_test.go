package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chaojimct/cli-agent-gateway/internal/config"
	"github.com/chaojimct/cli-agent-gateway/internal/cursor"
)

func TestModelsHandlerNotReady(t *testing.T) {
	cfg := config.Default()
	runner := cursor.NewRunner(cfg, 0, nil)
	h := NewModelsHandler(runner, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want 503", w.Code)
	}
}
