package informationhttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRedirectCannotLeakCredentialsAndErrorsAreRedacted(t *testing.T) {
	leaked := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true }))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL+"?key=secret", http.StatusFound)
	}))
	defer origin.Close()
	_, err := Get(t.Context(), Client(), origin.URL+"?apikey=secret", map[string]string{"X-Naver-Client-Secret": "secret"}, 1024)
	if err == nil || leaked || strings.Contains(err.Error(), "secret") {
		t.Fatalf("redirect leaked credentials or exposed URL: %v", err)
	}
}
func TestResponseLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("12345")) }))
	defer server.Close()
	if _, err := Get(t.Context(), Client(), server.URL, nil, 4); err == nil {
		t.Fatal("oversized response accepted")
	}
}
