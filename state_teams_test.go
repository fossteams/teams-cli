package main

import (
	api "github.com/fossteams/teams-api/pkg"
	"github.com/fossteams/teams-api/pkg/csa"
	"testing"
	"time"
)

func TestResolveChatTitlePrefersMeetingSubject(t *testing.T) {
	chat := csa.Chat{
		MeetingInformation: csa.MeetingInfo{Subject: "Daily Standup"},
		Members: []csa.ChatMember{
			{Mri: "self", FriendlyName: "Me"},
			{Mri: "other", FriendlyName: "Alex"},
		},
	}

	got := resolveChatTitle(chat, "self", "Me")
	if got != "Daily Standup" {
		t.Fatalf("expected meeting subject title, got %q", got)
	}
}

func TestResolveChatTitleUsesSortedParticipantNames(t *testing.T) {
	chat := csa.Chat{
		Members: []csa.ChatMember{
			{Mri: "self", FriendlyName: "Me"},
			{Mri: "b", FriendlyName: "bravo"},
			{Mri: "a", FriendlyName: "Alpha"},
			{Mri: "dup", FriendlyName: "alpha"},
		},
	}

	got := resolveChatTitle(chat, "self", "Me")
	if got != "Alpha, bravo" {
		t.Fatalf("expected sorted unique participant names, got %q", got)
	}
}

func TestResolveChatTitleAvoidsCurrentUserNameFallback(t *testing.T) {
	chat := csa.Chat{
		ChatType: "chat",
		Members: []csa.ChatMember{
			{Mri: "self"},
			{Mri: "other-a"},
			{Mri: "other-b"},
		},
		LastMessage: csa.Message{ImDisplayName: "Me"},
	}

	got := resolveChatTitle(chat, "self", "Me")
	if got != "Group chat (2 people)" {
		t.Fatalf("expected neutral fallback title, got %q", got)
	}
}

func TestSortChannelsPrefersPinnedThenGeneralThenName(t *testing.T) {
	channels := []csa.Channel{
		{Id: "random", DisplayName: "Random"},
		{Id: "general", DisplayName: "General", IsGeneral: true},
		{Id: "announcements", DisplayName: "Announcements"},
	}

	sortChannels(channels, map[string]int{"announcements": 0})

	want := []string{"announcements", "general", "random"}
	for idx, channel := range channels {
		if channel.Id != want[idx] {
			t.Fatalf("expected channel %q at position %d, got %q", want[idx], idx, channel.Id)
		}
	}
}

func TestSortChatsPrefersRecentActivity(t *testing.T) {
	chats := []csa.Chat{
		{
			Id: "older",
			Members: []csa.ChatMember{
				{Mri: "older", FriendlyName: "Older"},
			},
			LastMessage: csa.Message{ComposeTime: mustRFC3339Time(t, "2026-03-26T10:00:00Z")},
		},
		{
			Id: "newer",
			Members: []csa.ChatMember{
				{Mri: "newer", FriendlyName: "Newer"},
			},
			LastMessage: csa.Message{ComposeTime: mustRFC3339Time(t, "2026-03-27T10:00:00Z")},
		},
	}

	sortChats(chats, "", "")

	if chats[0].Id != "newer" {
		t.Fatalf("expected most recent chat first, got %q", chats[0].Id)
	}
}

func mustRFC3339Time(t *testing.T, raw string) api.RFC3339Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("unable to parse test time %q: %v", raw, err)
	}

	return api.RFC3339Time(parsed)
}
