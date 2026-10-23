package profilemigrate

import (
	"os"
	"testing"
	"time"

	"cercano/source/server/internal/testenv"
)

// TestMain keeps these tests' default Cercano paths under a temporary home.
// Run with CERCANO_SLEEP_FIXTURE=1, a copy of the test binary creates the
// file CERCANO_SLEEP_READY and sleeps: the live process tests start it under
// the name cercano.
func TestMain(m *testing.M) {
	if os.Getenv("CERCANO_SLEEP_FIXTURE") == "1" {
		_ = os.WriteFile(os.Getenv("CERCANO_SLEEP_READY"), nil, 0o600)
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	os.Exit(testenv.IsolateHome(m))
}
