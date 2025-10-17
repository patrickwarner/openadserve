package api

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/patrickwarner/openadserve/internal/analytics"
	"github.com/patrickwarner/openadserve/internal/logic/selectors"
	"github.com/patrickwarner/openadserve/internal/observability"
	"github.com/patrickwarner/openadserve/internal/token"

	"go.uber.org/zap"
)

func newTestServer() *Server {
	return &Server{
		Logger:      zap.NewNop(),
		Analytics:   &analytics.Analytics{DB: &sql.DB{}, Metrics: observability.NewNoOpRegistry()},
		SelectorMap: map[int]selectors.Selector{0: selectors.NewRuleBasedSelector()},
		TokenSecret: []byte("secret"),
		TokenTTL:    time.Millisecond,
		Metrics:     observability.NewNoOpRegistry(),
	}
}

func TestImpressionHandler_InvalidToken(t *testing.T) {
	srv := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/impression", nil)
	rec := httptest.NewRecorder()

	srv.ImpressionHandler(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestImpressionHandler_ExpiredToken(t *testing.T) {
	srv := newTestServer()
	tok, _ := token.Generate("r", "1", "1", "1", "1", "user123", "1", srv.TokenSecret)
	time.Sleep(10 * time.Millisecond)
	req := httptest.NewRequest(http.MethodGet, "/impression?t="+tok, nil)
	rec := httptest.NewRecorder()

	srv.ImpressionHandler(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
