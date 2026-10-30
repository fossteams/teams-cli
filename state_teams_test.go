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

	// Two different members share the name "alpha": one is listed, the other
	// counts toward the remainder so the title still reflects three people.
	got := resolveChatTitle(chat, "self", "Me")
	if got != "Alpha, bravo +1" {
		t.Fatalf("expected sorted unique participant names plus remainder, got %q", got)
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

	sortChats(chats, "", "", nil)

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

func TestSortChatsDoesNotDemoteHiddenChats(t *testing.T) {
	chats := []csa.Chat{
		{
			Id:          "older-visible",
			LastMessage: csa.Message{ComposeTime: mustRFC3339Time(t, "2026-03-26T10:00:00Z")},
		},
		{
			Id:          "newer-hidden",
			Hidden:      true,
			LastMessage: csa.Message{ComposeTime: mustRFC3339Time(t, "2026-03-27T10:00:00Z")},
		},
	}

	sortChats(chats, "", "", nil)

	if chats[0].Id != "newer-hidden" {
		t.Fatalf("expected hidden flag to be ignored and most recent chat first, got %q", chats[0].Id)
	}
}

func TestIsMeetingChat(t *testing.T) {
	cases := map[string]bool{
		"meeting": true,
		"Meeting": true,
		"chat":    false,
		"":        false,
	}
	for chatType, want := range cases {
		if got := isMeetingChat(csa.Chat{ChatType: chatType}); got != want {
			t.Fatalf("isMeetingChat(%q) = %t, want %t", chatType, got, want)
		}
	}
}

func TestResolveChatTitleUsesDisplayNameLookupWhenMembersAreUnnamed(t *testing.T) {
	chat := csa.Chat{
		IsOneOnOne: true,
		Members: []csa.ChatMember{
			{Mri: "8:orgid:self"},
			{Mri: "8:orgid:other"},
		},
		LastMessage: csa.Message{ImDisplayName: "Me"},
	}
	names := map[string]string{"8:orgid:other": "Legaspi, Billy"}

	if got := resolveChatTitleWith(chat, "8:orgid:self", "Me", names); got != "Legaspi, Billy" {
		t.Fatalf("expected looked-up participant name, got %q", got)
	}
}

func TestResolveChatTitleMarksTitledGroupChatsWithParticipantCount(t *testing.T) {
	chat := csa.Chat{
		Title: "Project Falcon",
		Members: []csa.ChatMember{
			{Mri: "self"}, {Mri: "a"}, {Mri: "b"}, {Mri: "c"},
		},
	}

	if got := resolveChatTitleWith(chat, "self", "Me", nil); got != "Project Falcon (3)" {
		t.Fatalf("expected group title with participant count, got %q", got)
	}
}

func TestResolveChatTitleShowsRemainingCountForPartiallyNamedGroup(t *testing.T) {
	chat := csa.Chat{
		Members: []csa.ChatMember{
			{Mri: "self"}, {Mri: "a"}, {Mri: "b"}, {Mri: "c"},
		},
	}
	names := map[string]string{"a": "Alpha"}

	if got := resolveChatTitleWith(chat, "self", "Me", names); got != "Alpha +2" {
		t.Fatalf("expected named participant plus remaining count, got %q", got)
	}
}

func TestUnresolvedMemberMrisSkipsSelfNamedKnownAndMeetings(t *testing.T) {
	chats := []csa.Chat{
		{ChatType: "chat", Members: []csa.ChatMember{{Mri: "self"}, {Mri: "a"}, {Mri: "b", FriendlyName: "Bee"}}},
		{ChatType: "chat", Members: []csa.ChatMember{{Mri: "self"}, {Mri: "a"}, {Mri: "known"}}},
		{ChatType: "meeting", Members: []csa.ChatMember{{Mri: "self"}, {Mri: "meeting-only"}}},
	}

	got := unresolvedMemberMris(chats, "self", map[string]string{"known": "Known"})

	if len(got) != 1 || got[0] != "a" {
		t.Fatalf("expected only the unnamed, unknown, non-meeting member, got %v", got)
	}
}

func TestChunkStrings(t *testing.T) {
	got := chunkStrings([]string{"a", "b", "c", "d", "e"}, 2)
	if len(got) != 3 || len(got[0]) != 2 || len(got[2]) != 1 || got[2][0] != "e" {
		t.Fatalf("unexpected chunks %v", got)
	}
	if chunks := chunkStrings(nil, 2); len(chunks) != 0 {
		t.Fatalf("expected no chunks for empty input, got %v", chunks)
	}
}

func TestResolveChatTitleGroupFallbackToLastSenderKeepsRemainingCount(t *testing.T) {
	// An untitled group chat whose member names could not be looked up must
	// not collapse to the last sender's bare name, or it becomes
	// indistinguishable from the one-to-one chat with that person.
	chat := csa.Chat{
		Members: []csa.ChatMember{
			{Mri: "self"}, {Mri: "a"}, {Mri: "b"},
		},
		LastMessage: csa.Message{ImDisplayName: "Sangaraju, Krishna"},
	}

	if got := resolveChatTitleWith(chat, "self", "Me", nil); got != "Sangaraju, Krishna +1" {
		t.Fatalf("expected last sender plus remaining count for group chat, got %q", got)
	}
}

func TestResolveChatTitleOneOnOneFallbackToLastSenderStaysBare(t *testing.T) {
	chat := csa.Chat{
		IsOneOnOne:  true,
		Members:     []csa.ChatMember{{Mri: "self"}, {Mri: "a"}},
		LastMessage: csa.Message{ImDisplayName: "Sangaraju, Krishna"},
	}

	if got := resolveChatTitleWith(chat, "self", "Me", nil); got != "Sangaraju, Krishna" {
		t.Fatalf("expected bare last sender name for one-to-one chat, got %q", got)
	}
}

func TestDedupeChatsKeepsNewestEntryPerID(t *testing.T) {
	older := time.Date(2026, time.October, 27, 14, 0, 0, 0, time.UTC)
	newer := older.Add(time.Minute)
	chats := []csa.Chat{
		{Id: "19:krishna", LastMessage: csa.Message{ImDisplayName: "Sangaraju, Krishna", ComposeTime: api.RFC3339Time(older)}},
		{Id: "19:other"},
		{Id: "19:krishna", LastMessage: csa.Message{ImDisplayName: "Me", ComposeTime: api.RFC3339Time(newer)}},
	}

	got, duplicates := dedupeChats(chats)

	if len(got) != 2 || got[0].Id != "19:krishna" || got[1].Id != "19:other" {
		t.Fatalf("expected one entry per chat in original order, got %+v", got)
	}
	if got[0].LastMessage.ImDisplayName != "Me" {
		t.Fatalf("expected the entry with the newest activity to win, got %+v", got[0].LastMessage)
	}
	if len(duplicates) != 1 || duplicates[0] != "19:krishna" {
		t.Fatalf("expected the repeated id reported, got %v", duplicates)
	}
}

func TestDedupeChatsLeavesUniqueListAlone(t *testing.T) {
	chats := []csa.Chat{{Id: "a"}, {Id: "b"}}

	got, duplicates := dedupeChats(chats)

	if len(got) != 2 || len(duplicates) != 0 {
		t.Fatalf("expected unchanged list and no duplicates, got %+v %v", got, duplicates)
	}
}

func TestResolveChatTitleCountsSameNamedMembersTowardRemaining(t *testing.T) {
	chat := csa.Chat{
		Members: []csa.ChatMember{
			{Mri: "self"}, {Mri: "8:orgid:1", FriendlyName: "Alex Kim"}, {Mri: "8:orgid:2", FriendlyName: "Alex Kim"},
		},
	}

	if got := resolveChatTitleWith(chat, "self", "Me", nil); got != "Alex Kim +1" {
		t.Fatalf("expected the group chat to stay distinguishable from a direct chat, got %q", got)
	}

	direct := csa.Chat{IsOneOnOne: true, Members: []csa.ChatMember{{Mri: "self"}, {Mri: "8:orgid:1", FriendlyName: "Alex Kim"}}}
	if got := resolveChatTitleWith(direct, "self", "Me", nil); got != "Alex Kim" {
		t.Fatalf("expected direct chat title unchanged, got %q", got)
	}
}

func TestResolveDisplayNamesIsSafeAlongsideApply(t *testing.T) {
	state := &TeamsState{displayNames: map[string]string{"a": "Alpha"}}
	chats := []csa.Chat{{ChatType: "chat", Members: []csa.ChatMember{{Mri: "self"}, {Mri: "a"}}}}
	done := make(chan struct{})

	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			state.applyConversationData(&conversationData{displayNames: map[string]string{"a": "Alpha", "b": "Beta"}})
			_ = state.displayNameFor("a")
		}
	}()
	for i := 0; i < 200; i++ {
		names, err := state.resolveDisplayNames(chats)
		if err != nil || names["a"] != "Alpha" {
			t.Fatalf("unexpected lookup result %v err=%v", names, err)
		}
	}
	<-done
}
