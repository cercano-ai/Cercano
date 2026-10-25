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

// PerProfileAgentsValue applies the same rule to a value from another
// process's environment.
func TestPerProfileAgentsValue(t *testing.T) {
	for value, want := range map[string]bool{"": false, "0": false, "false": false, "yes": false, "1": true, "true": true, "T": true} {
		if got := PerProfileAgentsValue(value); got != want {
			t.Errorf("PerProfileAgentsValue(%q) = %v, want %v", value, got, want)
		}
	}
}
