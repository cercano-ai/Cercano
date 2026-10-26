package enterprise

import (
	"os"
	"testing"

	"cercano/source/server/internal/testenv"
)

// TestMain keeps these tests' default Cercano paths under a temporary home.
func TestMain(m *testing.M) { os.Exit(testenv.IsolateHome(m)) }
