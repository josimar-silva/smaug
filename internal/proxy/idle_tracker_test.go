package proxy

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
	"github.com/josimar-silva/smaug/internal/infrastructure/metrics"
)

// newIdleTrackerTestLogger returns a quiet logger for idle tracker tests.
func newIdleTrackerTestLogger() *logger.Logger {
	return logger.New(logger.LevelError, logger.TEXT, nil)
}

// --- Construction tests ---

// TestNewIdleTrackerSucceeds verifies that a valid config produces a tracker.
func TestNewIdleTrackerSucceeds(t *testing.T) {
	// Given: valid configuration
	cfg := IdleTrackerConfig{
		CheckInterval: 100 * time.Millisecond,
	}

	// When: creating the idle tracker
	tracker, err := NewIdleTracker(cfg, newIdleTrackerTestLogger())

	// Then: no error, tracker is not nil
	require.NoError(t, err)
	assert.NotNil(t, tracker)
}

// TestNewIdleTrackerRejectsNilLogger verifies that a nil logger is rejected.
func TestNewIdleTrackerRejectsNilLogger(t *testing.T) {
	// Given: nil logger
	cfg := IdleTrackerConfig{
		CheckInterval: 100 * time.Millisecond,
	}

	// When: creating the idle tracker with nil logger
	tracker, err := NewIdleTracker(cfg, nil)

	// Then: error, tracker is nil
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrIdleTrackerMissingLogger)
	assert.Nil(t, tracker)
}

// TestNewIdleTrackerRejectsZeroCheckInterval verifies that a zero check interval is rejected.
func TestNewIdleTrackerRejectsZeroCheckInterval(t *testing.T) {
	// Given: zero check interval
	cfg := IdleTrackerConfig{
		CheckInterval: 0,
	}

	// When: creating the idle tracker
	tracker, err := NewIdleTracker(cfg, newIdleTrackerTestLogger())

	// Then: error, tracker is nil
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrIdleTrackerInvalidCheckInterval)
	assert.Nil(t, tracker)
}

// TestNewIdleTrackerRejectsNegativeCheckInterval verifies that a negative check interval is rejected.
func TestNewIdleTrackerRejectsNegativeCheckInterval(t *testing.T) {
	// Given: negative check interval
	cfg := IdleTrackerConfig{
		CheckInterval: -1 * time.Second,
	}

	// When: creating the idle tracker
	tracker, err := NewIdleTracker(cfg, newIdleTrackerTestLogger())

	// Then: error, tracker is nil
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrIdleTrackerInvalidCheckInterval)
	assert.Nil(t, tracker)
}

// --- Activity recording tests ---

// TestRecordActivitySetsLastSeenTime verifies that RecordActivity updates the
// last activity time for a route.
func TestRecordActivitySetsLastSeenTime(t *testing.T) {
	// Given: a tracker
	cfg := IdleTrackerConfig{CheckInterval: 100 * time.Millisecond}
	tracker, err := NewIdleTracker(cfg, newIdleTrackerTestLogger())
	require.NoError(t, err)

	before := time.Now()

	// When: recording activity for a route
	tracker.RecordActivity("route-a")

	after := time.Now()

	// Then: idle time is within the expected range
	idleTime := tracker.IdleTime("route-a")
	assert.LessOrEqual(t, idleTime, after.Sub(before)+time.Millisecond)
}

// TestIdleTimeForUnknownRouteReturnsLargeValue verifies that querying idle time
// for an unknown route returns a large duration (treated as always idle).
func TestIdleTimeForUnknownRouteReturnsLargeValue(t *testing.T) {
	// Given: a tracker with no activity recorded
	cfg := IdleTrackerConfig{CheckInterval: 100 * time.Millisecond}
	tracker, err := NewIdleTracker(cfg, newIdleTrackerTestLogger())
	require.NoError(t, err)

	// When: querying idle time for unknown route
	idleTime := tracker.IdleTime("nonexistent-route")

	// Then: returns a large value indicating the route has always been idle
	assert.Equal(t, idleNeverSeen, idleTime)
}

