package server

import (
	"crypto/sha256"
	"strings"
	"time"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	protobuf "google.golang.org/protobuf/proto"
)

// SubscribeTasks reads durable evidence, not a main-turn replay buffer. Pages
// bound each database read, and Send supplies transport backpressure: no event
// queue grows behind a slow observer. A reconnect always refreshes task status;
// supplied progress cursors suppress already-applied immutable records.
func (s *Server) SubscribeTasks(req *proto.SubscribeTasksRequest, stream proto.Agent_SubscribeTasksServer) error {
	if err := s.refuseUnintegratedCoexistenceWork(); err != nil {
		return err
	}
	parent := req.GetParentId()
	if strings.TrimSpace(parent) == "" {
		return status.Error(codes.InvalidArgument, "parent_id required")
	}

	cursors := make(map[string]int64, len(req.Cursors))
	for _, cursor := range req.Cursors {
		if cursor.GetTaskId() == "" || cursor.GetSequence() < 0 {
			return status.Error(codes.InvalidArgument, "invalid progress cursor")
		}
		if _, duplicate := cursors[cursor.TaskId]; duplicate {
			return status.Error(codes.InvalidArgument, "duplicate progress cursor")
		}
		cursors[cursor.TaskId] = cursor.Sequence
	}
	if s.persistSvc == nil || s.persistSvc.Store() == nil {
		return status.Error(codes.Unavailable, "task persistence unavailable")
	}
	store, ok := s.persistSvc.Store().(conversation.TaskSubscriptionStore)
	if !ok {
		return status.Error(codes.Unavailable, "task subscriptions unavailable")
	}
	ctx := stream.Context()
	seen := make(map[string][32]byte)
	drained := make(map[string]bool)
	first := true
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		after := ""
		for {
			tasks, err := store.TaskSnapshotPage(ctx, parent, after)
			if err != nil {
				return status.Error(codes.Internal, "task snapshot unavailable")
			}
			for _, task := range tasks {
				snapshot := &proto.TaskLifecycleSnapshot{TaskId: task.ID, ChildId: task.ChildID, Status: task.Status, Task: task.Task, Error: task.Error}
				data, err := protobuf.Marshal(snapshot)
				if err != nil {
					return err
				}
				hash := sha256.Sum256(data)
				if old, found := seen[task.ID]; !found || old != hash {
					if err := stream.Send(&proto.TaskSubscriptionEvent{Payload: &proto.TaskSubscriptionEvent_Task{Task: snapshot}}); err != nil {
						return err
					}
					seen[task.ID] = hash
				}
				// One bounded progress page per child per pass prevents a noisy child from
				// starving its siblings. A remaining backlog is drained on subsequent passes.
				if task.Terminal() && drained[task.ID] {
					continue
				}
				events, err := store.TaskProgressPage(ctx, parent, task.ID, cursors[task.ID])
				if err != nil {
					return status.Error(codes.Internal, "task progress unavailable")
				}
				drained[task.ID] = task.Terminal() && len(events) < conversation.TaskSubscriptionPageSize
				for _, event := range events {
					progress := &proto.TaskProgressRecord{TaskId: task.ID, ChildId: task.ChildID, Sequence: event.Seq, Kind: event.Kind, Iteration: int32(event.Iteration), PayloadJson: event.PayloadJSON}
					if err := stream.Send(&proto.TaskSubscriptionEvent{Payload: &proto.TaskSubscriptionEvent_Progress{Progress: progress}}); err != nil {
						return err
					}
					cursors[task.ID] = event.Seq
				}
			}
			if len(tasks) < conversation.TaskSubscriptionPageSize {
				break
			}
			after = tasks[len(tasks)-1].ID
		}
		if first {
			if err := stream.Send(&proto.TaskSubscriptionEvent{Payload: &proto.TaskSubscriptionEvent_SnapshotComplete{SnapshotComplete: true}}); err != nil {
				return err
			}
			first = false
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
