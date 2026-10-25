package server

import (
	"context"
	"testing"
	"time"
)

func TestWorkerPoolResizeWhileBusy(t *testing.T) {
	var p workerPool
	start := func(int, *workerSlot) bool { return true }
	active := func() int {
		n := 0
		for _, slot := range p.slots {
			if !slot.stopping {
				n++
			}
		}
		return n
	}
	p.resize(5, start)
	p.resize(20, start)
	if len(p.slots) != 20 || active() != 20 {
		t.Fatal("existing pool failed to grow")
	}
	p.resize(5, start)
	if len(p.slots) != 20 || active() != 5 {
		t.Fatal("shrink must retire exactly the excess, without interrupting work")
	}
	p.resize(5, start)
	if active() != 5 {
		t.Fatal("repeated resize retired additional workers")
	}
	p.resize(10, start)
	if len(p.slots) != 20 || active() != 5 {
		t.Fatal("draining work must count toward the concurrency limit")
	}
	for _, slot := range p.slots {
		if slot.stopping {
			close(slot.done)
		}
	}
	p.resize(10, start)
	if len(p.slots) != 10 || active() != 10 {
		t.Fatal("pool failed to converge after work drained")
	}
	p.resize(0, start)
	if active() != 0 {
		t.Fatal("zero limit failed to retire all workers")
	}
}

func TestWorkerPoolRejectsStartupDuringShutdown(t *testing.T) {
	var p workerPool
	p.resize(20, func(int, *workerSlot) bool { return false })
	if len(p.slots) != 0 {
		t.Fatal("failed admission registered a phantom worker")
	}
}

func TestWorkerSettingCacheDoesNotShrinkOnReadFailure(t *testing.T) {
	s := newProxyFixture(t)
	if err := s.m.SetWorkers(20); err != nil {
		t.Fatal(err)
	}
	if got := s.m.Workers(); got != 20 {
		t.Fatal(got)
	}
	s.m.workersReadAt = time.Time{}
	if err := s.m.pg.Close(); err != nil {
		t.Fatal(err)
	}
	if got := s.m.Workers(); got != 20 {
		t.Fatal("database failure changed the last configured worker count", got)
	}
	if err := s.m.SetWorkers(5); err == nil {
		t.Fatal("failed persistence was reported as saved")
	}
	if got := s.m.Workers(); got != 20 {
		t.Fatal("failed setting write changed concurrency", got)
	}
}

func TestWorkerPoolDynamicAdmissionDrainsOnTaskStop(t *testing.T) {
	s := newProxyFixture(t)
	e := s.engine
	task := &Task{ID: "worker-pool-fixture"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.deleteMu.RLock()
	rt := e.registerTaskRoutines(ctx, task.ID, 1)
	e.deleteMu.RUnlock()
	runTaskRoutine(rt, func(ctx context.Context) { e.workerPoolLoop(ctx, task, rt) })
	for _, n := range []int{5, 20, 5} {
		if err := s.m.SetWorkers(n); err != nil {
			t.Fatal(err)
		}
		time.Sleep(550 * time.Millisecond)
	}
	done := make(chan struct{})
	go func() { e.StopTask(task.ID); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("task stop did not drain the dynamic worker pool")
	}
	e.runtimeMu.Lock()
	defer e.runtimeMu.Unlock()
	if e.runtimes[task.ID] != nil {
		t.Fatal("deleted task retained worker runtime")
	}
}
