package main

// Wiring tests for the real main(): assert the private-update dispatch and
// the install-context claim are wired at the TOP of main() and in the
// required order, and the version capability query is exposed as documented
// with unchanged plain `version` output. These are exact-match guards against
// regression — e.g. moving the claim below the subcommand switch would let
// early private-update arguments bypass required verification.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"
)

// mainEncodedLines holds the real main.go source lines, read at test init so
// the assertions below are made against the actual shipped wiring, not a copy.
var mainEncodedLines = loadMainLines()

func loadMainLines() []string {
	data, err := os.ReadFile("main.go")
	if err != nil {
		panic(err)
	}
	return splitSourceLines(string(data))
}

// splitSourceLines splits Go source into lines, normalizing CRLF line
// endings to LF so a Windows checkout of main.go behaves identically to
// Linux/macOS — in particular, `$`-anchored regexes (e.g. `verified = v$`)
// must not fail on a trailing carriage return.
func splitSourceLines(data string) []string {
	return strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
}

// assertMatches fails with the pattern and the mismatched lines.
func assertMatches(t *testing.T, what, pattern string, want []string) {
	t.Helper()
	re := regexp.MustCompile(pattern)
	var got []string
	for _, line := range mainEncodedLines {
		if re.MatchString(line) {
			got = append(got, strings.TrimSpace(line))
		}
	}
	if len(got) != len(want) {
		t.Fatalf("%s: pattern %q matched %d lines, want %d (got %q)", what, pattern, len(got), len(want), got)
	}
	for i, wantLine := range want {
		if got[i] != wantLine {
			t.Fatalf("%s: line %d = %q, want %q", what, i, got[i], wantLine)
		}
	}
}

func TestMainWiresPrivateDispatchAndInstallContextClaimInOrder(t *testing.T) {
	// The private-update dispatch must be the first startup gate and the
	// install-context claim must run before the subcommand switch (so no
	// subcommand — including early ones — can start before required child
	// verification).
	assertMatches(t, "dispatchPrivateUpdate call", `dispatchPrivateUpdate\(os\.Args\[1:\], os\.Stdout, os\.Stderr, nil\); claimed`,
		[]string{
			"if code, claimed := dispatchPrivateUpdate(os.Args[1:], os.Stdout, os.Stderr, nil); claimed {",
		})
	assertMatches(t, "claimInstallContext call", `claimInstallContext\(os\.Args\); !ok`,
		[]string{
			"if v, code, ok := claimInstallContext(os.Args); !ok {",
		})
	// Ordering: dispatch before claim, claim before the subcommand switch.
	dispatchAt, claimAt, switchAt := -1, -1, -1
	for i, line := range mainEncodedLines {
		switch {
		case strings.Contains(line, "dispatchPrivateUpdate(os.Args[1:]"):
			if dispatchAt == -1 {
				dispatchAt = i
			}
		case strings.Contains(line, "claimInstallContext(os.Args)"):
			if claimAt == -1 {
				claimAt = i
			}
		case strings.Contains(line, `switch os.Args[1] {`):
			if switchAt == -1 {
				switchAt = i
			}
		}
	}
	if dispatchAt < 0 || claimAt < 0 || switchAt < 0 {
		t.Fatalf("main wiring incomplete: dispatchAt=%d claimAt=%d switchAt=%d", dispatchAt, claimAt, switchAt)
	}
	if !(dispatchAt < claimAt && claimAt < switchAt) {
		t.Fatalf("order wrong: dispatch(%d) claim(%d) switch(%d) — the claim must precede the subcommand switch so private-update ordering cannot bypass required verification", dispatchAt, claimAt, switchAt)
	}
}

func TestMainWiresRefusalExits(t *testing.T) {
	// All three gates must exit with the returned refusal code (exactly three
	// `os.Exit(code)` occurrences in main(), one per gate).
	assertMatches(t, "gate exits", `os\.Exit\(code\)$`,
		[]string{
			"os.Exit(code)",
			"os.Exit(code)",
			"os.Exit(code)",
		})
}

