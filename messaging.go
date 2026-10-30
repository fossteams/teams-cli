package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fossteams/teams-api/pkg/csa"
)

const (
	authzEndpoint       = "https://teams.microsoft.com/api/authsvc/v1.0/authz"
	defaultMessagesHost = csa.MessagesHost
	sendMessageTimeout  = 15 * time.Second
)

// messagingSession holds what the chat-service calls need: the regional
// messaging host for this tenant and the short-lived session token, both
// taken from the Teams authz exchange.
type messagingSession struct {
	host       *url.URL
	skypeToken string
}

type authzResponse struct {
	Tokens struct {
		SkypeToken string `json:"skypeToken"`
		ExpiresIn  int    `json:"expiresIn"`
	} `json:"tokens"`
	Region     string `json:"region"`
	RegionGtms struct {
		ChatService string `json:"chatService"`
	} `json:"regionGtms"`
}

func newMessagingSession(ctx context.Context, client *http.Client, rootSkypeToken, endpoint string) (*messagingSession, error) {
	if client == nil {
		client = newMessageHTTPClient()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("unable to build authz request: %v", err)
	}
	req.ContentLength = 0
	req.Header.Set("Authorization", "Bearer "+rootSkypeToken)
	req.Header.Set("ms-teams-authz-type", "TokenRefresh")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("authz request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("authz rejected: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed authzResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("unable to decode authz response: %v", err)
	}
	if strings.TrimSpace(parsed.Tokens.SkypeToken) == "" {
		return nil, fmt.Errorf("authz response did not include a session token")
	}

	hostRaw := strings.TrimSpace(parsed.RegionGtms.ChatService)
	if hostRaw == "" {
		hostRaw = defaultMessagesHost
	}
	if !strings.HasSuffix(hostRaw, "/") {
		hostRaw += "/"
	}
	host, err := url.Parse(hostRaw)
	if err != nil || host.Scheme == "" || host.Host == "" {
		return nil, fmt.Errorf("invalid chat service host %q", hostRaw)
	}

	return &messagingSession{host: host, skypeToken: parsed.Tokens.SkypeToken}, nil
}

func (m *messagingSession) messagesURL(conversationID string) string {
	endpoint, err := m.host.Parse("v1/users/ME/conversations/" + url.QueryEscape(conversationID) + "/messages")
	if err != nil {
		return m.host.String()
	}

	return endpoint.String()
}

func (m *messagingSession) newRequest(ctx context.Context, method, endpoint string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authentication", "skypetoken="+m.skypeToken)

	return req, nil
}

// sendMessage posts a plain-text message to a conversation. Teams stores
// message bodies as HTML, so the text is escaped and wrapped in a paragraph.
func (s *AppState) sendMessage(ctx context.Context, target ConversationTarget, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("message is empty")
	}
	session := s.session()
	if session == nil {
		return fmt.Errorf("messaging session is not ready")
	}

	payload := map[string]string{
		"content":         "<p>" + html.EscapeString(text) + "</p>",
		"messagetype":     "RichText/Html",
		"contenttype":     "text",
		"imdisplayname":   s.selfDisplayName(),
		"clientmessageid": strconv.FormatInt(time.Now().UnixMilli(), 10),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("unable to encode message: %v", err)
	}

	req, err := session.newRequest(ctx, http.MethodPost, session.messagesURL(target.ID), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := s.httpClient
	if client == nil {
		client = newMessageHTTPClient()
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("unable to send message: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("unable to send message: unexpected status %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}

	return nil
}
