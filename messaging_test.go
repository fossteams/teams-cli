package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewMessagingSessionUsesRegionalHostFromAuthz(t *testing.T) {
	var gotAuth, gotType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotType = r.Header.Get("ms-teams-authz-type")
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		_, _ = io.WriteString(w, `{"tokens":{"skypeToken":"session-token","expiresIn":86400},"region":"amer","regionGtms":{"chatService":"https://amer.ng.msg.example"}}`)
	}))
	defer server.Close()

	session, err := newMessagingSession(context.Background(), server.Client(), "root-jwt", server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer root-jwt" || gotType != "TokenRefresh" {
		t.Fatalf("unexpected authz headers: auth=%q type=%q", gotAuth, gotType)
	}
	if session.skypeToken != "session-token" {
		t.Fatalf("expected session token, got %q", session.skypeToken)
	}
	if got := session.messagesURL("19:abc@thread"); got != "https://amer.ng.msg.example/v1/users/ME/conversations/19%3Aabc%40thread/messages" {
		t.Fatalf("unexpected messages URL %q", got)
	}
}

func TestNewMessagingSessionFallsBackToDefaultHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"tokens":{"skypeToken":"session-token"},"regionGtms":{}}`)
	}))
	defer server.Close()

	session, err := newMessagingSession(context.Background(), server.Client(), "root-jwt", server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(session.messagesURL("x"), defaultMessagesHost) {
		t.Fatalf("expected default host fallback, got %q", session.messagesURL("x"))
	}
}

func TestNewMessagingSessionReportsRejectedToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"errorCode":"TokenExpired"}`)
	}))
	defer server.Close()

	if _, err := newMessagingSession(context.Background(), server.Client(), "root-jwt", server.URL); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 to be reported, got %v", err)
	}
}

func TestSendMessagePostsHTMLBodyWithSessionToken(t *testing.T) {
	var captured *http.Request
	var body map[string]string
	state := AppState{
		TeamsState: TeamsState{me: testSelfUser("Tucker, Michael")},
		logger:     discardLogger,
		messaging:  testMessagingSession(),
		httpClient: &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			captured = req
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("unable to decode body: %v", err)
			}
			return testHTTPResponse(http.StatusCreated, `{"OriginalArrivalTime":"2026-10-27T15:00:00Z"}`), nil
		})},
	}

	err := state.sendMessage(context.Background(), ConversationTarget{ID: "19:chat@unq.gbl.spaces", Kind: targetKindChat}, "hi <there> & bye")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.Method != http.MethodPost || captured.URL.String() != testMessagingSession().messagesURL("19:chat@unq.gbl.spaces") {
		t.Fatalf("unexpected request %s %s", captured.Method, captured.URL)
	}
	if got := captured.Header.Get("Authentication"); got != "skypetoken=test-session-token" {
		t.Fatalf("unexpected auth header %q", got)
	}
	if body["content"] != "<p>hi &lt;there&gt; &amp; bye</p>" || body["messagetype"] != "RichText/Html" || body["contenttype"] != "text" {
		t.Fatalf("unexpected body %v", body)
	}
	if body["imdisplayname"] != "Tucker, Michael" || body["clientmessageid"] == "" {
		t.Fatalf("expected display name and client message id, got %v", body)
	}
}

func TestSendMessageReportsRejectedPost(t *testing.T) {
	state := AppState{
		logger:    discardLogger,
		messaging: testMessagingSession(),
		httpClient: &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return testHTTPResponse(http.StatusForbidden, "nope"), nil
		})},
	}

	err := state.sendMessage(context.Background(), ConversationTarget{ID: "19:chat"}, "hi")
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected 403 to be reported, got %v", err)
	}
}

func TestSendMessageRejectsBlankText(t *testing.T) {
	state := AppState{logger: discardLogger, messaging: testMessagingSession()}
	if err := state.sendMessage(context.Background(), ConversationTarget{ID: "19:chat"}, "   "); err == nil {
		t.Fatal("expected blank message to be rejected")
	}
}
