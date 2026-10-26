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

func TestAWSInputsAreIsolated(t *testing.T) {
	for _, key := range []string{"AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE", "AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_CONTAINER_CREDENTIALS_FULL_URI", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE", "AWS_ROLE_ARN", "AWS_DEFAULT_PROFILE", "AWS_ENDPOINT_URL", "AWS_CA_BUNDLE"} {
		t.Setenv(key, "inherited-synthetic-sentinel")
	}
	Enable(t)
	for _, key := range []string{"AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE"} {
		data, err := os.ReadFile(os.Getenv(key))
		if err != nil || len(data) != 0 {
			t.Fatalf("%s must name an empty fixture: %v", key, err)
		}
	}
	for _, key := range []string{"AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_CONTAINER_CREDENTIALS_FULL_URI", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE", "AWS_ROLE_ARN", "AWS_DEFAULT_PROFILE", "AWS_ENDPOINT_URL", "AWS_CA_BUNDLE"} {
		if os.Getenv(key) != "" {
			t.Errorf("inherited %s", key)
		}
	}
}
