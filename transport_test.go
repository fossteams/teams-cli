package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	api "github.com/fossteams/teams-api/pkg"
	"github.com/fossteams/teams-api/pkg/csa"
	"github.com/fossteams/teams-api/pkg/models"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestFetchMessagesRetriesAndReturnsLatestMessages(t *testing.T) {
	var attempts int32

	state := AppState{
		logger:       discardLogger,
		messaging:    testMessagingSession(),
		messageLimit: 2,
		httpClient: &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				atomic.AddInt32(&attempts, 1)

				if req.URL.Host != "amer.ng.msg.example" {
					t.Fatalf("expected request to the session host, got %s", req.URL.Host)
				}
				if got := req.URL.Query().Get("pageSize"); got != "2" {
					t.Fatalf("expected pageSize=2, got %q", got)
				}
				if got := req.Header.Get("Authentication"); !strings.HasPrefix(got, "skypetoken=") {
					t.Fatalf("expected Skype auth header, got %q", got)
				}

				if atomic.LoadInt32(&attempts) == 1 {
					return testHTTPResponse(http.StatusTooManyRequests, "slow down"), nil
				}

				return testHTTPResponse(http.StatusOK, testMessagesJSON(t, []csa.ChatMessage{
					testChatMessage("message-3", "Carol", "third", time.Date(2026, time.March, 27, 9, 3, 0, 0, time.UTC)),
					testChatMessage("message-1", "Alice", "first", time.Date(2026, time.March, 27, 9, 1, 0, 0, time.UTC)),
					testChatMessage("message-2", "Bob", "second", time.Date(2026, time.March, 27, 9, 2, 0, 0, time.UTC)),
				})), nil
			}),
		},
	}

	messages, err := state.fetchMessages(context.Background(), ConversationTarget{
		ID:    "19:chat-thread",
		Title: "Incident Review",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Fatalf("expected 2 attempts, got %d", got)
	}
	if len(messages) != 2 {
		t.Fatalf("expected the latest 2 messages, got %d", len(messages))
	}
	if messages[0].Id != "message-2" || messages[1].Id != "message-3" {
		t.Fatalf("expected sorted trailing messages, got %q then %q", messages[0].Id, messages[1].Id)
	}
}

func TestFetchMessagesMalformedPayloadDoesNotRetry(t *testing.T) {
	var attempts int32

	state := AppState{
		logger:       discardLogger,
		messaging:    testMessagingSession(),
		messageLimit: 5,
		httpClient: &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				atomic.AddInt32(&attempts, 1)
				return testHTTPResponse(http.StatusOK, `{"messages":"bad-payload"}`), nil
			}),
		},
	}

	_, err := state.fetchMessages(context.Background(), ConversationTarget{
		ID:    "19:bad-chat",
		Title: "Broken Payload",
	})
	if err == nil {
		t.Fatal("expected a decode error")
	}
	if !strings.Contains(err.Error(), "unable to decode messages") {
		t.Fatalf("expected decode guidance, got %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("expected malformed payload to stop after 1 attempt, got %d", got)
	}
}

func TestWaitForMessageFetchRetryHonorsCancellation(t *testing.T) {
	state := AppState{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	startedAt := time.Now()
	err := state.waitForMessageFetchRetry(ctx, 2)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed > 50*time.Millisecond {
		t.Fatalf("expected cancellation to return quickly, got %s", elapsed)
	}
}

func TestRunLiveRefreshLoopStopsWhenContextCancelled(t *testing.T) {
	state := AppState{
		logger:                       discardLogger,
		liveMessageRefreshEvery:      time.Hour,
		liveConversationRefreshEvery: time.Hour,
	}
	state.initRuntime(context.Background())

	done := make(chan struct{})
	go func() {
		state.runLiveRefreshLoop()
		close(done)
	}()

	state.requestStop()

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("expected live refresh loop to stop after shutdown")
	}
}

func testMessagingSession() *messagingSession {
	host, _ := url.Parse("https://amer.ng.msg.example/")
	return &messagingSession{host: host, skypeToken: "test-session-token"}
}

func testSelfUser(name string) *models.User {
	return &models.User{DisplayName: name, Mri: "8:orgid:self"}
}

func testHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Status:     fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func testMessagesJSON(t *testing.T, messages []csa.ChatMessage) string {
	t.Helper()

	bodyBytes, err := json.Marshal(csa.MessagesResponse{Messages: messages})
	if err != nil {
		t.Fatalf("unable to marshal test messages: %v", err)
	}

	return string(bodyBytes)
}

func testChatMessage(id, author, content string, composedAt time.Time) csa.ChatMessage {
	return csa.ChatMessage{
		Id:            id,
		Version:       "1",
		ImDisplayName: author,
		Content:       content,
		ComposeTime:   api.RFC3339Time(composedAt),
	}
}
