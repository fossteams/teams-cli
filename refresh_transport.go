package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fossteams/teams-api/pkg/csa"
	"github.com/rivo/tview"
	"github.com/sirupsen/logrus"
)

type messageFetchStatusError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *messageFetchStatusError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Body) == "" {
		return fmt.Sprintf("unable to fetch messages: unexpected status %s", e.Status)
	}

	return fmt.Sprintf("unable to fetch messages: unexpected status %s: %s", e.Status, strings.TrimSpace(e.Body))
}

func (s *AppState) runLiveRefreshLoop() {
	messageTicker := time.NewTicker(s.messageRefreshInterval())
	conversationTicker := time.NewTicker(s.conversationRefreshInterval())
	defer messageTicker.Stop()
	defer conversationTicker.Stop()
	defer s.appLogger().Debug("live refresh loop stopped")

	for {
		select {
		case <-messageTicker.C:
			if !atomic.CompareAndSwapUint32(&s.liveMessageRefreshActive, 0, 1) {
				continue
			}
			go func() {
				defer atomic.StoreUint32(&s.liveMessageRefreshActive, 0)
				s.refreshSelectedConversationMessages()
			}()
		case <-conversationTicker.C:
			if !atomic.CompareAndSwapUint32(&s.liveTreeRefreshActive, 0, 1) {
				continue
			}
			go func() {
				defer atomic.StoreUint32(&s.liveTreeRefreshActive, 0)
				s.refreshConversationTree()
			}()
		case <-s.appContext().Done():
			return
		}
	}
}

func (s *AppState) refreshSelectedConversationMessages() {
	select {
	case <-s.appContext().Done():
		return
	default:
	}

	if atomic.LoadUint64(&s.activeLoadSeq) != 0 {
		return
	}

	target, ok, previousSignature, _ := s.currentConversationSnapshot()
	if !ok {
		return
	}

	messages, err := s.fetchMessages(s.appContext(), target)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		s.appLogger().WithFields(logrus.Fields{
			"conversation_id":    target.ID,
			"conversation_title": target.Title,
		}).WithError(err).Warn("live message refresh failed")
		s.app.QueueUpdateDraw(func() {
			select {
			case <-s.appContext().Done():
				return
			default:
			}

			if !s.currentConversationIs(target.ID) {
				return
			}
			s.updateConversationPaneTitle(target, time.Time{}, "live retry "+time.Now().Format("15:04:05"))
		})
		return
	}

	signature := messagesSignature(messages)
	syncedAt := time.Now()
	s.app.QueueUpdateDraw(func() {
		select {
		case <-s.appContext().Done():
			return
		default:
		}

		currentTarget, ok, _, _ := s.currentConversationSnapshot()
		if !ok || currentTarget.ID != target.ID {
			return
		}

		s.rememberConversationSync(currentTarget, signature, syncedAt)
		if signature == previousSignature {
			s.updateConversationPaneTitle(currentTarget, syncedAt, "")
			return
		}

		s.renderConversationMessages(currentTarget, messages, syncedAt, true)
	})
}

func (s *AppState) refreshConversationTree() {
	select {
	case <-s.appContext().Done():
		return
	default:
	}

	data, err := s.fetchConversationData()
	if err != nil {
		s.appLogger().WithError(err).Warn("live conversation refresh failed")
		return
	}

	s.app.QueueUpdateDraw(func() {
		select {
		case <-s.appContext().Done():
			return
		default:
		}

		treeView, ok := s.components[TrChat].(*tview.TreeView)
		if !ok {
			return
		}

		state := captureTreeViewState(treeView.GetRoot(), treeView.GetCurrentNode())
		s.applyConversationData(data)

		rootNode, selectedNode, _ := s.buildConversationTree(state)
		treeView.SetRoot(rootNode)
		treeView.SetCurrentNode(selectedNode)
		if selectedNode == nil {
			return
		}

		if target, ok := selectedNode.GetReference().(ConversationTarget); ok && s.currentConversationIs(target.ID) {
			_, _, _, syncedAt := s.currentConversationSnapshot()
			s.updateCurrentConversationTarget(target)
			s.updateConversationPaneTitle(target, syncedAt, "")
			return
		}

		s.handleTreeSelectionChange(selectedNode)
	})
}

