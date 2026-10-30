package main

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestRequestTokenRefreshReloadsSessionOnSuccess(t *testing.T) {
	state := newUITestState()
	interactive := make(chan bool, 1)
	reloaded := make(chan struct{}, 1)
	state.refreshFunc = func(ctx context.Context, allowInteractive bool) error {
		interactive <- allowInteractive
		return nil
	}
	state.reloadFunc = func() error {
		reloaded <- struct{}{}
		return nil
	}

	state.requestTokenRefresh("test")

	select {
	case got := <-interactive:
		if got {
			t.Fatal("expected a silent refresh first")
		}
	case <-time.After(time.Second):
		t.Fatal("expected refresh to run")
	}
	select {
	case <-reloaded:
	case <-time.After(time.Second):
		t.Fatal("expected session reload after refresh")
	}
	waitFor(t, "signed-in notice", func() bool { return strings.Contains(state.noticeText(), "signed in") })
}

func TestRequestTokenRefreshMarksSignInRequired(t *testing.T) {
	state := newUITestState()
	state.refreshFunc = func(ctx context.Context, allowInteractive bool) error { return errSignInRequired }
	state.reloadFunc = func() error { t.Error("reload must not run after a failed refresh"); return nil }

	state.requestTokenRefresh("test")

	waitFor(t, "sign-in required state", state.signInNeeded)
	if !strings.Contains(state.noticeText(), "R") {
		t.Fatalf("expected notice to mention the R key, got %q", state.noticeText())
	}
}

func TestRequestTokenRefreshRunsOneAtATime(t *testing.T) {
	state := newUITestState()
	release := make(chan struct{})
	calls := make(chan struct{}, 4)
	state.refreshFunc = func(ctx context.Context, allowInteractive bool) error {
		calls <- struct{}{}
		<-release
		return errors.New("boom")
	}

	state.requestTokenRefresh("first")
	state.requestTokenRefresh("second")
	close(release)

	waitFor(t, "refresh to finish", func() bool { return !state.tokenRefreshRunning() })
	if len(calls) != 1 {
		t.Fatalf("expected a single refresh run, got %d", len(calls))
	}
	if !strings.Contains(state.noticeText(), "failed") {
		t.Fatalf("expected failure notice, got %q", state.noticeText())
	}
}

func TestGlobalKeyRStartsInteractiveSignIn(t *testing.T) {
	state := newUITestState()
	interactive := make(chan bool, 1)
	state.refreshFunc = func(ctx context.Context, allowInteractive bool) error {
		interactive <- allowInteractive
		return nil
	}
	state.reloadFunc = func() error { return nil }
	state.focusComponent(TrChat)

	if got := state.globalKeyHandler(tcell.NewEventKey(tcell.KeyRune, 'R', tcell.ModNone)); got != nil {
		t.Fatal("expected R to be consumed")
	}
	select {
	case got := <-interactive:
		if !got {
			t.Fatal("expected R to allow an interactive sign-in")
		}
	case <-time.After(time.Second):
		t.Fatal("expected sign-in to start")
	}
}

func TestNoteAuthFailureIgnoresOtherErrors(t *testing.T) {
	state := newUITestState()
	state.refreshFunc = func(ctx context.Context, allowInteractive bool) error {
		t.Error("refresh must not run for a non-auth error")
		return nil
	}

	state.noteAuthFailure(errors.New("dial tcp: timeout"))
	time.Sleep(50 * time.Millisecond)
}

func (s *AppState) tokenRefreshRunning() bool {
	return atomic.LoadUint32(&s.tokenRefreshActive) == 1
}

func TestNoteAuthFailureDoesNotRefreshAgainRightAfterSuccess(t *testing.T) {
	state := newUITestState()
	calls := make(chan struct{}, 4)
	state.refreshFunc = func(ctx context.Context, allowInteractive bool) error {
		calls <- struct{}{}
		return nil
	}
	state.reloadFunc = func() error { return nil }
	rejected := &messageFetchStatusError{StatusCode: 403, Status: "403 Forbidden"}

	state.noteAuthFailure(rejected)
	waitFor(t, "first refresh to finish", func() bool { return len(calls) == 1 && !state.tokenRefreshRunning() })

	state.noteAuthFailure(rejected)
	time.Sleep(50 * time.Millisecond)
	if len(calls) != 1 {
		t.Fatalf("expected a rejection right after a successful refresh to be ignored, got %d refreshes", len(calls))
	}

	// Once the cooldown has passed the next rejection refreshes again.
	atomic.StoreInt64(&state.lastTokenRefreshAt, time.Now().Add(-authFailureRefreshCooldown-time.Second).UnixNano())
	state.noteAuthFailure(rejected)
	waitFor(t, "second refresh", func() bool { return len(calls) == 2 })
}

func TestHandleRefreshOutcomeUsesSingleFlightRefreshes(t *testing.T) {
	state := newUITestState()
	state.reloadFunc = func() error { return nil }
	// Pretend both refreshes are already in flight: the outcome handler must
	// go through the same guards as the ticker instead of starting duplicates.
	atomic.StoreUint32(&state.liveMessageRefreshActive, 1)
	atomic.StoreUint32(&state.liveTreeRefreshActive, 1)

	state.handleRefreshOutcome(nil)

	time.Sleep(50 * time.Millisecond)
	if atomic.LoadUint32(&state.liveMessageRefreshActive) != 1 || atomic.LoadUint32(&state.liveTreeRefreshActive) != 1 {
		t.Fatal("expected refresh guards to be left alone when a refresh is already running")
	}
}