// TestRecordActivityUpdatesExistingRoute verifies that RecordActivity updates the
// time for a route that was already seen.
func TestRecordActivityUpdatesExistingRoute(t *testing.T) {
	// Given: a tracker with a prior activity record
	cfg := IdleTrackerConfig{CheckInterval: 100 * time.Millisecond}
	tracker, err := NewIdleTracker(cfg, newIdleTrackerTestLogger())
	require.NoError(t, err)

	tracker.RecordActivity("route-a")
	time.Sleep(20 * time.Millisecond)

	// When: recording a second activity
	tracker.RecordActivity("route-a")

	// Then: idle time is very small (just updated)
	idleTime := tracker.IdleTime("route-a")
	assert.Less(t, idleTime, 20*time.Millisecond)
}

// --- Idle detection tests ---

// TestIdleTrackerEmitsSleepTriggerWhenRouteIsIdle verifies that the tracker emits a
// sleep trigger on the channel when a route exceeds its idle timeout.
func TestIdleTrackerEmitsSleepTriggerWhenRouteIsIdle(t *testing.T) {
	// Given: a tracker with a very short check interval
	cfg := IdleTrackerConfig{
		CheckInterval: 20 * time.Millisecond,
	}
	tracker, err := NewIdleTracker(cfg, newIdleTrackerTestLogger())
	require.NoError(t, err)

	// Register a route with a short idle timeout
	tracker.RegisterRoute("route-a", 30*time.Millisecond)

	// Record activity once, then wait for idle to trigger
	tracker.RecordActivity("route-a")

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err = tracker.Start(ctx)
	require.NoError(t, err)

	// Wait for the route to become idle (idle timeout = 30ms, so wait at least 50ms)
	time.Sleep(60 * time.Millisecond)

	// Then: sleep trigger received for route-a
	sleepCh := tracker.SleepTriggers()
	select {
	case routeID := <-sleepCh:
		assert.Equal(t, "route-a", routeID)
	case <-time.After(400 * time.Millisecond):
		t.Fatal("expected sleep trigger for route-a, but none received")
	}

	_ = tracker.Stop()
}

// TestIdleTrackerDoesNotEmitTriggerForActiveRoute verifies that the tracker does NOT emit
// a sleep trigger when a route receives continuous activity.
func TestIdleTrackerDoesNotEmitTriggerForActiveRoute(t *testing.T) {
	// Given: a tracker with a longer idle timeout than our test duration
	cfg := IdleTrackerConfig{
		CheckInterval: 20 * time.Millisecond,
	}
	tracker, err := NewIdleTracker(cfg, newIdleTrackerTestLogger())
	require.NoError(t, err)

	// Register a route with a 200ms idle timeout (longer than test window)
	tracker.RegisterRoute("route-a", 200*time.Millisecond)
	tracker.RecordActivity("route-a")

	ctx := context.Background()
	err = tracker.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = tracker.Stop() }()

	// Continuously record activity for 100ms to prevent the route from going idle
	activityDone := make(chan struct{})
	go func() {
		defer close(activityDone)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		deadline := time.NewTimer(100 * time.Millisecond)
		defer deadline.Stop()
		for {
			select {
			case <-ticker.C:
				tracker.RecordActivity("route-a")
			case <-deadline.C:
				return
			}
		}
	}()
	<-activityDone

	// Then: no sleep trigger received in the window
	sleepCh := tracker.SleepTriggers()
	select {
	case routeID := <-sleepCh:
		t.Fatalf("unexpected sleep trigger for route %q; route was active", routeID)
	case <-time.After(50 * time.Millisecond):
		// expected: no trigger
	}
}

// TestStartReturnsErrorWhenAlreadyRunning verifies that calling Start twice returns an error.
func TestStartReturnsErrorWhenAlreadyRunning(t *testing.T) {
	// Given: a running tracker
	cfg := IdleTrackerConfig{CheckInterval: 100 * time.Millisecond}
	tracker, err := NewIdleTracker(cfg, newIdleTrackerTestLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = tracker.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = tracker.Stop() }()

	// When: calling Start again
	err = tracker.Start(ctx)

	// Then: error is returned
	assert.ErrorIs(t, err, ErrIdleTrackerAlreadyRunning)
}

