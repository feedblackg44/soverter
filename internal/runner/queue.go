package runner

import (
	"context"
	"sync"
	"time"

	"vidpreset/internal/preset"
)

type Status int

const (
	Pending Status = iota
	Running
	Done
	Failed
	Canceled
)

type Item struct {
	Preset   preset.Preset
	File     string
	Status   Status
	Output   string
	Err      error
	Progress Progress
	Total    time.Duration
}

// Queue processes items sequentially; Add may be called at any time.
type Queue struct {
	Tools    Tools
	OnChange func() // called from worker goroutine after any state change

	mu     sync.Mutex
	items  []*Item
	cancel context.CancelFunc
	wake   chan struct{}
	start  sync.Once
}

func NewQueue(t Tools, onChange func()) *Queue {
	return &Queue{Tools: t, OnChange: onChange, wake: make(chan struct{}, 1)}
}

func (q *Queue) Add(p preset.Preset, files ...string) {
	q.mu.Lock()
	for _, f := range files {
		q.items = append(q.items, &Item{Preset: p, File: f})
	}
	q.mu.Unlock()
	q.start.Do(func() { go q.loop() })
	select {
	case q.wake <- struct{}{}:
	default:
	}
	q.changed()
}

// Snapshot returns copies of all items.
func (q *Queue) Snapshot() []Item {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Item, len(q.items))
	for i, it := range q.items {
		out[i] = *it
	}
	return out
}

// CancelCurrent stops the running item (its partial output is deleted).
func (q *Queue) CancelCurrent() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.cancel != nil {
		q.cancel()
	}
}

func (q *Queue) changed() {
	if q.OnChange != nil {
		q.OnChange()
	}
}

func (q *Queue) next() *Item {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, it := range q.items {
		if it.Status == Pending {
			it.Status = Running
			return it
		}
	}
	return nil
}

func (q *Queue) loop() {
	for {
		it := q.next()
		if it == nil {
			<-q.wake
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		q.mu.Lock()
		q.cancel = cancel
		q.mu.Unlock()
		q.changed()

		total := q.Tools.Duration(ctx, it.File)
		q.mu.Lock()
		it.Total = total
		q.mu.Unlock()
		out, err := q.Tools.Run(ctx, it.Preset, it.File, func(p Progress) {
			q.mu.Lock()
			it.Progress = p
			q.mu.Unlock()
			q.changed()
		})
		cancel()
		q.mu.Lock()
		q.cancel = nil
		it.Output, it.Err = out, err
		switch {
		case err == nil:
			it.Status = Done
		case err == ErrCanceled:
			it.Status = Canceled
		default:
			it.Status = Failed
		}
		q.mu.Unlock()
		q.changed()
	}
}

// Idle reports whether nothing is pending or running.
func (q *Queue) Idle() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, it := range q.items {
		if it.Status == Pending || it.Status == Running {
			return false
		}
	}
	return true
}
