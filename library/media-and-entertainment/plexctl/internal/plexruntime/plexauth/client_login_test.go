package plexauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoginReportsExpiredPINWithActionableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/pins":
			if r.Method != http.MethodPost {
				t.Fatalf("create pin method = %s, want POST", r.Method)
			}
			_, _ = w.Write([]byte(`{"id":123,"code":"example"}`))
		case "/api/v2/pins/123":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := New(server.URL, "test-client", server.Client())
	client.PollInterval = time.Millisecond
	client.Timeout = time.Second
	_, err := client.Login(context.Background())
	if err == nil || !strings.Contains(err.Error(), "expired or was cancelled") {
		t.Fatalf("Login() error = %v, want expired/cancelled PIN guidance", err)
	}
}
