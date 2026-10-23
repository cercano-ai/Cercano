package secrets

import "testing"

func TestServiceName(t *testing.T) {
	if got := ServiceName("default"); got != "cercano" {
		t.Fatalf("default = %q", got)
	}
	if got := ServiceName("work"); got != "cercano.work" {
		t.Fatalf("work = %q", got)
	}
}
