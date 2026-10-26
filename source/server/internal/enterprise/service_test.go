package enterprise

import "testing"

func TestEnterpriseServiceName(t *testing.T) {
	if got := EnterpriseServiceName("default"); got != "cercano-enterprise" {
		t.Fatalf("default = %q", got)
	}
	if got := EnterpriseServiceName("work"); got != "cercano-enterprise.work" {
		t.Fatalf("work = %q", got)
	}
}
