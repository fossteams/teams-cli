package main

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"strings"
	"time"

	"github.com/fossteams/teams-api/pkg/csa"
	"github.com/rivo/tview"
)

func conversationPaneTitle(target ConversationTarget, syncedAt time.Time, status string) string {
	if status == "" {
		if syncedAt.IsZero() {
			return target.Title
		}
		status = "live " + syncedAt.Format("15:04:05")
	}

	if target.Title == "" {
		return status
	}

	return target.Title + " | " + status
}

func messagesSignature(messages []csa.ChatMessage) string {
	hasher := fnv.New64a()
	for _, message := range messages {
		_, _ = hasher.Write([]byte(message.Id))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(message.Version))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(message.ImDisplayName))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(message.Content))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(time.Time(message.ComposeTime).Format(time.RFC3339Nano)))
		_, _ = hasher.Write([]byte{'\n'})
	}

	return fmt.Sprintf("%x", hasher.Sum64())
}

func (s *AppState) renderConversationMessages(target ConversationTarget, messages []csa.ChatMessage, syncedAt time.Time, preserveSelection bool) {
	chatList := s.components[ViChat].(*tview.List)
	previousCount := chatList.GetItemCount()
	previousIndex := chatList.GetCurrentItem()
	followTail := previousCount > 0 && previousIndex >= previousCount-1
	status := ""
	if !syncedAt.IsZero() {
		status = s.conversationSyncStatus(syncedAt)
	}

	chatList.Clear()
	chatList.SetTitle(conversationPaneTitle(target, time.Time{}, status)).
		SetBorder(true).
		SetTitleAlign(tview.AlignCenter)
	s.chatPaneFocusable = true

	for _, message := range messages {
		if message.ImDisplayName == "" {
			continue
		}
		chatList.AddItem(textMessage(message.Content), message.ImDisplayName, 0, nil)
	}

	if chatList.GetItemCount() == 0 {
		chatList.AddItem("No recent messages found", fmt.Sprintf("Loaded the latest %d messages for this conversation.", s.messageLimit), 0, nil)
	}

	if !preserveSelection || chatList.GetItemCount() == 0 {
		return
	}

	switch {
	case followTail:
		chatList.SetCurrentItem(chatList.GetItemCount() - 1)
	case previousIndex < chatList.GetItemCount():
		chatList.SetCurrentItem(previousIndex)
	default:
		chatList.SetCurrentItem(chatList.GetItemCount() - 1)
	}
}

func (s *AppState) updateConversationPaneTitle(target ConversationTarget, syncedAt time.Time, status string) {
	chatList, ok := s.components[ViChat].(*tview.List)
	if !ok {
		return
	}
	if status == "" && !syncedAt.IsZero() {
		status = s.conversationSyncStatus(syncedAt)
	}

	chatList.SetTitle(conversationPaneTitle(target, time.Time{}, status)).
		SetBorder(true).
		SetTitleAlign(tview.AlignCenter)
}

func (s *AppState) renderConversationLoadError(target ConversationTarget, err error) {
	chatList, ok := s.components[ViChat].(*tview.List)
	if !ok {
		return
	}

	mainText, secondaryText := conversationLoadErrorText(err)
	chatList.Clear()
	chatList.SetTitle(conversationPaneTitle(target, time.Time{}, "load failed")).
		SetBorder(true).
		SetTitleAlign(tview.AlignCenter)
	chatList.AddItem(mainText, secondaryText, 0, nil)
	chatList.AddItem("Retry", "Press Right or l to retry this conversation. Esc returns to the list.", 0, nil)
	s.chatPaneFocusable = true
}

func conversationLoadErrorText(err error) (string, string) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "Timed out while loading recent messages", "Teams did not respond before the request timeout. Press Right or l to retry."
	case errors.Is(err, context.Canceled):
		return "Loading was cancelled", "Move to another conversation or press Right or l to retry."
	}

	var statusErr *messageFetchStatusError
	if errors.As(err, &statusErr) {
		switch statusErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return "Teams rejected the request", "Your Teams tokens may have expired. Refresh them and try again."
		case http.StatusTooManyRequests:
			return "Teams is rate limiting requests", "Wait a few seconds, then press Right or l to retry."
		default:
			if statusErr.StatusCode >= http.StatusInternalServerError {
				return "Teams returned a server error", "The service is temporarily unavailable. Press Right or l to retry."
			}
		}
	}

	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "An unexpected error occurred while loading recent messages."
	}

	return "Unable to load recent messages", message
}
