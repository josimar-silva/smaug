package proxy

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

// --- Fakes ---

// fakeSleepSender records Sleep calls and returns a configurable error.
type fakeSleepSender struct {
	mu        sync.Mutex
	calls     int
	returnErr error
	// blockUntil, when non-nil, makes Sleep block until the channel is closed.
	blockUntil chan struct{}
}

func (f *fakeSleepSender) Sleep(_ context.Context, _ string) error {
	if f.blockUntil != nil {
		<-f.blockUntil
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++

	return f.returnErr
}

func (f *fakeSleepSender) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// newSleepCoordinatorTestLogger returns a quiet logger for sleep coordinator tests.
func newSleepCoordinatorTestLogger() *logger.Logger {
	return logger.New(logger.LevelError, logger.TEXT, nil)
}

// --- Construction tests ---

// TestNewSleepCoordinatorReturnsNilOnValidInput verifies that valid inputs produce a non-nil coordinator.
func TestNewSleepCoordinatorReturnsNilOnValidInput(t *testing.T) {
	// Given: valid inputs
	triggers := make(chan string, 1)
	sender := &fakeSleepSender{}
	log := newSleepCoordinatorTestLogger()

	// When: constructing the coordinator
	sc, err := NewSleepCoordinator((<-chan string)(triggers), sender, log)

	// Then: no error and non-nil coordinator
	require.NoError(t, err)
	assert.NotNil(t, sc)
}

// TestNewSleepCoordinatorRejectsNilTriggers verifies that a nil triggers channel is rejected.
func TestNewSleepCoordinatorRejectsNilTriggers(t *testing.T) {
	// Given: nil trigger channel
	sender := &fakeSleepSender{}
	log := newSleepCoordinatorTestLogger()

	// When
	sc, err := NewSleepCoordinator(nil, sender, log)

	// Then
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrSleepCoordinatorMissingTriggers))
	assert.Nil(t, sc)
}

// TestNewSleepCoordinatorRejectsNilSender verifies that a nil sleep sender is rejected.
func TestNewSleepCoordinatorRejectsNilSender(t *testing.T) {
	// Given: nil sender
	triggers := make(chan string, 1)
	log := newSleepCoordinatorTestLogger()

	// When
	sc, err := NewSleepCoordinator((<-chan string)(triggers), nil, log)

	// Then
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrSleepCoordinatorMissingSender))
	assert.Nil(t, sc)
}

// TestNewSleepCoordinatorRejectsNilLogger verifies that a nil logger is rejected.
func TestNewSleepCoordinatorRejectsNilLogger(t *testing.T) {
	// Given: nil logger
	triggers := make(chan string, 1)
	sender := &fakeSleepSender{}

	// When
	sc, err := NewSleepCoordinator((<-chan string)(triggers), sender, nil)

	// Then
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrSleepCoordinatorMissingLogger))
	assert.Nil(t, sc)
}

// --- Lifecycle tests ---

// TestSleepCoordinatorStartAndStopSucceeds verifies the happy-path Start/Stop lifecycle.
func TestSleepCoordinatorStartAndStopSucceeds(t *testing.T) {
	// Given: a valid coordinator
	triggers := make(chan string, 1)
	sc, err := NewSleepCoordinator((<-chan string)(triggers), &fakeSleepSender{}, newSleepCoordinatorTestLogger())
	require.NoError(t, err)

	// When: starting and stopping
	require.NoError(t, sc.Start(context.Background()))

	// Then: stopping succeeds
	assert.NoError(t, sc.Stop())
}

// TestSleepCoordinatorStartRejectsDoubleStart verifies that calling Start twice returns an error.
func TestSleepCoordinatorStartRejectsDoubleStart(t *testing.T) {
	// Given: an already-running coordinator
	triggers := make(chan string, 1)
	sc, err := NewSleepCoordinator((<-chan string)(triggers), &fakeSleepSender{}, newSleepCoordinatorTestLogger())
	require.NoError(t, err)
	require.NoError(t, sc.Start(context.Background()))
	t.Cleanup(func() { _ = sc.Stop() })

	// When: calling Start again
	err = sc.Start(context.Background())

	// Then: error returned
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrSleepCoordinatorAlreadyRunning))
}

// TestSleepCoordinatorStopRejectsStopWhenNotRunning verifies that Stop on an unstarted coordinator returns an error.
func TestSleepCoordinatorStopRejectsStopWhenNotRunning(t *testing.T) {
	// Given: a coordinator that has never been started
	triggers := make(chan string, 1)
	sc, err := NewSleepCoordinator((<-chan string)(triggers), &fakeSleepSender{}, newSleepCoordinatorTestLogger())
	require.NoError(t, err)

	// When: stopping without starting
	err = sc.Stop()

	// Then: error returned
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrSleepCoordinatorNotRunning))
}

// --- Trigger tests ---

// TestSleepCoordinatorCallsSenderOnTrigger verifies that the coordinator calls the
// sleep sender exactly once when a trigger arrives on the channel.
func TestSleepCoordinatorCallsSenderOnTrigger(t *testing.T) {
	// Given: a coordinator with a fake sender
	triggers := make(chan string, 4)
	sender := &fakeSleepSender{}
	sc, err := NewSleepCoordinator((<-chan string)(triggers), sender, newSleepCoordinatorTestLogger())
	require.NoError(t, err)
	require.NoError(t, sc.Start(context.Background()))
	t.Cleanup(func() { _ = sc.Stop() })

	// When: a trigger is sent
	triggers <- "route-a"

	// Then: the sender is called within a reasonable time
	assert.Eventually(t, func() bool {
		return sender.callCount() == 1
	}, 500*time.Millisecond, 5*time.Millisecond, "expected Sleep to be called once")
}

