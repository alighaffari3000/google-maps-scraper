package exiter

import (
	"context"
	"sync"
)

type Exiter interface {
	SetSeedCount(int)
	SetCancelFunc(context.CancelFunc)
	IncrSeedCompleted(int)
	IncrSeedFailed(int)
	IncrPlacesFound(int)
	IncrPlacesCompleted(int)
	Progress() (placesFound, placesCompleted int)
	SeedFailures() int
	Run(context.Context)
}

type exiter struct {
	seedCount       int
	seedCompleted   int
	seedFailed      int
	placesFound     int
	placesCompleted int

	mu         *sync.Mutex
	cancelFunc context.CancelFunc
	doneCh     chan struct{}
}

func New() Exiter {
	return &exiter{
		mu:     &sync.Mutex{},
		doneCh: make(chan struct{}, 1),
	}
}

func (e *exiter) SetSeedCount(val int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.seedCount = val
}

func (e *exiter) SetCancelFunc(fn context.CancelFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.cancelFunc = fn
}

func (e *exiter) IncrSeedCompleted(val int) {
	e.mu.Lock()
	e.seedCompleted += val
	done := e.seedCompleted >= e.seedCount && e.placesCompleted >= e.placesFound
	e.mu.Unlock()

	if done {
		select {
		case e.doneCh <- struct{}{}:
		default:
		}
	}
}

// IncrSeedFailed records that a seed job ended in an error. It is tracked
// separately from completion because a failed seed still counts as completed
// for exit purposes, but means the results are incomplete rather than empty.
func (e *exiter) IncrSeedFailed(val int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.seedFailed += val
}

func (e *exiter) SeedFailures() int {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.seedFailed
}

func (e *exiter) IncrPlacesFound(val int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.placesFound += val
}

func (e *exiter) IncrPlacesCompleted(val int) {
	e.mu.Lock()
	e.placesCompleted += val
	done := e.seedCompleted >= e.seedCount && e.placesCompleted >= e.placesFound
	e.mu.Unlock()

	if done {
		select {
		case e.doneCh <- struct{}{}:
		default:
		}
	}
}

func (e *exiter) Progress() (placesFound, placesCompleted int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.placesFound, e.placesCompleted
}

func (e *exiter) Run(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-e.doneCh:
		if e.cancelFunc != nil {
			e.cancelFunc()
		}
	}
}
