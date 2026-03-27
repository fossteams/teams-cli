package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestShouldRetryMessageFetchStatusErrors(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "rate limited",
			err: &messageFetchStatusError{
				StatusCode: http.StatusTooManyRequests,
				Status:     "429 Too Many Requests",
			},
			want: true,
		},
		{
			name: "server error",
			err: &messageFetchStatusError{
				StatusCode: http.StatusBadGateway,
				Status:     "502 Bad Gateway",
			},
			want: true,
		},
		{
			name: "client error",
			err: &messageFetchStatusError{
				StatusCode: http.StatusBadRequest,
				Status:     "400 Bad Request",
			},
			want: false,
		},
		{
			name: "cancelled",
			err:  context.Canceled,
			want: false,
		},
		{
			name: "deadline exceeded",
			err:  context.DeadlineExceeded,
			want: true,
		},
	}

	for _, tc := range testCases {
		if got := shouldRetryMessageFetch(tc.err); got != tc.want {
			t.Fatalf("%s: expected retry=%v, got %v", tc.name, tc.want, got)
		}
	}
}

func TestMessageFetchBackoffCapsAtMaximum(t *testing.T) {
	if got := messageFetchBackoff(1); got != messageFetchRetryBaseDelay {
		t.Fatalf("expected first backoff %v, got %v", messageFetchRetryBaseDelay, got)
	}

	got := messageFetchBackoff(10)
	if got != messageFetchRetryMaxDelay {
		t.Fatalf("expected backoff to cap at %v, got %v", messageFetchRetryMaxDelay, got)
	}
}

func TestConversationLoadErrorTextForUnauthorized(t *testing.T) {
	mainText, secondaryText := conversationLoadErrorText(&messageFetchStatusError{
		StatusCode: http.StatusUnauthorized,
		Status:     "401 Unauthorized",
	})

	if !strings.Contains(mainText, "Teams rejected") {
		t.Fatalf("expected auth failure title, got %q", mainText)
	}
	if !strings.Contains(secondaryText, "tokens may have expired") {
		t.Fatalf("expected auth guidance, got %q", secondaryText)
	}
}

func TestConversationLoadErrorTextForTimeout(t *testing.T) {
	mainText, secondaryText := conversationLoadErrorText(context.DeadlineExceeded)

	if !strings.Contains(mainText, "Timed out") {
		t.Fatalf("expected timeout title, got %q", mainText)
	}
	if !strings.Contains(secondaryText, "request timeout") {
		t.Fatalf("expected timeout guidance, got %q", secondaryText)
	}
}

func TestRequestStopCancelsRuntimeContext(t *testing.T) {
	state := AppState{}
	state.initRuntime(context.Background())

	state.requestStop()

	select {
	case <-state.appContext().Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected runtime context to be cancelled")
	}
}
