package paths

import "testing"

func TestPerProfileAgentsOffByDefault(t *testing.T) {
	for _, value := range []string{"", "0", "false", "FALSE", "no", "off"} {
		t.Setenv(PerProfileAgentsEnv, value)
		if PerProfileAgents() {
			t.Errorf("PerProfileAgents() with %s=%q = true, want false", PerProfileAgentsEnv, value)
		}
	}
}

func TestPerProfileAgentsOnWhenEnabled(t *testing.T) {
	for _, value := range []string{"1", "t", "true", "TRUE", "True"} {
		t.Setenv(PerProfileAgentsEnv, value)
		if !PerProfileAgents() {
			t.Errorf("PerProfileAgents() with %s=%q = false, want true", PerProfileAgentsEnv, value)
		}
	}
}
