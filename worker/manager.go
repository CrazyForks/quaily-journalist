package worker

import (
	"context"
	"sync"
)

// Manager starts and supervises a set of workers.
type Manager struct {
	workers []Worker
}

func NewManager(ws ...Worker) *Manager {
	return &Manager{workers: ws}
}

func (m *Manager) Start(ctx context.Context) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(m.workers))
	for _, w := range m.workers {
		wg.Add(1)
		go func(w Worker) {
			defer wg.Done()
			if err := w.Start(ctx); err != nil {
				errs <- err
			}
		}(w)
	}
	// Wait for context cancellation then wait for workers to exit.
	<-ctx.Done()
	wg.Wait()
	close(errs)
	// If any worker returned an error before context cancelled, report one.
	for err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
