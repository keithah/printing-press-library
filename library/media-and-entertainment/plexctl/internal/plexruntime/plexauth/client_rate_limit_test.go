package plexauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/plexctl/internal/cliutil"
)

func TestGetJSONReturnsTypedRateLimitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("slow down"))
	}))
	defer server.Close()

	client := New(server.URL, "client", server.Client())
	var out User
	err := client.getJSON(context.Background(), "/api/v2/user", "token", &out)
	var limited *cliutil.RateLimitError
	if !errors.As(err, &limited) {
		t.Fatalf("getJSON error = %v, want *cliutil.RateLimitError", err)
	}
	if limited.RetryAfter.Seconds() != 2 {
		t.Fatalf("RetryAfter = %s, want 2s", limited.RetryAfter)
	}
}
