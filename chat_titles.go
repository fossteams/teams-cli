package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/fossteams/teams-api/pkg/csa"
)

func (s *AppState) chatTitle(chat csa.Chat) string {
	selfMri := ""
	selfDisplayName := ""
	if s.me != nil {
		selfMri = s.me.Mri
		selfDisplayName = s.me.DisplayName
	}

	return resolveChatTitleWith(chat, selfMri, selfDisplayName, s.displayNameMap())
}

func resolveChatTitle(chat csa.Chat, selfMri, selfDisplayName string) string {
	return resolveChatTitleWith(chat, selfMri, selfDisplayName, nil)
}

const maxNamesInChatTitle = 3

// resolveChatTitleWith builds a tree label for a chat. displayNames maps member
// MRIs to names looked up separately, because the conversations API no longer
// includes friendly names for most members. Group chats always carry either
// several participant names, a "+N" remainder, or a "(N)" participant count so
// they are distinguishable from one-to-one chats.
func resolveChatTitleWith(chat csa.Chat, selfMri, selfDisplayName string, displayNames map[string]string) string {
	others := 0
	for _, member := range chat.Members {
		if member.Mri != selfMri {
			others += 1
		}
	}
	isGroup := !chat.IsOneOnOne && others >= 2

	if title := strings.TrimSpace(chat.Title); title != "" && !sameDisplayName(title, selfDisplayName) {
		if isGroup {
			return fmt.Sprintf("%s (%d)", title, others)
		}
		return title
	}

	if subject := strings.TrimSpace(chat.MeetingInformation.Subject); subject != "" {
		return subject
	}

	names, unlisted := chatParticipantNames(chat, selfMri, displayNames)
	if len(names) > 0 {
		shown := names
		if len(shown) > maxNamesInChatTitle {
			shown = shown[:maxNamesInChatTitle]
		}
		remaining := unlisted + len(names) - len(shown)
		if remaining > 0 {
			return fmt.Sprintf("%s +%d", strings.Join(shown, ", "), remaining)
		}
		return strings.Join(shown, ", ")
	}

	if title := strings.TrimSpace(chat.LastMessage.ImDisplayName); title != "" && !sameDisplayName(title, selfDisplayName) {
		// Only the last sender is known by name here. A group chat still has
		// to look different from the one-to-one chat with that same person.
		if isGroup {
			return fmt.Sprintf("%s +%d", title, others-1)
		}
		return title
	}

	return anonymousChatTitle(chat, selfMri)
}

// chatParticipantNames returns the sorted, de-duplicated names of the other
// participants and how many other participants are not in that list: those
// with no resolvable name, and those sharing a name with someone already
// listed. Counting the latter keeps a group chat of two people with the same
// name from looking like the direct chat with one of them.
func chatParticipantNames(chat csa.Chat, selfMri string, displayNames map[string]string) ([]string, int) {
	names := []string{}
	seen := map[string]struct{}{}
	unlisted := 0

	for _, member := range chat.Members {
		if member.Mri == selfMri {
			continue
		}
		name := strings.TrimSpace(member.FriendlyName)
		if name == "" {
			name = strings.TrimSpace(displayNames[member.Mri])
		}
		if name == "" {
			unlisted += 1
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			unlisted += 1
			continue
		}
		seen[key] = struct{}{}
		names = append(names, name)
	}

	sort.Slice(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})

	return names, unlisted
}

func anonymousChatTitle(chat csa.Chat, selfMri string) string {
	participantCount := 0
	for _, member := range chat.Members {
		if member.Mri == selfMri {
			continue
		}
		participantCount += 1
	}

	switch {
	case strings.EqualFold(chat.ChatType, "meeting"):
		return "Meeting chat"
	case chat.IsOneOnOne || participantCount <= 1:
		return "Direct chat"
	case participantCount > 1:
		return fmt.Sprintf("Group chat (%d people)", participantCount)
	case len(chat.Members) > 0:
		return fmt.Sprintf("Group chat (%d people)", len(chat.Members))
	default:
		return "Conversation"
	}
}

func sameDisplayName(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}

	return strings.EqualFold(left, right)
}
