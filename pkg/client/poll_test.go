package client

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/figchain/go-client/pkg/config"
	"github.com/figchain/go-client/pkg/model"
	"github.com/figchain/go-client/pkg/transport"
)

// fakeTransport records the wait of each update request and answers from a
// script of errors (nil means an empty successful response). It closes the
// client once the script is exhausted.
type fakeTransport struct {
	transport.Transport
	mu      sync.Mutex
	waits   []time.Duration
	script  []error
	onDone  func()
	stopped bool
}

func (f *fakeTransport) FetchUpdateWait(_ context.Context, _ *model.UpdateFetchRequest, wait time.Duration) (*model.UpdateFetchResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.waits = append(f.waits, wait)
	i := len(f.waits) - 1
	if i == len(f.script)-1 && !f.stopped {
		f.stopped = true
		f.onDone()
	}
	if i < len(f.script) && f.script[i] != nil {
		return nil, f.script[i]
	}
	return &model.UpdateFetchResponse{}, nil
}

func runPoller(t *testing.T, useLongPolling bool, script []error) []time.Duration {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &Client{
		cfg:              &config.Config{UseLongPolling: useLongPolling, PollingInterval: time.Millisecond},
		namespaceCursors: map[string]string{"ns": ""},
		closeCh:          make(chan struct{}),
		ctx:              ctx,
	}
	ft := &fakeTransport{script: script, onDone: func() { close(c.closeCh) }}
	c.transport = ft

	done := make(chan struct{})
	go func() { c.pollNamespace("ns"); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("poller did not stop")
	}
	return ft.waits
}

func TestPollNamespace_LongPollingAsksServerToWait(t *testing.T) {
	waits := runPoller(t, true, []error{nil, nil, nil})
	for i, w := range waits {
		if w != longPollWait {
			t.Errorf("request %d: wait %v, want %v", i, w, longPollWait)
		}
	}
}

func TestPollNamespace_PollingDoesNotWait(t *testing.T) {
	waits := runPoller(t, false, []error{nil, nil, nil})
	for i, w := range waits {
		if w != 0 {
			t.Errorf("request %d: wait %v, want 0", i, w)
		}
	}
}

func TestPollNamespace_FallsBackAfterRepeatedTransitFailures(t *testing.T) {
	waits := runPoller(t, true, []error{
		context.DeadlineExceeded, context.DeadlineExceeded, context.DeadlineExceeded, nil, nil,
	})
	want := []time.Duration{longPollWait, longPollWait, longPollWait, 0, 0}
	if len(waits) < len(want) {
		t.Fatalf("got %d requests, want %d", len(waits), len(want))
	}
	for i := range want {
		if waits[i] != want[i] {
			t.Errorf("request %d: wait %v, want %v", i, waits[i], want[i])
		}
	}
}

func TestPollNamespace_ServerErrorsDoNotTriggerFallback(t *testing.T) {
	serverErr := &transport.StatusError{StatusCode: 500}
	waits := runPoller(t, true, []error{serverErr, serverErr, serverErr, serverErr, nil})
	for i, w := range waits {
		if w != longPollWait {
			t.Errorf("request %d: wait %v, want %v (server errors must not disable long polling)", i, w, longPollWait)
		}
	}
}

func TestIsTransitFailure(t *testing.T) {
	if isTransitFailure(&transport.StatusError{StatusCode: 502}) {
		t.Error("status errors are not transit failures")
	}
	if !isTransitFailure(context.DeadlineExceeded) {
		t.Error("deadline exceeded is a transit failure")
	}
	if isTransitFailure(errors.New("connection refused")) {
		t.Error("unrelated errors are not transit failures")
	}
}
