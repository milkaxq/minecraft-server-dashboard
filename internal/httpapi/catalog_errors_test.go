package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"minecraft-hosting/internal/catalog"
	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/management"
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

func TestBackupPreflightCodesDoNotMarkUncertainWritesSafeToRetry(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{
		{management.ErrBackupDecision, "BACKUP_DECISION_REQUIRED"},
		{management.ErrBackupUnavailable, "BACKUP_CHECK_FAILED"},
		{control.ErrUnknown, ""},
	} {
		r := httptest.NewRequest("POST", "/servers/id/plugins/catalog/project", nil)
		r = r.WithContext(context.WithValue(r.Context(), requestKey, "test-reference"))
		w := httptest.NewRecorder()
		(&API{}).fail(w, r, tc.err)
		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["code"] != tc.code {
			t.Fatal(body)
		}
	}
}