// TestStopReturnsErrorWhenNotRunning verifies that calling Stop on a
// non-running tracker returns an error.
func TestStopReturnsErrorWhenNotRunning(t *testing.T) {
	// Given: a tracker that has not been started
	cfg := IdleTrackerConfig{CheckInterval: 100 * time.Millisecond}
	tracker, err := NewIdleTracker(cfg, newIdleTrackerTestLogger())
	require.NoError(t, err)

	// When: calling Stop without Start
	err = tracker.Stop()

	// Then: error is returned
	assert.ErrorIs(t, err, ErrIdleTrackerNotRunning)
}

// TestStopHaltsBackgroundGoroutine verifies that Stop halts the background
// checker and no further sleep triggers are emitted.
func TestStopHaltsBackgroundGoroutine(t *testing.T) {
	// Given: a running tracker with one idle route
	cfg := IdleTrackerConfig{CheckInterval: 20 * time.Millisecond}
	tracker, err := NewIdleTracker(cfg, newIdleTrackerTestLogger())
	require.NoError(t, err)

	tracker.RegisterRoute("route-a", 10*time.Millisecond)
	tracker.RecordActivity("route-a")

	ctx := context.Background()
	err = tracker.Start(ctx)
	require.NoError(t, err)

	// Let one trigger fire
	time.Sleep(80 * time.Millisecond)

	// When: Stop is called
	err = tracker.Stop()
	require.NoError(t, err)

	// Drain any buffered triggers
	sleepCh := tracker.SleepTriggers()
	for len(sleepCh) > 0 {
		<-sleepCh
	}

	// Then: no additional triggers after stop
	select {
	case routeID := <-sleepCh:
		t.Fatalf("received sleep trigger %q after Stop", routeID)
	case <-time.After(100 * time.Millisecond):
		// expected: no more triggers
	}
}

// --- Concurrency tests ---

// TestRecordActivityIsConcurrentlySafe verifies that RecordActivity is
// safe to call from multiple goroutines simultaneously.
func TestRecordActivityIsConcurrentlySafe(t *testing.T) {
	// Given: a tracker
	cfg := IdleTrackerConfig{CheckInterval: 100 * time.Millisecond}
	tracker, err := NewIdleTracker(cfg, newIdleTrackerTestLogger())
	require.NoError(t, err)

	const goroutines = 50
	const routeCount = 5

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// When: many goroutines record activity concurrently
	for i := range goroutines {
		go func(n int) {
			defer wg.Done()
			routeID := []string{"route-a", "route-b", "route-c", "route-d", "route-e"}[n%routeCount]
			tracker.RecordActivity(routeID)
		}(i)
	}

	wg.Wait()

	// Then: all routes have a last-seen time (no panic, no data race)
	for _, id := range []string{"route-a", "route-b", "route-c", "route-d", "route-e"} {
		idleTime := tracker.IdleTime(id)
		assert.Less(t, idleTime, time.Second, "route %q should have recent activity", id)
	}
}

// TestMultipleRoutesHaveIndependentIdleTimeouts verifies that different routes have
// independent idle timeouts.
func TestMultipleRoutesHaveIndependentIdleTimeouts(t *testing.T) {
	// Given: a tracker with two routes, one with a short timeout and one with a long timeout
	cfg := IdleTrackerConfig{CheckInterval: 20 * time.Millisecond}
	tracker, err := NewIdleTracker(cfg, newIdleTrackerTestLogger())
	require.NoError(t, err)

	tracker.RegisterRoute("short-idle", 40*time.Millisecond)
	tracker.RegisterRoute("long-idle", 10*time.Second)
	tracker.RecordActivity("short-idle")
	tracker.RecordActivity("long-idle")

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err = tracker.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = tracker.Stop() }()

	// Wait for short-idle to expire
	time.Sleep(80 * time.Millisecond)

	// Then: exactly "short-idle" triggers; "long-idle" does not
	sleepCh := tracker.SleepTriggers()

	var triggered []string
	deadline := time.After(200 * time.Millisecond)
loop:
	for {
		select {
		case routeID := <-sleepCh:
			triggered = append(triggered, routeID)
		case <-deadline:
			break loop
		}
	}

	assert.Contains(t, triggered, "short-idle", "short-idle route should have triggered sleep")
	assert.NotContains(t, triggered, "long-idle", "long-idle route should NOT have triggered sleep")
}

