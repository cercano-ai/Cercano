package conversation

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Targeted DML goes through execTarget/execResolvedTarget or beginOwned with
// explicit targets; execOwned is the row-fenced bulk primitive. Row triggers are
// installed on the transaction's connection (including replacement connections),
// so predicates, cascades, bulk operations and both sides of an UPDATE cannot
// evade the same fence. The temporary capability set is replaced inside EVERY
// write transaction; it is never a mutable process/store 'current owner'.
// Raw db writes are reserved for legacy migrations and ownership transitions.
// Query/QueryRow paths in this package are read-only.
var ownedTables = map[string]string{
	"conversations":              "id",
	"turns":                      "conversation_id",
	"conversation_compaction":    "conversation_id",
	"conversation_context_usage": "conversation_id",
	"conversation_request_usage": "conversation_id",
	"conversation_chat_routes":   "conversation_id",
	"autonomy_runs":              "conversation_id",
	"dispatch_events":            "conversation_id",
	"delegated_tasks":            "parent_id",
}

func (s *sqliteStore) execOwned(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if s.coexistencePath == "" {
		return s.db.ExecContext(ctx, query, args...)
	}
	tx, err := s.beginOwned(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *sqliteStore) beginOwned(ctx context.Context, opts *sql.TxOptions, targets ...string) (*sql.Tx, error) {
	if s.coexistencePath == "" {
		return s.db.BeginTx(ctx, opts)
	}
	caps, _ := ctx.Value(ownershipContextKey{}).([]*Ownership)
	if len(caps) == 0 {
		return nil, fmt.Errorf("coexistence write requires explicit ownership capability")
	}
	tx, err := s.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			tx.Rollback()
		}
	}()
	if err = s.checkOwnershipContract(ctx, tx); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `CREATE TEMP TABLE IF NOT EXISTS ownership_capabilities (conversation_id TEXT PRIMARY KEY, owner TEXT NOT NULL, epoch TEXT NOT NULL); DELETE FROM ownership_capabilities;`); err != nil {
		return nil, err
	}
	for _, o := range caps {
		if o == nil {
			return nil, fmt.Errorf("nil ownership capability")
		}
		active := o.usable.Load()
		if o.store != s || !active {
			return nil, fmt.Errorf("closed or foreign-store ownership capability")
		}
		var n int
		err = tx.QueryRowContext(ctx, `SELECT count(*) FROM conversation_ownership WHERE conversation_id=? AND owner=? AND epoch=? AND state='owned'`, o.conversation, o.owner, o.epoch).Scan(&n)
		if err != nil {
			return nil, err
		}
		if n != 1 {
			return nil, fmt.Errorf("stale ownership epoch for %q", o.conversation)
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO ownership_capabilities VALUES(?,?,?)`, o.conversation, o.owner, o.epoch); err != nil {
			return nil, err
		}
	}
	for table, col := range ownedTables {
		for _, op := range []string{"INSERT", "UPDATE", "DELETE"} {
			refs := []string{"NEW"}
			if op == "DELETE" {
				refs = []string{"OLD"}
			}
			if op == "UPDATE" {
				refs = []string{"OLD", "NEW"}
			}
			body := ""
			for _, ref := range refs {
				body += rowOwnershipCheck(ref+"."+col, false)
				if table == "conversations" && (op == "INSERT" || op == "UPDATE") {
					for _, relationship := range []string{"parent_id", "precursor_id"} {
						check := rowOwnershipCheck(ref+"."+relationship, true)
						if op == "UPDATE" {
							// Changing an edge requires both endpoints' authority;
							// editing unrelated content does not mutate that edge.
							check = strings.Replace(check, "CASE WHEN ", "CASE WHEN OLD."+relationship+" IS NOT NEW."+relationship+" AND ", 1)
						}
						body += check
					}
				}
			}
			q := fmt.Sprintf(`CREATE TEMP TRIGGER IF NOT EXISTS fence_%s_%s BEFORE %s ON main.%s BEGIN %s END`, table, op, op, table, body)
			if _, err = tx.ExecContext(ctx, q); err != nil {
				return nil, err
			}
		}
	}
	for _, target := range targets {
		if err := authorizeTarget(ctx, tx, target); err != nil {
			return nil, err
		}
	}
	ok = true
	return tx, nil
}

func rowOwnershipCheck(expr string, optional bool) string {
	skip := ""
	if optional {
		skip = expr + "<>'' AND "
	}
	return fmt.Sprintf(`SELECT CASE WHEN %sNOT EXISTS (
 SELECT 1 FROM ownership_capabilities c JOIN conversation_ownership o
 ON o.conversation_id=c.conversation_id AND o.owner=c.owner AND o.epoch=c.epoch
 WHERE o.state='owned' AND c.conversation_id=%s
 ) THEN RAISE(ABORT,'missing or stale conversation ownership capability (including parent/precursor)') END;`, skip, expr)
}

// authorizeTarget is independent of DML predicates and row counts. An absent
// conversation may remain an idempotent no-op only with its live capability;
// this also permits authorized creation before a conversation row exists.
func authorizeTarget(ctx context.Context, tx *sql.Tx, target string) error {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT count(*) FROM ownership_capabilities c JOIN conversation_ownership o ON o.conversation_id=c.conversation_id AND o.owner=c.owner AND o.epoch=c.epoch WHERE o.state='owned' AND c.conversation_id=?`, target).Scan(&n)
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("missing or stale ownership capability for target %q", target)
	}
	return nil
}

func (s *sqliteStore) execTarget(ctx context.Context, target, query string, args ...any) (sql.Result, error) {
	return s.execResolvedTarget(ctx, target, "", "", query, args...)
}

// ID-keyed mutations resolve their actual owner inside the write transaction.
// Missing task/run IDs return sql.ErrNoRows, never successful no-ops. A supplied
// parent must match the stored parent, even when the DML predicate matches zero.
func (s *sqliteStore) execResolvedTarget(ctx context.Context, target, lookup, id, query string, args ...any) (sql.Result, error) {
	if s.coexistencePath == "" {
		return s.db.ExecContext(ctx, query, args...)
	}
	tx, err := s.beginOwned(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if lookup != "" {
		var actual string
		if err := tx.QueryRowContext(ctx, lookup, id).Scan(&actual); err != nil {
			return nil, err
		}
		if target != "" && target != actual {
			return nil, fmt.Errorf("target conversation does not match stored owner")
		}
		target = actual
	}
	if err := authorizeTarget(ctx, tx, target); err != nil {
		return nil, err
	}
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return res, nil
}
