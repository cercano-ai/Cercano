package main

import (
	"bytes"
	"testing"
)

func TestCredentialAcceptanceDispatch(t *testing.T) {
	for _, status := range []int{0, -25300, -25293} {
		var out bytes.Buffer
		calls := 0
		if !dispatchCredentialAcceptance([]string{credentialAcceptanceCommand}, &out, func() int { calls++; return status }) {
			t.Fatal("not handled")
		}
		if calls != 1 {
			t.Fatal("wrong query count")
		}
		want := map[int]string{0: "0\n", -25300: "-25300\n", -25293: "-25293\n"}[status]
		if out.String() != want {
			t.Fatalf("unexpected output %q", out.String())
		}
	}
}

func TestCredentialAcceptanceRejectsArguments(t *testing.T) {
	var out bytes.Buffer
	query := func() int { t.Fatal("query must not run"); return 0 }
	if !dispatchCredentialAcceptance([]string{credentialAcceptanceCommand, "other-profile"}, &out, query) || out.String() != "-50\n" {
		t.Fatal("invalid invocation not rejected")
	}
	out.Reset()
	if dispatchCredentialAcceptance([]string{"development-host"}, &out, query) || out.Len() != 0 {
		t.Fatal("ordinary command intercepted")
	}
}

func TestCredentialAcceptanceDisabled(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	if queryAcceptanceCredential() != -4 {
		t.Fatal("disabled query did not fail closed")
	}
}
