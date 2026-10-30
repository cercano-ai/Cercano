package slash

import (
	"reflect"
	"strings"
	"testing"
)

func TestRegistry_DispatchExact(t *testing.T) {
	r := New()
	called := false
	r.Register(Command{
		Name:    "foo",
		Help:    "test",
		Handler: func(args []string) Result { called = true; return Result{Kind: ResultText, Text: "ok"} },
	})
	res, ok := r.Dispatch("/foo")
	if !ok || !called || res.Text != "ok" {
		t.Fatalf("dispatch /foo failed: ok=%v called=%v text=%q", ok, called, res.Text)
	}
}

func TestRegistry_DispatchAlias(t *testing.T) {
	r := New()
	r.Register(Command{
		Name:    "quit",
		Aliases: []string{"exit"},
		Help:    "leave",
		Handler: func(args []string) Result { return Result{Kind: ResultQuit} },
	})
	for _, in := range []string{"/quit", "quit", "/exit", "exit"} {
		res, ok := r.Dispatch(in)
		if !ok || res.Kind != ResultQuit {
			t.Errorf("dispatch %q: ok=%v kind=%v", in, ok, res.Kind)
		}
	}
}

func TestRegistry_PrefixSuggestion(t *testing.T) {
	r := New()
	r.Register(Command{Name: "models", Help: "list", Handler: func(args []string) Result { return Result{} }})
	r.Register(Command{Name: "model", Help: "set", Handler: func(args []string) Result { return Result{} }})

	res, ok := r.Dispatch("/mod")
	if ok {
		t.Fatalf("expected no exact match for /mod")
	}
	if res.Text != "unknown command /mod — did you mean /model?" {
		t.Errorf("suggestion mismatch: %q", res.Text)
	}
}

func TestRegistry_UnknownNoPrefix(t *testing.T) {
	r := New()
	r.Register(Command{Name: "models", Help: "list", Handler: func(args []string) Result { return Result{} }})

	res, ok := r.Dispatch("/xyz")
	if ok || res.Text == "" {
		t.Fatalf("expected unknown w/ help text: ok=%v text=%q", ok, res.Text)
	}
}

func TestRegistry_DuplicateNamePanics(t *testing.T) {
	r := New()
	r.Register(Command{Name: "foo", Help: "1", Handler: func(args []string) Result { return Result{} }})
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate name")
		}
	}()
	r.Register(Command{Name: "foo", Help: "2", Handler: func(args []string) Result { return Result{} }})
}

func TestRegisterBasics_All3Present(t *testing.T) {
	r := New()
	RegisterBasics(r)
	for _, want := range []string{"quit", "exit", "clear", "help"} {
		if _, ok := r.cmds[want]; !ok {
			t.Errorf("missing command %q", want)
		}
	}
}

func TestRegistry_CaseInsensitive(t *testing.T) {
	r := New()
	var gotArgs []string
	r.Register(Command{Name: "ConFig", Aliases: []string{"Prefs"}, Handler: func(args []string) Result {
		gotArgs = args
		return Result{Text: "ok"}
	}})
	for _, name := range []string{"config", "CONFIG", "ConFig", "prefs", "PREFS", "PrEfS"} {
		t.Run(name, func(t *testing.T) {
			c, ok := r.Lookup(name)
			if !ok || c.Name != "ConFig" {
				t.Errorf("Lookup(%q) = %v, %v", name, c, ok)
			}
			res, ok := r.Dispatch("/" + name + "  API-Key\tMiXeD/Path AbC123")
			if !ok || res.Text != "ok" {
				t.Fatalf("Dispatch(%q) = %+v, %v", name, res, ok)
			}
			if want := []string{"API-Key", "MiXeD/Path", "AbC123"}; !reflect.DeepEqual(gotArgs, want) {
				t.Errorf("args = %q, want %q", gotArgs, want)
			}
		})
	}
	for _, prefix := range []string{"co", "/CO", "/cO"} {
		if got := r.PrefixMatches(prefix); !reflect.DeepEqual(got, []string{"ConFig"}) {
			t.Errorf("PrefixMatches(%q) = %q", prefix, got)
		}
	}
	if got := r.PrefixMatches("PR"); len(got) != 0 {
		t.Errorf("alias prefix should remain excluded: %q", got)
	}
	if got := r.All(); len(got) != 1 || got[0].Name != "ConFig" || got[0].Aliases[0] != "Prefs" {
		t.Errorf("canonical metadata changed: %+v", got)
	}
	res, ok := r.Dispatch("/CO")
	if ok || res.Text != "unknown command /CO — did you mean /ConFig?" {
		t.Errorf("suggestion = %+v, %v", res, ok)
	}
}

func TestRegistry_CaseInsensitiveDuplicatePanics(t *testing.T) {
	for _, c := range []Command{
		{Name: "CONFIG"},
		{Name: "PREFS"},
		{Name: "other", Aliases: []string{"CONFIG"}},
		{Name: "other", Aliases: []string{"PREFS"}},
	} {
		t.Run(c.Name+"/"+strings.Join(c.Aliases, ","), func(t *testing.T) {
			r := New()
			r.Register(Command{Name: "config", Aliases: []string{"prefs"}})
			defer func() {
				if recover() == nil {
					t.Fatal("expected case-insensitive duplicate panic")
				}
			}()
			r.Register(c)
		})
	}
}

func TestRegistry_ConfigResumeCaseInsensitive(t *testing.T) {
	r := New()
	RegisterConfig(r, nil)
	RegisterHistory(r, nil, nil)
	for _, in := range []string{"/config", "/CONFIG", "/CoNfIg"} {
		res, ok := r.Dispatch(in)
		if !ok || res.Kind != ResultOpenSettings {
			t.Errorf("Dispatch(%q) = %+v, %v", in, res, ok)
		}
	}
	for _, in := range []string{"/resume", "/RESUME", "/ReSuMe"} {
		res, ok := r.Dispatch(in + " Session-AbC123")
		if !ok || res.Kind != ResultResumeConversation || res.Text != "Session-AbC123" {
			t.Errorf("Dispatch(%q) = %+v, %v", in, res, ok)
		}
	}
}