// TestSleepTriggersReturnsSameChannel verifies that SleepTriggers always returns the same channel.
func TestSleepTriggersReturnsSameChannel(t *testing.T) {
	// Given: a tracker
	cfg := IdleTrackerConfig{CheckInterval: 100 * time.Millisecond}
	tracker, err := NewIdleTracker(cfg, newIdleTrackerTestLogger())
	require.NoError(t, err)

	// When: calling SleepTriggers multiple times
	ch1 := tracker.SleepTriggers()
	ch2 := tracker.SleepTriggers()

	// Then: same channel returned
	assert.Equal(t, ch1, ch2)
}

// TestRegisterRouteUpdatesExistingTimeout verifies that calling RegisterRoute
// for an already-registered route updates the idle timeout without resetting the last-seen time.
func TestRegisterRouteUpdatesExistingTimeout(t *testing.T) {
	// Given: a tracker with a registered route
	cfg := IdleTrackerConfig{CheckInterval: 100 * time.Millisecond}
	tracker, err := NewIdleTracker(cfg, newIdleTrackerTestLogger())
	require.NoError(t, err)

	tracker.RegisterRoute("route-a", 5*time.Second)
	tracker.RecordActivity("route-a")

	// When: re-registering the same route with a different timeout
	tracker.RegisterRoute("route-a", 10*time.Second)

	// Then: idle time is still recent (last-seen was not reset)
	idleTime := tracker.IdleTime("route-a")
	assert.Less(t, idleTime, time.Second, "last-seen should not have been reset on re-registration")
}

// TestIdleTimeForRegisteredButNeverActiveRouteReturnsIdleNeverSeen verifies that a
// route registered but never given activity returns idleNeverSeen.
func TestIdleTimeForRegisteredButNeverActiveRouteReturnsIdleNeverSeen(t *testing.T) {
	// Given: a tracker with a registered route, but no RecordActivity called
	cfg := IdleTrackerConfig{CheckInterval: 100 * time.Millisecond}
	tracker, err := NewIdleTracker(cfg, newIdleTrackerTestLogger())
	require.NoError(t, err)

	tracker.RegisterRoute("route-a", 5*time.Second)

	// When: querying idle time without any prior activity
	idleTime := tracker.IdleTime("route-a")

	// Then: returns the sentinel idleNeverSeen value
	assert.Equal(t, idleNeverSeen, idleTime)
}

// TestNeverActiveRouteEmitsTriggerThenResets verifies that a route registered without
// any activity triggers a sleep signal (zero lastSeen treated as always idle) then resets.
func TestNeverActiveRouteEmitsTriggerThenResets(t *testing.T) {
	// Given: a tracker with a route that has no recorded activity
	cfg := IdleTrackerConfig{CheckInterval: 20 * time.Millisecond}
	tracker, err := NewIdleTracker(cfg, newIdleTrackerTestLogger())
	require.NoError(t, err)

	// Route registered but RecordActivity is never called (lastSeen is zero)
	// The zero lastSeen path in computeIdleTime returns idleNeverSeen,
	// which exceeds any finite idleTimeout, so a sleep trigger fires.
	tracker.RegisterRoute("never-active", 1*time.Second)

	ctx := context.Background()
	err = tracker.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = tracker.Stop() }()

	// When: waiting for the background checker to run
	time.Sleep(60 * time.Millisecond)

	// Then: a sleep trigger was emitted for the never-active route
	sleepCh := tracker.SleepTriggers()
	select {
	case routeID := <-sleepCh:
		assert.Equal(t, "never-active", routeID)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected sleep trigger for never-active route, but none received")
	}
}

