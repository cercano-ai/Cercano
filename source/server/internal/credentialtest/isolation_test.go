package credentialtest

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"cercano/source/server/internal/secrets"
)

func TestIsolation(t *testing.T) {
	t.Setenv(secrets.DisableEnv, "1")
	t.Setenv("OPENAI_API_KEY", "must-not-inherit")
	transport := http.DefaultTransport
	home := os.Getenv("HOME")
	t.Run("enabled memory backend and fake provider", func(t *testing.T) {
		Enable(t)
		if err := secrets.CheckAllowed(); err != nil {
			t.Fatal(err)
		}
		if os.Getenv("OPENAI_API_KEY") != "" || os.Getenv("HOME") == home {
			t.Fatal("inherited caller state")
		}
		store := secrets.NewMemory()
		if err := store.Set("test-key", "fake"); err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
		defer server.Close()
		resp, err := http.Get(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if _, err := http.Get("https://provider.invalid/token"); err == nil {
			t.Fatal("external provider allowed")
		}
	})
	if os.Getenv(secrets.DisableEnv) != "1" || os.Getenv("HOME") != home || http.DefaultTransport != transport {
		t.Fatal("isolation did not restore outer environment")
	}
}
