package conversation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// TaskStore is a host-only extension. Worker conversation proxies do not own
// independent tasks and do not acquire this persistence authority.
type TaskStore interface {
	InitTasks(context.Context) error
	InsertTask(context.Context, TaskRecord) error
	GetTask(context.Context, string, string) (TaskRecord, error)
	GetTaskByChild(context.Context, string, string) (TaskRecord, error)
	ListTasks(context.Context, string, int) ([]TaskRecord, error)
	StartTask(context.Context, string, string) error
	FinishTask(context.Context, TaskRecord) (bool, error)
	ReconcileTasks(context.Context) error
	PendingTasks(context.Context, string) ([]TaskRecord, error)
	ClaimTaskWake(context.Context, string, string) (bool, error)
	HandleTaskNotification(context.Context, string, string) error
}

// TaskRecord is durable execution state, distinct from a child transcript or
// tab visibility. ResultJSON contains the complete dispatch result, including
// partial output on failure. Interrupted work must never be replayed implicitly.
type TaskRecord struct {
	Evidence            *TaskExecutionEvidence `json:"evidence,omitempty"` // inspection-only; not persisted
	ID                  string                 `json:"id"`
	ParentID            string                 `json:"parent_id"`
	Status              string                 `json:"status"`
	Task                string                 `json:"task"`
	WorkDir             string                 `json:"work_dir"`
	ChildID             string                 `json:"child_id,omitempty"`
	ResultJSON          json.RawMessage        `json:"result,omitempty"`
	Error               string                 `json:"error,omitempty"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
	NotificationPending bool                   `json:"notification_pending"`
	WakeClaimed         bool                   `json:"wake_claimed"`
}

func (t TaskRecord) Terminal() bool {
	switch t.Status {
	case "completed", "failed", "stopped", "interrupted":
		return true
	}
	return false
}

// InitTasks is additive and idempotent. Reconciliation is deliberately separate:
// constructing a second accessor must not interrupt a live manager's tasks.
func (s *sqliteStore) InitTasks(ctx context.Context) error {
	if s.coexistencePath != "" {
		return fmt.Errorf("coexistence forbids global InitTasks; use prepared tasks and explicit ownership")
	}
	_, err := s.execOwned(ctx, `CREATE TABLE IF NOT EXISTS delegated_tasks (
 id TEXT PRIMARY KEY, parent_id TEXT NOT NULL, status TEXT NOT NULL,
 task TEXT NOT NULL, work_dir TEXT NOT NULL, child_id TEXT NOT NULL DEFAULT '',
 result_json TEXT NOT NULL DEFAULT 'null', error TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
 notification_pending INTEGER NOT NULL DEFAULT 0, wake_claimed INTEGER NOT NULL DEFAULT 0
 ); CREATE INDEX IF NOT EXISTS delegated_tasks_parent ON delegated_tasks(parent_id, created_at);`)
	return err
}

func (s *sqliteStore) InsertTask(ctx context.Context, t TaskRecord) error {
	if t.ID == "" || t.ParentID == "" || t.Status != "accepted" {
		return fmt.Errorf("invalid accepted task")
	}
	_, err := s.execTarget(ctx, t.ParentID, `INSERT INTO delegated_tasks(id,parent_id,status,task,work_dir,child_id,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, t.ID, t.ParentID, t.Status, t.Task, t.WorkDir, t.ChildID, t.CreatedAt.UnixMilli(), t.UpdatedAt.UnixMilli())
	return err
}

var ErrTaskNotFound = errors.New("task not found in this conversation")

// ErrTaskWaitInterrupted reports that a Wait returned early because another
// task in the SAME conversation finished while the caller held the foreground
// turn. The waited task was not touched and is still running; its current
// record accompanies the error. The pending sibling completion is left for the
// host's normal completion-boundary machinery — this signal only releases the
// blocked wait and consumes nothing. The error text is also a stable wire
// marker for the worker task-control bridge, which cannot transport Go error
// identity across the process boundary.
var ErrTaskWaitInterrupted = errors.New("task wait interrupted: another task in this conversation completed; the waited task is still running")

func scanTask(row interface{ Scan(...any) error }) (TaskRecord, error) {
	var t TaskRecord
	var created, updated int64
	var result string
	err := row.Scan(&t.ID, &t.ParentID, &t.Status, &t.Task, &t.WorkDir, &t.ChildID, &result, &t.Error, &created, &updated, &t.NotificationPending, &t.WakeClaimed)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrTaskNotFound
	}
	t.CreatedAt = time.UnixMilli(created)
	t.UpdatedAt = time.UnixMilli(updated)
	t.ResultJSON = json.RawMessage(result)
	return t, err
}