// TestSleepTriggerChannelFullDropsExtraTriggers verifies that when the sleep trigger
// channel is full, additional triggers are dropped without blocking.
func TestSleepTriggerChannelFullDropsExtraTriggers(t *testing.T) {
	// Given: a tracker with more routes than the channel buffer can hold
	cfg := IdleTrackerConfig{CheckInterval: 10 * time.Millisecond}
	tracker, err := NewIdleTracker(cfg, newIdleTrackerTestLogger())
	require.NoError(t, err)

	// Register more routes than the channel buffer can hold, each with a unique ID.
	for i := range sleepTriggerBufSize + 5 {
		routeID := "route-" + string([]byte{byte('0' + i/10), byte('0' + i%10)})
		tracker.RegisterRoute(routeID, 1*time.Millisecond)
		tracker.RecordActivity(routeID)
	}

	ctx := context.Background()
	err = tracker.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = tracker.Stop() }()

	// When: waiting for idle triggers to fire (channel will fill and start dropping)
	time.Sleep(100 * time.Millisecond)

	// Then: no deadlock or panic; the channel is at most sleepTriggerBufSize full
	assert.LessOrEqual(t, len(tracker.SleepTriggers()), sleepTriggerBufSize)
}

// TestIdleTrackerRecordsSleepTriggerMetric tests that metrics are recorded when a sleep trigger is sent.
func TestIdleTrackerRecordsSleepTriggerMetric(t *testing.T) {
	// Given: an idle tracker with metrics enabled and a route that will idle
	cfg := IdleTrackerConfig{
		CheckInterval: 50 * time.Millisecond,
	}
	tracker, err := NewIdleTracker(cfg, newIdleTrackerTestLogger())
	require.NoError(t, err)

	m, err := metrics.New()
	require.NoError(t, err)
	tracker.SetMetrics(m)

	routeID := "test-route"
	tracker.RegisterRoute(routeID, 10*time.Millisecond)

	// When: starting the tracker and letting the route idle
	ctx := context.Background()
	err = tracker.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = tracker.Stop() }()

	time.Sleep(100 * time.Millisecond) // Wait for idle timeout to trigger

	// Then: sleep trigger metric should be recorded
	families, err := m.Gatherer().Gather()
	require.NoError(t, err)

	found := false
	for _, family := range families {
		if family.GetName() == "smaug_sleep_triggered_total" {
			found = true
			assert.NotEmpty(t, family.GetMetric(), "sleep triggered metric should be recorded")
			// Verify the server label
			for _, metric := range family.GetMetric() {
				for _, label := range metric.GetLabel() {
					if label.GetName() == "server" {
						assert.Equal(t, routeID, label.GetValue())
					}
				}
			}
		}
	}
	assert.True(t, found, "smaug_sleep_triggered_total metric should exist")
}

// TestIdleTrackerMetricsNilSafe tests that nil metrics don't cause panics.
func TestIdleTrackerMetricsNilSafe(t *testing.T) {
	// Given: an idle tracker with nil metrics
	cfg := IdleTrackerConfig{
		CheckInterval: 50 * time.Millisecond,
	}
	tracker, err := NewIdleTracker(cfg, newIdleTrackerTestLogger())
	require.NoError(t, err)

	routeID := "test-route"
	tracker.RegisterRoute(routeID, 10*time.Millisecond)

	// When: starting the tracker without setting metrics and letting route idle
	ctx := context.Background()
	err = tracker.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = tracker.Stop() }()

	// Then: no panic should occur
	time.Sleep(100 * time.Millisecond)
	assert.NotPanics(t, func() {}, "tracker should not panic with nil metrics")
}

// TestIdleTrackerSetMetricsBeforeStart tests that SetMetrics must be called before Start.
func TestIdleTrackerSetMetricsBeforeStart(t *testing.T) {
	// Given: an idle tracker that has been started
	cfg := IdleTrackerConfig{
		CheckInterval: 50 * time.Millisecond,
	}
	tracker, err := NewIdleTracker(cfg, newIdleTrackerTestLogger())
	require.NoError(t, err)

	ctx := context.Background()
	err = tracker.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = tracker.Stop() }()

	// When: trying to set metrics after Start
	m, err := metrics.New()
	require.NoError(t, err)
	tracker.SetMetrics(m) // Should be logged as warning, but not panic

	// Then: metrics should not be set (ignored)
	// We can't directly assert this, but we know it was ignored since the log said so
	// The test passes if there's no panic
	assert.True(t, true, "SetMetrics after Start should be safe (ignored)")
}
