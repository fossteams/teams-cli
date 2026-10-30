package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/fossteams/teams-api/pkg/csa"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func childTexts(node *tview.TreeNode) []string {
	texts := []string{}
	for _, child := range node.GetChildren() {
		texts = append(texts, child.GetText())
	}
	return texts
}

func findChildByText(node *tview.TreeNode, text string) *tview.TreeNode {
	for _, child := range node.GetChildren() {
		if child.GetText() == text {
			return child
		}
	}
	return nil
}

func newTreeTestState(t *testing.T, chats []csa.Chat) *AppState {
	t.Helper()
	pins, err := loadPinStore(t.TempDir() + "/pins.json")
	if err != nil {
		t.Fatalf("unable to create pin store: %v", err)
	}
	return &AppState{
		TeamsState: TeamsState{conversations: &csa.ConversationResponse{Chats: chats}},
		pins:       pins,
	}
}

func TestBuildConversationTreeSeparatesMeetingsFromChats(t *testing.T) {
	state := newTreeTestState(t, []csa.Chat{
		{Id: "19:meeting", ChatType: "meeting", Title: "Weekly Sync"},
		{Id: "19:chat", ChatType: "chat", Title: "Alex"},
	})

	root, _, _ := state.buildConversationTree(treeViewState{})

	if got := childTexts(root); strings.Join(got, ",") != "Teams,Chats,Meetings" {
		t.Fatalf("expected Teams, Chats and Meetings branches, got %v", got)
	}
	chats := findChildByText(root, "Chats")
	if got := childTexts(chats); len(got) != 1 || got[0] != "Alex" {
		t.Fatalf("expected only the direct chat under Chats, got %v", got)
	}
	meetings := findChildByText(root, "Meetings")
	if got := childTexts(meetings); len(got) != 1 || got[0] != "Weekly Sync" {
		t.Fatalf("expected only the meeting chat under Meetings, got %v", got)
	}
	target := meetings.GetChildren()[0].GetReference().(ConversationTarget)
	if target.Kind != targetKindMeeting {
		t.Fatalf("expected meeting target kind, got %q", target.Kind)
	}
}

func TestBuildConversationTreeListsPinnedChatsFirst(t *testing.T) {
	state := newTreeTestState(t, []csa.Chat{
		{Id: "19:recent", ChatType: "chat", Title: "Recent"},
		{Id: "19:pinned", ChatType: "chat", Title: "Pinned"},
	})
	if _, err := state.pins.toggle("19:pinned"); err != nil {
		t.Fatalf("unable to pin chat: %v", err)
	}

	root, _, _ := state.buildConversationTree(treeViewState{})
	chats := findChildByText(root, "Chats")

	got := childTexts(chats)
	if len(got) != 2 || got[0] != pinMarker+"Pinned" || got[1] != "Recent" {
		t.Fatalf("expected pinned chat first with marker, got %v", got)
	}
	target := chats.GetChildren()[0].GetReference().(ConversationTarget)
	if target.ID != "19:pinned" || target.Title != "Pinned" {
		t.Fatalf("expected pinned target to keep its id and unmarked title, got %+v", target)
	}
}

func TestBuildConversationTreeSelectsFirstChatWhenNoTeams(t *testing.T) {
	state := newTreeTestState(t, []csa.Chat{
		{Id: "19:meeting", ChatType: "meeting", Title: "Weekly Sync"},
		{Id: "19:chat", ChatType: "chat", Title: "Alex"},
	})

	_, selected, _ := state.buildConversationTree(treeViewState{})

	if target, ok := selected.GetReference().(ConversationTarget); !ok || target.ID != "19:chat" {
		t.Fatalf("expected the first direct chat to be selected, got %v", selected.GetReference())
	}
}

