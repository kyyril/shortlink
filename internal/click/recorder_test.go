package click_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kyyril/shortlink/internal/click"
)

type fakeWriter struct {
	mu      sync.Mutex
	flushed []click.Event
	flushCh chan int // sends count of items flushed
}

func newFakeWriter() *fakeWriter {
	return &fakeWriter{
		flushCh: make(chan int, 100),
	}
}

func (f *fakeWriter) InsertClicks(_ context.Context, events []click.Event) error {
	f.mu.Lock()
	f.flushed = append(f.flushed, events...)
	f.mu.Unlock()

	select {
	case f.flushCh <- len(events):
	default:
	}
	return nil
}

func (f *fakeWriter) totalFlushed() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.flushed)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

// AC-14: Full buffer drops and does not block.
func TestRecorder_FullBuffer_DropsWithoutBlocking(t *testing.T) {
	fw := newFakeWriter()
	// bufferSize = 2, batchSize = 10, long interval. Do NOT start worker yet.
	rec := click.NewRecorder(fw, 2, 10, time.Hour, discardLogger())

	// Push 5 events into buffer with capacity 2.
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		for range 5 {
			rec.Record(click.Event{LinkID: uuid.New(), ClickedAt: time.Now()})
		}
	}()

	select {
	case <-doneCh:
		// Succeeded without blocking
	case <-time.After(time.Second):
		t.Fatal("Record blocked on full buffer")
	}

	if rec.Dropped() != 3 {
		t.Errorf("dropped count = %d, want 3", rec.Dropped())
	}
}

func TestRecorder_FlushBySize(t *testing.T) {
	fw := newFakeWriter()
	batchSize := 5
	rec := click.NewRecorder(fw, 20, batchSize, time.Hour, discardLogger())
	rec.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = rec.Close(ctx)
	}()

	for range batchSize {
		rec.Record(click.Event{LinkID: uuid.New(), ClickedAt: time.Now()})
	}

	select {
	case count := <-fw.flushCh:
		if count != batchSize {
			t.Errorf("flushed count = %d, want %d", count, batchSize)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for batch flush by size")
	}
}

func TestRecorder_FlushByInterval(t *testing.T) {
	fw := newFakeWriter()
	interval := 20 * time.Millisecond
	rec := click.NewRecorder(fw, 20, 100, interval, discardLogger())
	rec.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = rec.Close(ctx)
	}()

	rec.Record(click.Event{LinkID: uuid.New(), ClickedAt: time.Now()})

	select {
	case count := <-fw.flushCh:
		if count != 1 {
			t.Errorf("flushed count = %d, want 1", count)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for flush by interval")
	}
}

func TestRecorder_DrainOnClose(t *testing.T) {
	fw := newFakeWriter()
	// Do not start immediate interval ticks
	rec := click.NewRecorder(fw, 50, 100, time.Hour, discardLogger())
	rec.Start()

	total := 15
	for range total {
		rec.Record(click.Event{LinkID: uuid.New(), ClickedAt: time.Now()})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := rec.Close(ctx); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if fw.totalFlushed() != total {
		t.Errorf("total flushed on drain = %d, want %d", fw.totalFlushed(), total)
	}

	// Subsequent calls after close are dropped
	rec.Record(click.Event{LinkID: uuid.New(), ClickedAt: time.Now()})
	if rec.Dropped() != 1 {
		t.Errorf("expected 1 dropped after close, got %d", rec.Dropped())
	}
}
