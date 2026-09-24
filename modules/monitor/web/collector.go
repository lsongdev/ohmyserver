package web

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	monitor "github.com/lsongdev/ohmyserver/modules/monitor"
)

const (
	defaultCollectionInterval = 3 * time.Second
	statsRetention            = 7 * 24 * time.Hour
)

// HistoricalCollector collects samples from hosts managed by another module.
// The host application can provide an adapter without coupling Monitor to it.
type HistoricalCollector interface {
	CollectHistoricalStats(context.Context) ([]monitor.Stats, error)
}

type statsWriter interface {
	Insert(context.Context, *Stats) error
	PruneBefore(context.Context, time.Time) (int64, error)
}

// CollectionWorker owns Monitor's periodic collection and retention policy.
type CollectionWorker struct {
	repo     statsWriter
	local    Collector
	remote   HistoricalCollector
	interval time.Duration
}

// CollectionService owns the background lifecycle around Monitor collection.
// The host supplies the process context and any cross-module collectors.
type CollectionService struct {
	parent context.Context
	worker *CollectionWorker
	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
	closed bool
}

func NewCollectionService(parent context.Context, repo *Repository, local Collector, remote HistoricalCollector, interval time.Duration) *CollectionService {
	return &CollectionService{parent: parent, worker: NewCollectionWorker(repo, local, remote, interval)}
}

func (s *CollectionService) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("monitor collection service is closed")
	}
	if s.done != nil {
		return errors.New("monitor collection service is already started")
	}
	if err := s.parent.Err(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(s.parent)
	s.cancel = cancel
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		s.worker.Run(ctx)
	}()
	return nil
}

func (s *CollectionService) Close() error {
	s.mu.Lock()
	s.closed = true
	cancel, done := s.cancel, s.done
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	return nil
}

func NewCollectionWorker(repo *Repository, local Collector, remote HistoricalCollector, interval time.Duration) *CollectionWorker {
	if interval <= 0 {
		interval = defaultCollectionInterval
	}
	return &CollectionWorker{repo: repo, local: local, remote: remote, interval: interval}
}

func (w *CollectionWorker) CollectOnce(ctx context.Context) error {
	var failures []error
	if w.local != nil {
		stats, err := w.local.CollectMonitorStats(ctx)
		if err != nil {
			failures = append(failures, err)
		} else if stats != nil {
			if err := w.repo.Insert(ctx, stats); err != nil {
				failures = append(failures, err)
			}
		}
	}
	if w.remote != nil {
		items, err := w.remote.CollectHistoricalStats(ctx)
		if err != nil {
			failures = append(failures, err)
		}
		for i := range items {
			if err := w.repo.Insert(ctx, &items[i]); err != nil {
				failures = append(failures, err)
			}
		}
	}
	return errors.Join(failures...)
}

func (w *CollectionWorker) Run(ctx context.Context) {
	collectionTicker := time.NewTicker(w.interval)
	cleanupTicker := time.NewTicker(time.Hour)
	defer collectionTicker.Stop()
	defer cleanupTicker.Stop()
	if err := w.CollectOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("monitor collection: %v", err)
	}
	for {
		select {
		case <-collectionTicker.C:
			if err := w.CollectOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("monitor collection: %v", err)
			}
		case <-cleanupTicker.C:
			deleted, err := w.repo.PruneBefore(ctx, time.Now().Add(-statsRetention))
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("monitor stats cleanup: %v", err)
			} else if deleted > 0 {
				log.Printf("cleaned up %d old monitor stats records", deleted)
			}
		case <-ctx.Done():
			return
		}
	}
}