const taskColumns = `id,parent_id,status,task,work_dir,child_id,result_json,error,created_at,updated_at,notification_pending,wake_claimed`

func (s *sqliteStore) GetTask(ctx context.Context, parent, id string) (TaskRecord, error) {
	return scanTask(s.db.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM delegated_tasks WHERE parent_id=? AND id=?`, parent, id))
}

// GetTaskByChild resolves transcript identity without conflating it with a task
// handle. Ownership remains scoped to the child's actual parent conversation.
func (s *sqliteStore) GetTaskByChild(ctx context.Context, parent, child string) (TaskRecord, error) {
	return scanTask(s.db.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM delegated_tasks WHERE parent_id=? AND child_id=?`, parent, child))
}
func (s *sqliteStore) ListTasks(ctx context.Context, parent string, limit int) ([]TaskRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+taskColumns+` FROM delegated_tasks WHERE parent_id=? ORDER BY created_at DESC,id LIMIT ?`, parent, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []TaskRecord{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}
func (s *sqliteStore) StartTask(ctx context.Context, parent, id string) error {
	result, err := s.execResolvedTarget(ctx, parent, "SELECT parent_id FROM delegated_tasks WHERE id=?", id, `UPDATE delegated_tasks SET status='running',updated_at=? WHERE parent_id=? AND id=? AND status='accepted'`, time.Now().UnixMilli(), parent, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("task not accepted")
	}
	return nil
}

// FinishTask writes an outcome and its pending notification atomically. The
// first terminal outcome wins; duplicates cannot erase results or rearm wakes.
func (s *sqliteStore) FinishTask(ctx context.Context, t TaskRecord) (bool, error) {
	if !t.Terminal() {
		return false, fmt.Errorf("task outcome is not terminal")
	}
	resultJSON := string(t.ResultJSON)
	if resultJSON == "" {
		resultJSON = "null"
	}
	if !json.Valid([]byte(resultJSON)) {
		return false, fmt.Errorf("invalid task result JSON")
	}
	result, err := s.execResolvedTarget(ctx, t.ParentID, "SELECT parent_id FROM delegated_tasks WHERE id=?", t.ID, `UPDATE delegated_tasks SET status=?,child_id=?,result_json=?,error=?,updated_at=?,notification_pending=1,wake_claimed=0 WHERE parent_id=? AND id=? AND status IN ('accepted','running')`, t.Status, t.ChildID, resultJSON, t.Error, time.Now().UnixMilli(), t.ParentID, t.ID)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

// ReconcileTasks is called once when the owning host starts, never on attach.
func (s *sqliteStore) ReconcileTasks(ctx context.Context) error {
	if s.coexistencePath != "" {
		return fmt.Errorf("coexistence forbids global ReconcileTasks; use prepared tasks and explicit ownership")
	}
	_, err := s.execOwned(ctx, `UPDATE delegated_tasks SET status='interrupted',error='Agent restarted; unfinished execution was not replayed. Inspect partial work before retrying.',updated_at=?,notification_pending=1,wake_claimed=0 WHERE status IN ('accepted','running')`, time.Now().UnixMilli())
	return err
}

// PendingTasks includes already-woken but unhandled outcomes. Wake claiming is
// independent of handling so a canceled notification turn cannot lose evidence.
func (s *sqliteStore) PendingTasks(ctx context.Context, parent string) ([]TaskRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+taskColumns+` FROM delegated_tasks WHERE parent_id=? AND notification_pending=1 ORDER BY created_at,id`, parent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []TaskRecord{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}
func (s *sqliteStore) ClaimTaskWake(ctx context.Context, parent, id string) (bool, error) {
	result, err := s.execResolvedTarget(ctx, parent, "SELECT parent_id FROM delegated_tasks WHERE id=?", id, `UPDATE delegated_tasks SET wake_claimed=1 WHERE parent_id=? AND id=? AND notification_pending=1 AND wake_claimed=0`, parent, id)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
func (s *sqliteStore) HandleTaskNotification(ctx context.Context, parent, id string) error {
	_, err := s.execResolvedTarget(ctx, parent, "SELECT parent_id FROM delegated_tasks WHERE id=?", id, `UPDATE delegated_tasks SET notification_pending=0 WHERE parent_id=? AND id=?`, parent, id)
	return err
}