func (s *AppState) loadMessages(ctx context.Context, target ConversationTarget, loadSeq uint64, done chan struct{}) {
	defer close(done)
	defer s.clearActiveLoadCancel(loadSeq)

	s.appLogger().WithFields(logrus.Fields{
		"conversation_id":    target.ID,
		"conversation_title": target.Title,
		"message_limit":      s.messageLimit,
		"load_seq":           loadSeq,
	}).Debug("loading conversation messages")

	messages, err := s.fetchMessages(ctx, target)
	if err != nil {
		atomic.CompareAndSwapUint64(&s.activeLoadSeq, loadSeq, 0)
		if errors.Is(err, context.Canceled) {
			s.appLogger().WithFields(logrus.Fields{
				"conversation_id": target.ID,
				"load_seq":        loadSeq,
			}).Debug("conversation message load cancelled")
			return
		}
		if atomic.LoadUint64(&s.loadSeq) != loadSeq {
			return
		}

		s.appLogger().WithFields(logrus.Fields{
			"conversation_id":    target.ID,
			"conversation_title": target.Title,
			"load_seq":           loadSeq,
		}).WithError(err).Warn("conversation message load failed")
		s.app.QueueUpdateDraw(func() {
			select {
			case <-s.appContext().Done():
				return
			default:
			}

			currentTarget, ok, _, _ := s.currentConversationSnapshot()
			if !ok || currentTarget.ID != target.ID {
				return
			}

			s.renderConversationLoadError(currentTarget, err)
		})
		return
	}

	if atomic.LoadUint64(&s.loadSeq) != loadSeq {
		return
	}

	atomic.CompareAndSwapUint64(&s.activeLoadSeq, loadSeq, 0)
	syncedAt := time.Now()
	signature := messagesSignature(messages)
	s.appLogger().WithFields(logrus.Fields{
		"conversation_id":    target.ID,
		"conversation_title": target.Title,
		"message_count":      len(messages),
		"load_seq":           loadSeq,
	}).Debug("conversation messages loaded")
	s.app.QueueUpdateDraw(func() {
		select {
		case <-s.appContext().Done():
			return
		default:
		}

		currentTarget, ok, _, _ := s.currentConversationSnapshot()
		if !ok || currentTarget.ID != target.ID {
			return
		}

		s.rememberConversationSync(currentTarget, signature, syncedAt)
		s.renderConversationMessages(currentTarget, messages, syncedAt, false)
	})
}

func shouldRetryMessageFetch(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var statusErr *messageFetchStatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode == http.StatusTooManyRequests || statusErr.StatusCode >= http.StatusInternalServerError
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	return false
}

func messageFetchBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}

	backoff := messageFetchRetryBaseDelay * time.Duration(1<<(attempt-1))
	if backoff > messageFetchRetryMaxDelay {
		return messageFetchRetryMaxDelay
	}

	return backoff
}

func (s *AppState) waitForMessageFetchRetry(ctx context.Context, attempt int) error {
	timer := time.NewTimer(messageFetchBackoff(attempt))
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *AppState) fetchMessages(ctx context.Context, target ConversationTarget) ([]csa.ChatMessage, error) {
	if s.teamsClient == nil {
		return nil, fmt.Errorf("teams client is nil")
	}

	baseURL, err := url.Parse(csa.MessagesHost)
	if err != nil {
		return nil, fmt.Errorf("unable to parse messages host: %v", err)
	}

	endpointURL, err := baseURL.Parse("v1/users/ME/conversations/" + url.QueryEscape(target.ID) + "/messages")
	if err != nil {
		return nil, fmt.Errorf("unable to parse messages endpoint: %v", err)
	}

	values := endpointURL.Query()
	values.Add("view", "msnp24Equivalent|supportsMessageProperties")
	values.Add("pageSize", strconv.Itoa(s.messageLimit))
	values.Add("startTime", "1")
	endpointURL.RawQuery = values.Encode()

	s.appLogger().WithFields(logrus.Fields{
		"conversation_id":    target.ID,
		"conversation_title": target.Title,
		"message_limit":      s.messageLimit,
	}).Debug("fetching conversation messages")

	var lastErr error
	for attempt := 1; attempt <= messageFetchMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		attemptCtx, cancel := context.WithTimeout(ctx, messageRequestTimeout)
		messages, err := s.fetchMessagesOnce(attemptCtx, endpointURL.String())
		cancel()
		if err == nil {
			return messages, nil
		}
		if errors.Is(err, context.Canceled) {
			return nil, err
		}

		lastErr = err
		if attempt == messageFetchMaxAttempts || !shouldRetryMessageFetch(err) {
			return nil, err
		}

		s.appLogger().WithFields(logrus.Fields{
			"conversation_id": target.ID,
			"attempt":         attempt,
			"max_attempts":    messageFetchMaxAttempts,
			"backoff":         messageFetchBackoff(attempt).String(),
		}).WithError(err).Warn("retrying message fetch")
		if err := s.waitForMessageFetchRetry(ctx, attempt); err != nil {
			return nil, err
		}
	}

	return nil, lastErr
}

func (s *AppState) fetchMessagesOnce(ctx context.Context, endpoint string) ([]csa.ChatMessage, error) {
	req, err := s.teamsClient.ChatSvc().AuthenticatedRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)

	client := s.httpClient
	if client == nil {
		client = newMessageHTTPClient()
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, &messageFetchStatusError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       strings.TrimSpace(string(bodyBytes)),
		}
	}

	var msgResponse csa.MessagesResponse
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&msgResponse); err != nil {
		return nil, fmt.Errorf("unable to decode messages: %v", err)
	}

	messages := msgResponse.Messages
	sort.Sort(csa.SortMessageByTime(messages))
	if len(messages) > s.messageLimit {
		messages = messages[len(messages)-s.messageLimit:]
	}

	return messages, nil
}
