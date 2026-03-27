package main

import (
	"strings"
	"testing"

	"github.com/rivo/tview"
)

func TestHelpBarTextForConversations(t *testing.T) {
	text := helpBarText(TrChat)
	if !strings.Contains(text, "Tree") {
		t.Fatalf("expected conversations help text, got %q", text)
	}
}

func TestHelpBarTextForMessages(t *testing.T) {
	text := helpBarText(ViChat)
	if !strings.Contains(text, "Msgs") {
		t.Fatalf("expected messages help text, got %q", text)
	}
}

func TestHelpBarTextIsCompact(t *testing.T) {
	for _, focused := range []string{TrChat, ViChat} {
		text := helpBarText(focused)
		if strings.Contains(text, "\n") {
			t.Fatalf("expected single-line help bar text, got %q", text)
		}
	}
}

func TestFindParentNode(t *testing.T) {
	root := tview.NewTreeNode("root")
	child := tview.NewTreeNode("child")
	leaf := tview.NewTreeNode("leaf")
	root.AddChild(child)
	child.AddChild(leaf)

	parent := findParentNode(root, leaf)
	if parent != child {
		t.Fatalf("expected parent %p, got %p", child, parent)
	}
}

func TestFirstSelectableChildReturnsFirstChild(t *testing.T) {
	root := tview.NewTreeNode("root")
	child := tview.NewTreeNode("child")
	root.AddChild(child)

	got := firstSelectableChild(root)
	if got != child {
		t.Fatalf("expected first child %p, got %p", child, got)
	}
}

func TestConversationHintTextForChats(t *testing.T) {
	mainText, secondaryText := conversationHintText(tview.NewTreeNode("Chats"))
	if !strings.Contains(mainText, "chat") {
		t.Fatalf("expected chat hint, got %q", mainText)
	}
	if !strings.Contains(secondaryText, "Enter") {
		t.Fatalf("expected navigation hint, got %q", secondaryText)
	}
}

func TestConversationHintTextForTeam(t *testing.T) {
	mainText, _ := conversationHintText(tview.NewTreeNode("CyberSec"))
	if !strings.Contains(mainText, "CyberSec") {
		t.Fatalf("expected team-specific hint, got %q", mainText)
	}
}