func TestTreePinKeyTogglesPinAndRebuildsTree(t *testing.T) {
	state := newUITestState()
	pins, err := loadPinStore(filepath.Join(t.TempDir(), "pins.json"))
	if err != nil {
		t.Fatal(err)
	}
	state.pins = pins
	state.conversations = &csa.ConversationResponse{Chats: []csa.Chat{
		{Id: "19:a", ChatType: "chat", Title: "A"},
		{Id: "19:b", ChatType: "chat", Title: "B"},
	}}
	treeView := state.components[TrChat].(*tview.TreeView)
	root, _, _ := state.buildConversationTree(treeViewState{})
	treeView.SetRoot(root)
	treeView.SetCurrentNode(findChildByText(root, "Chats").GetChildren()[1])

	state.treeKeyHandler(treeView)(tcell.NewEventKey(tcell.KeyRune, 'p', tcell.ModNone))

	if !pins.pinned("19:b") {
		t.Fatal("expected p to pin the selected chat")
	}
	chats := findChildByText(treeView.GetRoot(), "Chats")
	if got := childTexts(chats); got[0] != pinMarker+"B" {
		t.Fatalf("expected tree to be rebuilt with pinned chat first, got %v", got)
	}
	if current, ok := treeView.GetCurrentNode().GetReference().(ConversationTarget); !ok || current.ID != "19:b" {
		t.Fatalf("expected selection to stay on the pinned chat, got %v", treeView.GetCurrentNode().GetReference())
	}

	state.treeKeyHandler(treeView)(tcell.NewEventKey(tcell.KeyRune, 'p', tcell.ModNone))
	if pins.pinned("19:b") {
		t.Fatal("expected second p to unpin the chat")
	}
}

func TestTreePinKeyIgnoresChannels(t *testing.T) {
	state := newUITestState()
	pins, _ := loadPinStore(filepath.Join(t.TempDir(), "pins.json"))
	state.pins = pins
	state.conversations = &csa.ConversationResponse{
		Teams: []csa.Team{{Id: "team", DisplayName: "Eng", Channels: []csa.Channel{{Id: "19:general", DisplayName: "General"}}}},
	}
	treeView := state.components[TrChat].(*tview.TreeView)
	root, _, _ := state.buildConversationTree(treeViewState{})
	treeView.SetRoot(root)
	channel := findChildByText(findChildByText(root, "Teams"), "Eng").GetChildren()[0]
	treeView.SetCurrentNode(channel)

	state.treeKeyHandler(treeView)(tcell.NewEventKey(tcell.KeyRune, 'p', tcell.ModNone))

	if pins.pinned("19:general") {
		t.Fatal("expected channels to be ignored by the pin key")
	}
}

func TestRestoreFailedSendPutsTextBackForSameConversation(t *testing.T) {
	state := newUITestState()
	target := ConversationTarget{ID: "19:a", Title: "Alex", Kind: targetKindChat}
	state.setCurrentConversation(target)
	compose := state.components[InCompose].(*tview.InputField)

	state.restoreFailedSend(target, "hello again")

	if compose.GetText() != "hello again" {
		t.Fatalf("expected failed text restored, got %q", compose.GetText())
	}
	if title := state.components[ViChat].(*tview.TextView).GetTitle(); !strings.Contains(title, "retry") {
		t.Fatalf("expected retry hint in pane title, got %q", title)
	}
}

func TestRestoreFailedSendSkipsOtherConversationAndNewDraft(t *testing.T) {
	state := newUITestState()
	failed := ConversationTarget{ID: "19:a", Title: "Alex", Kind: targetKindChat}
	state.setCurrentConversation(ConversationTarget{ID: "19:b", Title: "Billie", Kind: targetKindChat})
	compose := state.components[InCompose].(*tview.InputField)

	state.restoreFailedSend(failed, "confidential note")
	if compose.GetText() != "" {
		t.Fatalf("expected no text restored while another conversation is selected, got %q", compose.GetText())
	}

	state.setCurrentConversation(failed)
	compose.SetText("new draft")
	state.restoreFailedSend(failed, "confidential note")
	if compose.GetText() != "new draft" {
		t.Fatalf("expected the newer draft to be kept, got %q", compose.GetText())
	}
}