func TestMainWiresVerifiedContextPropagation(t *testing.T) {
	// The verified result of the early gate must be hoisted (not scoped to
	// the if-statement) and threaded explicitly through the server startup
	// paths — runServerMode and startGRPCServer carry it as a parameter and
	// the installed-TUI observer receives it — never parked in a
	// process-global. MCP mode is preserved unchanged (no verified
	// threading), and runMCPMode keeps its original signature.
	assertMatches(t, "verified declaration", `var verified installcontext\.Verified`,
		[]string{"var verified installcontext.Verified"})
	assertMatches(t, "verified hoist", `verified = v$`,
		[]string{"verified = v"})
	assertMatches(t, "runServerMode verified", `runServerMode\(cfg, verified\)`,
		[]string{
			"runServerMode(cfg, verified)",
			"runServerMode(cfg, verified)",
			"runServerMode(cfg, verified)",
		})
	assertMatches(t, "startGRPCServer verified", `startGRPCServer\(cfg, ":"\+cfg\.Port, crashWriter, stopRequest, verified\)`,
		[]string{`addr, cleanup, stopRequest, err := startGRPCServer(cfg, ":"+cfg.Port, crashWriter, stopRequest, verified)`})
	assertMatches(t, "MCP mode unchanged", `runMCPMode\(cfg, \*grpcAddr\)`,
		[]string{"runMCPMode(cfg, *grpcAddr)"})
	assertMatches(t, "TUI notices verified", `startInstalledTUINotices\(verified, srv\)`,
		[]string{"stopTUINotices := startInstalledTUINotices(verified, srv)"})
}

// TestSplitSourceLinesNormalizesCRLF guards the Windows-CRLF regression
// fixed alongside CI run 39162961836: a CRLF checkout left a trailing \r on
// every line, so `$`-anchored patterns matched zero lines.
func TestSplitSourceLinesNormalizesCRLF(t *testing.T) {
	lines := splitSourceLines("var verified installcontext.Verified\r\n\t\tverified = v\r\n")
	re := regexp.MustCompile(`verified = v$`)
	matched := 0
	for _, line := range lines {
		if re.MatchString(line) {
			matched++
		}
	}
	if matched != 1 {
		t.Fatalf("CRLF source: `verified = v$` matched %d lines, want 1 (line endings must be normalized)", matched)
	}
}

func TestMainWiresVersionCapabilityQuery(t *testing.T) {
	assertMatches(t, "version capability guard", `len\(os\.Args\) == 3 && os\.Args\[2\] == "--updater-protocol"`,
		[]string{
			`if len(os.Args) == 3 && os.Args[2] == "--updater-protocol" {`,
		})
	assertMatches(t, "capability print", `printUpdaterProtocol\(os\.Stdout\); err != nil`,
		[]string{
			"if err := printUpdaterProtocol(os.Stdout); err != nil {",
		})
	// Plain `version` output must stay unchanged (two occurrences: the
	// version subcommand and the -version/-v flag path).
	assertMatches(t, "plain version print", `fmt\.Printf\("cercano v%s\\n", version\)`,
		[]string{
			`fmt.Printf("cercano v%s\n", version)`,
			`fmt.Printf("cercano v%s\n", version)`,
		})
}

func TestMainStableLauncherRunsAfterChildVerificationBeforeCommands(t *testing.T) {
	text := strings.Join(mainEncodedLines, "\n")
	claim := strings.Index(text, "claimInstallContext(os.Args)")
	launch := strings.Index(text, "stablelaunch.Startup(os.Args[1:], false, verified)")
	commands := strings.Index(text, "switch os.Args[1] {")
	if claim < 0 || launch <= claim || commands <= launch {
		t.Fatal("stable launcher is outside verified startup boundary")
	}
}

func TestMainDispatcherIsFirstExecutableStatement(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "main" {
			continue
		}
		if len(fn.Body.List) == 0 {
			t.Fatal("empty main")
		}
		gate, ok := fn.Body.List[0].(*ast.IfStmt)
		if !ok {
			t.Fatal("first statement must be dispatcher gate")
		}
		init, ok := gate.Init.(*ast.AssignStmt)
		if !ok || len(init.Rhs) != 1 {
			t.Fatal("missing dispatch assignment")
		}
		call, ok := init.Rhs[0].(*ast.CallExpr)
		if !ok {
			t.Fatal("missing dispatch call")
		}
		name, ok := call.Fun.(*ast.Ident)
		if !ok || name.Name != "dispatchPrivateUpdate" {
			t.Fatal("dispatch is not first")
		}
		return
	}
	t.Fatal("main missing")
}