// TestSleepCoordinatorCallsSenderForEachTrigger verifies that the coordinator calls the
// sleep sender once per trigger received, preserving per-trigger semantics.
func TestSleepCoordinatorCallsSenderForEachTrigger(t *testing.T) {
	// Given: a coordinator with a fake sender
	triggers := make(chan string, 8)
	sender := &fakeSleepSender{}
	sc, err := NewSleepCoordinator((<-chan string)(triggers), sender, newSleepCoordinatorTestLogger())
	require.NoError(t, err)
	require.NoError(t, sc.Start(context.Background()))
	t.Cleanup(func() { _ = sc.Stop() })

	// When: multiple triggers are sent (one per route to simulate distinct idle cycles)
	const triggerCount = 3
	for i := range triggerCount {
		triggers <- "route-" + string(rune('a'+i))
	}

	// Then: the sender is called once for each trigger
	assert.Eventually(t, func() bool {
		return sender.callCount() == triggerCount
	}, 500*time.Millisecond, 5*time.Millisecond, "expected Sleep to be called for each trigger")
}

// TestSleepCoordinatorHandlesSenderErrorGracefully verifies that a Sleep API error
// does not crash or block the coordinator — it logs the error and continues processing.
func TestSleepCoordinatorHandlesSenderErrorGracefully(t *testing.T) {
	// Given: a sender that always returns an error
	triggers := make(chan string, 8)
	sender := &fakeSleepSender{returnErr: errors.New("sleep API unavailable")}
	sc, err := NewSleepCoordinator((<-chan string)(triggers), sender, newSleepCoordinatorTestLogger())
	require.NoError(t, err)
	require.NoError(t, sc.Start(context.Background()))
	t.Cleanup(func() { _ = sc.Stop() })

	// When: two triggers arrive despite the first call failing
	triggers <- "route-a"
	triggers <- "route-b"

	// Then: the sender is called for both triggers (error did not block the loop)
	assert.Eventually(t, func() bool {
		return sender.callCount() == 2
	}, 500*time.Millisecond, 5*time.Millisecond, "expected both triggers to be processed despite errors")
}

// TestSleepCoordinatorStopsProcessingAfterStop verifies that no new triggers are
// processed after Stop() returns.
func TestSleepCoordinatorStopsProcessingAfterStop(t *testing.T) {
	// Given: a coordinator that uses a blocking sender to hold the loop occupied
	triggers := make(chan string, 8)
	blockCh := make(chan struct{})
	sender := &fakeSleepSender{blockUntil: blockCh}

	sc, err := NewSleepCoordinator((<-chan string)(triggers), sender, newSleepCoordinatorTestLogger())
	require.NoError(t, err)
	require.NoError(t, sc.Start(context.Background()))

	// Enqueue one trigger that will block the sender, then stop.
	triggers <- "route-a"

	// Give the goroutine a moment to pick up the trigger and block in Sleep.
	time.Sleep(20 * time.Millisecond)

	// Unblock the sender so Stop() can complete.
	close(blockCh)
	require.NoError(t, sc.Stop())

	// Capture the call count immediately after Stop.
	countAfterStop := sender.callCount()

	// When: another trigger is enqueued after Stop
	triggers <- "route-b"
	time.Sleep(20 * time.Millisecond)

	// Then: the call count did not increase after Stop
	assert.Equal(t, countAfterStop, sender.callCount(), "no triggers must be processed after Stop")
}

// TestSleepCoordinatorContextCancellationStopsLoop verifies that cancelling the
// context passed to Start causes the background loop to exit.
func TestSleepCoordinatorContextCancellationStopsLoop(t *testing.T) {
	// Given: a coordinator started with a cancellable context
	triggers := make(chan string, 1)
	var callCount atomic.Int32
	sender := &fakeSleepSenderFunc{fn: func(_ context.Context) error {
		callCount.Add(1)
		return nil
	}}

	sc, err := NewSleepCoordinator((<-chan string)(triggers), sender, newSleepCoordinatorTestLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, sc.Start(ctx))

	// When: context is cancelled
	cancel()

	// Then: Stop should return without hanging
	done := make(chan error, 1)
	go func() { done <- sc.Stop() }()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Stop() did not return after context cancellation")
	}
}

// TestSleepCoordinatorExitsWhenTriggerChannelClosed verifies that the coordinator
// exits its loop cleanly when the triggers channel is closed by the producer.
func TestSleepCoordinatorExitsWhenTriggerChannelClosed(t *testing.T) {
	// Given: a coordinator backed by a channel that the producer will close
	triggers := make(chan string, 1)
	sender := &fakeSleepSender{}

	sc, err := NewSleepCoordinator((<-chan string)(triggers), sender, newSleepCoordinatorTestLogger())
	require.NoError(t, err)
	require.NoError(t, sc.Start(context.Background()))

	// When: the triggers channel is closed (simulating IdleTracker shutdown)
	close(triggers)

	// Then: Stop should still succeed (the loop already exited)
	done := make(chan error, 1)
	go func() { done <- sc.Stop() }()

	select {
	case err := <-done:
		// Either the coordinator already stopped (ErrSleepCoordinatorNotRunning) or
		// Stop succeeded normally after the loop exited on channel close.
		if err != nil && !errors.Is(err, ErrSleepCoordinatorNotRunning) {
			t.Fatalf("unexpected error from Stop: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Stop() did not return after triggers channel was closed")
	}
}

// fakeSleepSenderFunc is a function-based fake for the SleepSender interface.
type fakeSleepSenderFunc struct {
	fn func(ctx context.Context) error
}

func (f *fakeSleepSenderFunc) Sleep(ctx context.Context, _ string) error {
	return f.fn(ctx)
}
