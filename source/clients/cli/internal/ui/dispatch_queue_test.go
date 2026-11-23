package ui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestDispatchQueuedPromptRelease(t *testing.T) {
	for _, boundary := range []string{"stream-end", "wait-start", "wait-before-dispatch-complete"} {
		t.Run(boundary, func(t *testing.T) {
			m := New(nil, false)
			m.workDirOverride = t.TempDir()
			m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
			canceled := false
			m.cancelStream = func() { canceled = true }
			m.streaming = true
			// A durable child may still be doing ordinary work when the parent
			// enters task_wait. Its lifetime must not gate this queue.
			m.applySubAgentEvent(subAgentEventMsg{id: "child", kind: "started"})
			child := m.chatTabs.tabs["child"]
			child.taskID = "task-child"
			child.view.Apply(toolEntryStartMsg{id: "read", name: "Read"})
			m = send(t, m, chatStreamMsg{gen: m.turnGen, ev: toolEntryStartMsg{id: "d", name: "dispatch"}})
			m.input.SetValue("next main prompt")
			m = send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
			t.Logf("during dispatch: streaming=%t queued=%v canceled=%t waitBlocked=%t", m.streaming, m.mainChat().Queued(), canceled, m.waitBlocked())
			if canceled || len(m.mainChat().Queued()) != 1 {
				t.Fatal("dispatch must queue without canceling")
			}
			if boundary == "wait-before-dispatch-complete" {
				m = send(t, m, chatStreamMsg{gen: m.turnGen, ev: toolEntryStartMsg{id: "w", name: "task_wait"}})
				if canceled || len(m.mainChat().Queued()) != 1 || m.waitBlocked() {
					t.Fatal("overlapping dispatch and wait must not release the prompt yet")
				}
			}
			m = send(t, m, chatStreamMsg{gen: m.turnGen, ev: toolEntryExecCompleteMsg{id: "d"}})
			if boundary == "stream-end" {
				m = send(t, m, streamEndMsg{gen: m.turnGen})
			} else if boundary == "wait-start" {
				m = send(t, m, chatStreamMsg{gen: m.turnGen, ev: toolEntryStartMsg{id: "w", name: "task_wait"}})
			}
			t.Logf("after boundary: streaming=%t queued=%v canceled=%t waitBlocked=%t", m.streaming, m.mainChat().Queued(), canceled, m.waitBlocked())
			if len(m.mainChat().Queued()) != 0 {
				m = send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
				t.Logf("steer probe: streaming=%t queued=%v canceled=%t", m.streaming, m.mainChat().Queued(), canceled)
				if len(m.mainChat().Queued()) != 0 || !canceled {
					t.Fatal("steer probe did not release prompt")
				}
				t.Fatal("queued main prompt required steering to release")
			}
			if !canceled || m.lastSubmittedPrompt != "next main prompt" {
				t.Fatal("queued prompt was not submitted at the boundary")
			}
			if child.done || !child.view.hasInProgressTool() {
				t.Fatal("releasing the parent prompt stopped independent child work")
			}
			for _, entry := range m.mainChat().Entries() {
				if entry.Role == RoleSystem && entry.Content == "⊘ canceled" {
					t.Fatal("automatic wait interruption exposed a cancellation notice")
				}
			}
		})
	}
}

// Dispatch returning (success or failure) is not itself permission to steer
// ongoing foreground work. A real stream end still releases the queued prompt.
func TestDispatchQueueOrdinaryAndFailureBoundaries(t *testing.T) {
	for _, failed := range []bool{false, true} {
		m := New(nil, false)
		m.workDirOverride = t.TempDir()
		m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
		canceled := false
		m.cancelStream = func() { canceled = true }
		m.streaming = true
		m = send(t, m, chatStreamMsg{gen: m.turnGen, ev: toolEntryStartMsg{id: "d", name: "dispatch"}})
		m.input.SetValue("follow up")
		m = send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
		m = send(t, m, chatStreamMsg{gen: m.turnGen, ev: toolEntryExecCompleteMsg{id: "d", isError: failed}})
		if canceled || len(m.mainChat().Queued()) != 1 {
			t.Fatal("dispatch completion alone must not steer ordinary foreground work")
		}
		if failed {
			m = send(t, m, chatStreamMsg{gen: m.turnGen, ev: chatErrorMsg{err: errors.New("dispatch failed")}})
		}
		m = send(t, m, streamEndMsg{gen: m.turnGen})
		if len(m.mainChat().Queued()) != 0 || m.lastSubmittedPrompt != "follow up" {
			t.Fatal("stream completion did not release queued prompt")
		}
	}
}

func TestDispatchQueueWaitReleaseRespectsGenerationAndModelGate(t *testing.T) {
	m := New(nil, false)
	m.workDirOverride = t.TempDir()
	m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	canceled := false
	m.cancelStream = func() { canceled = true }
	m.streaming = true
	m.turnGen = 10
	m.convID = "parent"
	m = send(t, m, chatStreamMsg{gen: m.turnGen, ev: toolEntryStartMsg{id: "d", name: "dispatch"}})
	m.input.SetValue("first")
	m = send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m.sessionModelPending, m.sessionModelConv = true, m.convID
	m = send(t, m, chatStreamMsg{gen: 9, ev: toolEntryStartMsg{id: "stale", name: "task_wait"}})
	if canceled || m.waitBlocked() {
		t.Fatal("stale event released a prompt")
	}
	m = send(t, m, chatStreamMsg{gen: 10, ev: toolEntryExecCompleteMsg{id: "d"}})
	m = send(t, m, chatStreamMsg{gen: 10, ev: toolEntryStartMsg{id: "w", name: "task_wait"}})
	if canceled || len(m.mainChat().Queued()) != 1 {
		t.Fatal("wait transition bypassed pending model selection")
	}
	// Once the gate is released, the next foreground event can drain exactly
	// one prompt. Late completion from the canceled observer stays inert.
	m.sessionModelPending = false
	m.mainChat().Enqueue("second", nil)
	m = send(t, m, chatStreamMsg{gen: 10, ev: toolEntryExecStartMsg{id: "w"}})
	if !canceled || m.lastSubmittedPrompt != "first" || len(m.mainChat().Queued()) != 1 {
		t.Fatal("wait release must submit exactly one queued prompt")
	}
	m = send(t, m, streamEndMsg{gen: 10})
	m = send(t, m, chatStreamMsg{gen: 10, ev: toolEntryStartMsg{id: "late", name: "task_wait"}})
	if m.lastSubmittedPrompt != "first" || len(m.mainChat().Queued()) != 1 {
		t.Fatal("canceled observer drained the next prompt")
	}
}
