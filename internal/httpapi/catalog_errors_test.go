package httpapi

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"minecraft-hosting/internal/catalog"
)

func TestSearchFailureDoesNotSuggestPackageIncompatibility(t *testing.T) {
	a := &API{}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/servers/id/plugins/catalog", nil)
	r = r.WithContext(context.WithValue(r.Context(), requestKey, "test-reference"))
	a.fail(w, r, errors.Join(catalog.ErrSearchUnavailable, catalog.ErrUnavailable))
	if w.Code != 502 || !strings.Contains(w.Body.String(), "catalog search is temporarily unavailable") || strings.Contains(w.Body.String(), "stable server-compatible") {
		t.Fatal(w.Code, w.Body.String())
	}
}
