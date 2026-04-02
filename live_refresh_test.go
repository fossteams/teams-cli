package main

import (
	api "github.com/fossteams/teams-api/pkg"
	"github.com/fossteams/teams-api/pkg/csa"
	"github.com/rivo/tview"
	"strings"
	"testing"
	"time"
)

func TestConversationPaneTitleIncludesLiveStatus(t *testing.T) {
	target := ConversationTarget{ID: "chat-1", Title: "General"}
	syncedAt := time.Date(2026, time.March, 27, 12, 34, 56, 0, time.UTC)

	got := conversationPaneTitle(target, syncedAt, "")
	if !strings.Contains(got, "General") {
		t.Fatalf("expected title to include conversation title, got %q", got)
	}
	if !strings.Contains(got, "live 12:34:56") {
		t.Fatalf("expected live timestamp in title, got %q", got)
	}
}

func TestMessagesSignatureChangesWhenMessagesChange(t *testing.T) {
	base := []csa.ChatMessage{
		{
			Id:            "msg-1",
			Version:       "1",
			ImDisplayName: "Alice",
			Content:       "hello",
			ComposeTime:   api.RFC3339Time(time.Date(2026, time.March, 27, 10, 0, 0, 0, time.UTC)),
		},
	}

	changed := []csa.ChatMessage{
		{
			Id:            "msg-1",
			Version:       "2",
			ImDisplayName: "Alice",
			Content:       "hello again",
			ComposeTime:   api.RFC3339Time(time.Date(2026, time.March, 27, 10, 0, 0, 0, time.UTC)),
		},
	}

	if messagesSignature(base) == messagesSignature(changed) {
		t.Fatal("expected message signature to change when content changes")
	}
}

func TestCaptureTreeViewStateTracksSelectionAndExpansion(t *testing.T) {
	root := tview.NewTreeNode("Conversations").SetExpanded(true)
	teams := tview.NewTreeNode("Teams").SetExpanded(true)
	team := tview.NewTreeNode("Engineering").SetExpanded(true)
	channel := tview.NewTreeNode("General").SetReference(ConversationTarget{ID: "channel-1", Title: "Engineering / General"})
	root.AddChild(teams)
	teams.AddChild(team)
	team.AddChild(channel)

	state := captureTreeViewState(root, channel)
	if state.selectedTargetID != "channel-1" {
		t.Fatalf("expected selected target ID to be tracked, got %q", state.selectedTargetID)
	}

	teamPath := joinTreePath(joinTreePath(joinTreePath("", "Conversations"), "Teams"), "Engineering")
	if _, ok := state.expandedPaths[teamPath]; !ok {
		t.Fatalf("expected expanded team path %q to be captured", teamPath)
	}
}

func TestBuildConversationTreeRestoresSelectedConversation(t *testing.T) {
	state := AppState{
		TeamsState: TeamsState{
			conversations: &csa.ConversationResponse{
				Teams: []csa.Team{
					{
						DisplayName: "Engineering",
						Channels: []csa.Channel{
							{Id: "channel-1", DisplayName: "General"},
						},
					},
				},
			},
		},
	}

	_, selectedNode, _ := state.buildConversationTree(treeViewState{
		expandedPaths:    map[string]struct{}{},
		selectedTargetID: "channel-1",
	})
	if selectedNode == nil {
		t.Fatal("expected a selected node")
	}

	target, ok := selectedNode.GetReference().(ConversationTarget)
	if !ok {
		t.Fatal("expected selected node to reference a conversation target")
	}
	if target.ID != "channel-1" {
		t.Fatalf("expected selected conversation ID channel-1, got %q", target.ID)
	}
}
