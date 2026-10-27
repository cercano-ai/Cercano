package cloudfactory

import (
	"errors"
	"strings"
	"testing"

	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
)

func TestInvalidFlavorBeforeCredentialPolicy(t *testing.T) {
	t.Setenv(secrets.DisableEnv, "1")
	_, err := BuildCloudProvider(config.CloudProfile{Flavor: "invalid-fixture-flavor"}, "")
	t.Logf("invalid-flavor construction error=%v disabled=%v", err, errors.Is(err, secrets.ErrDisabled))
	if err == nil || errors.Is(err, secrets.ErrDisabled) || !strings.Contains(err.Error(), "invalid-fixture-flavor") {
		t.Fatalf("invalid flavor must be rejected independently of credential policy: %v", err)
	}
}
