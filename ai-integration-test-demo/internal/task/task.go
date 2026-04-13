package task

import (
	"fmt"

	"github.com/example/ai-integration-test-demo/internal/event"
)

type TaskState string

const (
	StateActive    TaskState = "active"
	StateCompleted TaskState = "completed"
)

type Task struct {
	TaskID   int       `json:"taskId"`
	Target   int       `json:"target"`
	Progress int       `json:"progress"`
	State    TaskState `json:"state"`
}

type TaskSystem struct {
	playerID int
	tasks    map[int]*Task
	bus      *event.Bus
}

func New(playerID int, bus *event.Bus) *TaskSystem {
	ts := &TaskSystem{
		playerID: playerID,
		tasks:    make(map[int]*Task),
		bus:      bus,
	}
	bus.Subscribe("item.added", ts.onItemAdded)
	return ts
}

func (ts *TaskSystem) onItemAdded(e event.Event) {
	playerID, _ := e.Data["playerID"].(int)
	if playerID != ts.playerID {
		return
	}
	itemID, _ := e.Data["itemID"].(int)

	taskMapping := map[int]int{
		2001: 3001,
		2002: 3002,
	}
	if tid, ok := taskMapping[itemID]; ok {
		ts.Progress(tid, 1)
	}
}

func (ts *TaskSystem) AddTask(taskID, target int) {
	ts.tasks[taskID] = &Task{
		TaskID: taskID,
		Target: target,
		State:  StateActive,
	}
	ts.bus.AppendLog(fmt.Sprintf("[Task] add task %d, target %d", taskID, target))
}

func (ts *TaskSystem) Progress(taskID, delta int) {
	t, ok := ts.tasks[taskID]
	if !ok || t.State == StateCompleted {
		return
	}
	t.Progress += delta
	ts.bus.AppendLog(fmt.Sprintf("[Task] trigger %d progress+%d (now %d/%d)", taskID, delta, t.Progress, t.Target))
	if t.Progress >= t.Target {
		t.Progress = t.Target
		t.State = StateCompleted
		ts.bus.AppendLog(fmt.Sprintf("[Task] task %d completed", taskID))
		ts.bus.Publish(event.Event{
			Type: "task.completed",
			Data: map[string]any{"playerID": ts.playerID, "taskID": taskID},
		})
	}
}

func (ts *TaskSystem) GetTask(taskID int) *Task {
	return ts.tasks[taskID]
}

func (ts *TaskSystem) AllTasks() []*Task {
	out := make([]*Task, 0, len(ts.tasks))
	for _, t := range ts.tasks {
		out = append(out, t)
	}
	return out
}
