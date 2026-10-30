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

// messageListItem renders a message as a list entry: the highlighted line
// carries the author and time, the line beneath carries the message text.
func messageListItem(message csa.ChatMessage, now time.Time, nameFor func(mri string) string) (string, string) {
	header := strings.TrimSpace(message.ImDisplayName)
	if composed := time.Time(message.ComposeTime); !composed.IsZero() {
		local := composed.Local()
		layout := "15:04"
		if local.YearDay() != now.Local().YearDay() || local.Year() != now.Local().Year() {
			layout = "Jan 02 15:04"
		}
		header += "  " + local.Format(layout)
	}

	return header, renderMessageText(message.Content, nameFor)
}

const messageAuthorStyle = "[green::b]"

func (s *AppState) renderConversationMessages(target ConversationTarget, messages []csa.ChatMessage, syncedAt time.Time, preserveScroll bool) {
	chatView := s.components[ViChat].(*tview.TextView)
	status := ""
	if !syncedAt.IsZero() {
		status = s.conversationSyncStatus(syncedAt)
	}

	chatView.SetTitle(conversationPaneTitle(target, time.Time{}, status)).
		SetBorder(true).
		SetTitleAlign(tview.AlignCenter)
	s.chatPaneFocusable = true

	now := time.Now()
	var text strings.Builder
	for _, message := range messages {
		if message.ImDisplayName == "" {
			continue
		}
		header, body := messageListItem(message, now, s.displayNameFor)
		text.WriteString(messageAuthorStyle)
		text.WriteString(tview.Escape(header))
		text.WriteString("[-:-:-]\n")
		text.WriteString(tview.Escape(body))
		text.WriteString("\n\n")
	}
	if text.Len() == 0 {
		text.WriteString(fmt.Sprintf("No recent messages found\nLoaded the latest %d messages for this conversation.", s.messageLimit))
	}

	// SetText keeps the view pinned to the end when it already was (tview
	// tracks that itself), and keeps the scroll offset otherwise. A freshly
	// opened conversation always starts at the newest message.
	chatView.SetText(strings.TrimRight(text.String(), "\n"))
	if !preserveScroll {
		chatView.ScrollToEnd()
	}
}

func (s *AppState) updateConversationPaneTitle(target ConversationTarget, syncedAt time.Time, status string) {
	chatView, ok := s.components[ViChat].(*tview.TextView)
	if !ok {
		return
	}
	if status == "" && !syncedAt.IsZero() {
		status = s.conversationSyncStatus(syncedAt)
	}

	chatView.SetTitle(conversationPaneTitle(target, time.Time{}, status)).
		SetBorder(true).
		SetTitleAlign(tview.AlignCenter)
}

func (s *AppState) renderConversationLoadError(target ConversationTarget, err error) {
	chatView, ok := s.components[ViChat].(*tview.TextView)
	if !ok {
		return
	}

	mainText, secondaryText := conversationLoadErrorText(err)
	chatView.SetTitle(conversationPaneTitle(target, time.Time{}, "load failed")).
		SetBorder(true).
		SetTitleAlign(tview.AlignCenter)
	chatView.SetText(tview.Escape(mainText) + "\n" + tview.Escape(secondaryText) +
		"\n\nRetry: press Right or l to retry this conversation. Esc returns to the list.")
	chatView.ScrollToBeginning()
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
