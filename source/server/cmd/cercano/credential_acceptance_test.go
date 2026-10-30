package main

import (
	"bytes"
	"testing"
)

func TestCredentialAcceptanceDispatch(t *testing.T) {
	for _, status := range []int{0, -25300, -25293} {
		var out bytes.Buffer
		calls := 0
		if !dispatchCredentialAcceptance([]string{credentialAcceptanceCommand}, &out, func(bool) int { calls++; return status }) {
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
	query := func(bool) int { t.Fatal("query must not run"); return 0 }
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
	if queryAcceptanceCredential(false) != -4 {
		t.Fatal("disabled query did not fail closed")
	}
}

func TestCredentialAcceptanceInteractiveDispatch(t *testing.T) {
	for _, args := range [][]string{{credentialAcceptanceCommand}, {credentialAcceptanceCommand, "--interactive"}} {
		var out bytes.Buffer
		calls := 0
		dispatchCredentialAcceptance(args, &out, func(interactive bool) int {
			calls++
			if interactive != (len(args) == 2) {
				t.Fatal("wrong interaction policy")
			}
			return -25293
		})
		if calls != 1 || out.String() != "-25293\n" {
			t.Fatal("wrong dispatch")
		}
	}
	for _, tail := range [][]string{{"--interactive=true"}, {"--interactive", "extra"}, {"--interactive", "--interactive"}} {
		var out bytes.Buffer
		dispatchCredentialAcceptance(append([]string{credentialAcceptanceCommand}, tail...), &out, func(bool) int { t.Fatal("invalid args queried"); return 0 })
		if out.String() != "-50\n" {
			t.Fatal("invalid args accepted")
		}
	}
}

func TestCredentialAcceptancePolicy(t *testing.T) {
	for _, value := range []string{"1", "", "true", "garbage"} {
		t.Setenv("CERCANO_DISABLE_CREDENTIALS", value)
		for _, interactive := range []bool{false, true} {
			if runAcceptanceCredential(interactive, func(bool) int { t.Fatal("policy reached native"); return 0 }) != -4 {
				t.Fatal("policy did not refuse")
			}
		}
	}
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	for _, interactive := range []bool{false, true} {
		calls := 0
		if runAcceptanceCredential(interactive, func(got bool) int {
			calls++
			if got != interactive {
				t.Fatal("policy changed option")
			}
			return -25300
		}) != -25300 || calls != 1 {
			t.Fatal("fake native not forwarded")
		}
	}
}
