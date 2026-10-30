package setupreset

import (
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
	"context"
	"os"
	"testing"
)

type interveningStore struct {
	secrets.Store
	beforeList func()
	deletes    int
}

func (s *interveningStore) List() ([]string, error) { s.beforeList(); return s.Store.List() }
func (s *interveningStore) Delete(k string) error   { s.deletes++; return s.Store.Delete(k) }

func TestResetPublicationRejectsBeforeEffects(t *testing.T) {
	for _, kind := range []string{"stale", "nonregular"} {
		t.Run(kind, func(t *testing.T) {
			p, memory := fixture(t)
			s := &interveningStore{Store: memory, beforeList: func() {
				if kind == "stale" {
					if err := os.WriteFile(p, []byte("port: 54322\n"), 0600); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := os.Remove(p); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(p, 0700); err != nil {
						t.Fatal(err)
					}
				}
			}}
			applied := false
			r, err := Reset(context.Background(), p, s, func(config.Config) error { applied = true; return nil })
			if err == nil || r.ConfigWritten || applied || s.deletes != 0 {
				t.Fatalf("rejected publication effects: err=%v written=%v applied=%v deletes=%d", err, r.ConfigWritten, applied, s.deletes)
			}
		})
	}
}

func TestResetFailedPublicationNoEffects(t *testing.T) {
	p, memory := fixture(t)
	before, _ := os.ReadFile(p)
	s := &interveningStore{Store: memory, beforeList: func() {}}
	r, err := reset(context.Background(), p, s, func(config.Config) error { t.Fatal("applied rejected proposal"); return nil }, func(context.Context, config.ConfigSnapshot) (config.ConfigSnapshot, error) {
		return config.ConfigSnapshot{}, os.ErrPermission
	})
	if err == nil || r.ConfigWritten || s.deletes != 0 {
		t.Fatalf("result=%+v err=%v deletes=%d", r, err, s.deletes)
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Fatal("failed publication changed file")
	}
}

func TestResetInvalidAuthorityNoEffects(t *testing.T) {
	for _, data := range []string{"# comment only\n", "port: 1\n---\nport: 2\n", "port: ["} {
		p, s := fixture(t)
		os.WriteFile(p, []byte(data), 0600)
		r, err := Reset(context.Background(), p, s, func(config.Config) error { t.Fatal("invalid authority applied"); return nil })
		names, _ := s.List()
		after, _ := os.ReadFile(p)
		if err == nil || r.ConfigWritten || len(names) != 2 || string(after) != data {
			t.Fatalf("invalid authority changed: %+v %v", r, err)
		}
	}
}
