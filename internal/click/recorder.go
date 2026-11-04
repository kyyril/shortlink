package click

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kyyril/shortlink/internal/httpx"
)

// RepositoryWriter defines the write operation needed by the recorder.
type RepositoryWriter interface {
	InsertClicks(ctx context.Context, events []Event) error
}

// Recorder batches click events and flushes them periodically or on threshold.
// Calls to Record never block the caller.
type Recorder struct {
	repo      RepositoryWriter
	ch        chan Event
	stop      chan struct{}
	done      chan struct{}
	closed    atomic.Bool
	closeOnce sync.Once
	dropped   atomic.Uint64
	batchSize int
	interval  time.Duration
	logger    *slog.Logger
}

// NewRecorder returns an initialized Recorder ready to Start.
func NewRecorder(repo RepositoryWriter, bufferSize, batchSize int, interval time.Duration, logger *slog.Logger) *Recorder {
	if bufferSize < 1 {
		bufferSize = 1024
	}
	if batchSize < 1 {
		batchSize = 100
	}
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Recorder{
		repo:      repo,
		ch:        make(chan Event, bufferSize),
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
		batchSize: batchSize,
		interval:  interval,
		logger:    logger,
	}
}

// Start spawns the background worker goroutine.
func (r *Recorder) Start() {
	go r.run()
}

// Record queues an event non-blockingly. If the buffer is full or recorder closed,
// the event is dropped and the drop counter is incremented.
func (r *Recorder) Record(e Event) {
	if r.closed.Load() {
		r.dropped.Add(1)
		httpx.ClickDropsTotal.Inc()
		return
	}

	select {
	case r.ch <- e:
	default:
		r.dropped.Add(1)
		httpx.ClickDropsTotal.Inc()
	}
}

// Dropped returns the number of click events dropped due to a full buffer or close.
func (r *Recorder) Dropped() uint64 {
	return r.dropped.Load()
}

func (r *Recorder) run() {
	defer close(r.done)

	batch := make([]Event, 0, r.batchSize)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := r.repo.InsertClicks(ctx, batch); err != nil {
			r.logger.Error("flush clicks failed", "error", err, "count", len(batch))
		}
		batch = batch[:0]
	}

	for {
		select {
		case e := <-r.ch:
			batch = append(batch, e)
			if len(batch) >= r.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-r.stop:
			for {
				select {
				case e := <-r.ch:
					batch = append(batch, e)
				default:
					flush()
					return
				}
			}
		}
	}
}

// Close stops the recorder and drains buffered events.
// Close is owned by app and must outlive the HTTP server during shutdown.
func (r *Recorder) Close(ctx context.Context) error {
	r.closeOnce.Do(func() {
		r.closed.Store(true)
		close(r.stop)
	})

	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
