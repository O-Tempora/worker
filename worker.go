package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/O-Tempora/worker/safe"
)

// Worker periodically runs given task.
type Worker struct {
	// function which worker runs over time.
	task Task

	// time between finishing N-th task and running N+1-th task.
	delay time.Duration
	// time between finishing N-th task and running N+1-th task if N-th task returned error.
	onErrDelay time.Duration

	// timeout for worker's task.
	runTimeout time.Duration

	// provides current time for worker.
	tp TimeProvider

	// time interval in which worker can run its task.
	// If equals nil - task can be run whenever worker is ready.
	ti *TimeInterval
}

// Option is a constructor option for worker.
type Option func(*Worker)

// WithDelay sets worker's delay.
func WithDelay(d time.Duration) Option {
	return func(w *Worker) { w.delay = d }
}

// WithOnErrDelay sets worker's onErrDelay.
func WithOnErrDelay(d time.Duration) Option {
	return func(w *Worker) { w.onErrDelay = d }
}

// WithCurrentTimeProvider sets worker's current time provider.
func WithCurrentTimeProvider(tp TimeProvider) Option {
	return func(w *Worker) { w.tp = tp }
}

// WithTaskRunTimeInterval sets worker's task run time interval.
func WithTaskRunTimeInterval(from, to Time) Option {
	return func(w *Worker) {
		ti := NewTimeInterval(from, to)
		w.ti = &ti
	}
}

// WithRunTimeout sets timeout for worker's task.
func WithRunTimeout(tm time.Duration) Option {
	return func(w *Worker) {
		w.runTimeout = tm
	}
}

// New creates new Worker.
//
// It is highly recommended to specify most of the options manualy in this constructor,
// since provided defaults may be insuffitient or inadequate in some specific usecases.
func New(task Task, opts ...Option) *Worker {
	w := newWorker(task)
	for i := range opts {
		opts[i](w)
	}
	return w
}

func newWorker(task Task) *Worker {
	return &Worker{
		task:       task,
		delay:      DefaultDelay,
		onErrDelay: DefaultOnErrDelay,
		runTimeout: DefaultDelay,
		tp:         DefaultTimeProvider,
		ti:         nil,
	}
}

// StartBackgroundWorker starts worker's background loop of executing given task.
func StartBackgroundWorker(ctx context.Context, w *Worker) error {
	if err := w.validate(); err != nil {
		return fmt.Errorf("worker validate: %w", err)
	}

	safe.Go(ctx, w.run)

	return nil
}

func (w *Worker) validate() error {
	if w == nil {
		return fmt.Errorf("worker is nil")
	}

	if w.task == nil {
		return fmt.Errorf("task is nil")
	}

	return nil
}

func (w *Worker) run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	t := time.NewTimer(w.delay)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if !w.isAllowedToRun(ctx) {
				t.Reset(w.delay)
				continue
			}

			childCtx := context.WithoutCancel(ctx)
			if err := w.runTask(childCtx); err != nil {
				t.Reset(w.onErrDelay)
			}
			t.Reset(w.delay)
		}
	}
}

func (w *Worker) runTask(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, w.runTimeout)
	defer cancel()

	return w.task(ctx)
}

func (w *Worker) isAllowedToRun(ctx context.Context) bool {
	if w.ti == nil {
		return true
	}

	return w.ti.IsInInterval(w.tp(ctx))
}
