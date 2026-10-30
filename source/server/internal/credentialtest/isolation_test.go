package credentialtest

import (
	"net/http"
	"os"
	"strings"
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

// Exercise the inherited loopback policy with a memory transport, never sockets.
func TestEnableLoopbackPolicy(t *testing.T) {
	Enable(t)
	policy, ok := http.DefaultTransport.(loopbackOnly)
	if !ok {
		t.Fatal("Enable must retain the loopback-only transport")
	}
	var hosts []string
	policy.next = isolationRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		hosts = append(hosts, req.URL.Host)
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})
	for _, host := range []string{"127.0.0.1:1234", "localhost:1234", "[::1]:1234"} {
		req, err := http.NewRequest(http.MethodGet, "http://"+host, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := policy.RoundTrip(req); err != nil {
			t.Fatalf("loopback rejected: %v", err)
		}
		if req.URL.Host != host {
			t.Fatal("policy mutated original request")
		}
	}
	if len(hosts) != 3 || hosts[1] != "127.0.0.1:1234" {
		t.Fatalf("loopback forwarding: %v", hosts)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://provider.invalid/token", nil)
	if _, err := policy.RoundTrip(req); err == nil || len(hosts) != 3 {
		t.Fatal("external request reached memory transport")
	}
}

type isolationRoundTripFunc func(*http.Request) (*http.Response, error)

func (f isolationRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestHermeticIsolation(t *testing.T) {
	inherited := map[string]string{
		"ANTHROPIC_AUTH_TOKEN":       "synthetic-inherited-token",
		"ANTHROPIC_BASE_URL":         "https://inherited.invalid",
		"ANTHROPIC_FIXTURE_OVERRIDE": "synthetic-inherited-override",
	}
	for key, value := range inherited {
		t.Setenv(key, value)
	}
	Enable(t)
	for key, value := range inherited {
		if os.Getenv(key) != value {
			t.Fatalf("Enable changed legacy environment behavior for %s", key)
		}
	}
	outer := http.DefaultTransport
	t.Run("deny all", func(t *testing.T) {
		EnableHermetic(t)
		for _, entry := range os.Environ() {
			key, value, _ := strings.Cut(entry, "=")
			if strings.HasPrefix(key, "ANTHROPIC_") && value != "" {
				t.Fatalf("hermetic isolation inherited %s", key)
			}
		}
		trap, ok := http.DefaultTransport.(*ForbiddenTransport)
		if !ok {
			t.Fatal("hermetic default must be a counting forbidden transport")
		}
		for _, url := range []string{"http://127.0.0.1:1", "http://localhost:1", "http://[::1]:1", "https://provider.invalid/token"} {
			if _, err := http.Get(url); err == nil {
				t.Fatalf("wire allowed: %s", url)
			}
		}
		if trap.Calls.Load() != 4 {
			t.Fatalf("forbidden calls = %d, want 4", trap.Calls.Load())
		}
	})
	if http.DefaultTransport != outer {
		t.Fatal("hermetic isolation did not restore outer transport")
	}
	for key, value := range inherited {
		if os.Getenv(key) != value {
			t.Fatalf("hermetic cleanup did not restore %s", key)
		}
	}
}
