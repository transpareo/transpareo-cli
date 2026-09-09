package transpareo

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitForTaskPollsWithGrowingWaits(t *testing.T) {
	ts := newTokenServer(t)
	statuses := []string{"pending", "running", "running", "running",
		"running", "running", "running", "completed"}
	var polls atomic.Int32
	var reloads atomic.Int32
	ts.Mux.HandleFunc("/api/exports/42", func(w http.ResponseWriter,
		r *http.Request) {
		if r.URL.Query().Get("reload") == "1" {
			reloads.Add(1)
		}
		i := int(polls.Add(1)) - 1
		writeJSON(w, 200, map[string]any{"status": statuses[i],
			"progress": i * 10,
			"id":       42})
	})
	clock := newFakeClock()
	c := newTestClient(t, ts, clock)
	var seen []string
	task, err := c.WaitForTask(context.Background(), ts.URL+"/api/exports/42",
		&WaitOptions{OnPoll: func(t *Task) { seen = append(seen, t.Status) }})
	if err != nil {
		t.Fatal(err)
	}
	if !task.Done() || task.Failed() || task.Status != "completed" {
		t.Errorf("task = %+v", task)
	}
	if len(seen) != len(statuses) || reloads.Load() != int32(len(statuses)) {
		t.Errorf("seen %v, reloads %d", seen, reloads.Load())
	}
	want := fmt.Sprint([]time.Duration{time.Second, 2 * time.Second,
		4 * time.Second,
		8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second})
	if fmt.Sprint(clock.waits) != want {
		t.Errorf("waits = %v, want %s", clock.waits, want)
	}
	if string(task.Body) == "" || task.URL == "" {
		t.Error("body and URL must be kept")
	}
}

func TestWaitForTaskReturnsFailedWithoutError(t *testing.T) {
	ts := newTokenServer(t)
	ts.Mux.HandleFunc("/api/dpps/bulk/1", func(w http.ResponseWriter,
		r *http.Request) {
		writeJSON(w, 200, map[string]any{"status": "failed"})
	})
	c := newTestClient(t, ts, newFakeClock())
	task, err := c.WaitForTask(context.Background(), "/dpps/bulk/1", nil)
	if err != nil || !task.Failed() {
		t.Errorf("task = %+v, err = %v", task, err)
	}
}

func TestWaitForTaskStopsWhenContextEnds(t *testing.T) {
	ts := newTokenServer(t)
	ts.Mux.HandleFunc("/api/imports/1", func(w http.ResponseWriter,
		r *http.Request) {
		writeJSON(w, 200, map[string]any{"status": "validating"})
	})
	clock := newFakeClock()
	c := newTestClient(t, ts, clock)
	ctx, cancel := context.WithCancel(context.Background())
	c.sleep = func(ctx context.Context, d time.Duration) error {
		cancel()
		return ctx.Err()
	}
	task, err := c.WaitForTask(ctx, "/imports/1", nil)
	if err != context.Canceled || task == nil || task.Status != "validating" {
		t.Errorf("task = %+v, err = %v", task, err)
	}
}

func TestWaitForTaskReportsErrors(t *testing.T) {
	ts := newTokenServer(t)
	ts.Mux.HandleFunc("/api/imports/1", func(w http.ResponseWriter,
		r *http.Request) {
		writeJSON(w, 404, map[string]any{"error": "IMPORT_NOT_FOUND",
			"message": "gone"})
	})
	c := newTestClient(t, ts, newFakeClock())
	if _, err := c.WaitForTask(context.Background(), "/imports/1",
		nil); !IsCode(err, "IMPORT_NOT_FOUND") {
		t.Errorf("err = %v", err)
	}
}

func TestTaskDone(t *testing.T) {
	for status, done := range map[string]bool{
		"pending": false, "running": false, "validating": false,
		"importing": false,
		"restoring": false, "completed": true, "failed": true,
		"cancelled": true,
		"validated": true, "reverted": true, "mapped": true, "": true,
	} {
		if (&Task{Status: status}).Done() != done {
			t.Errorf("%q: done should be %v", status, done)
		}
	}
}
