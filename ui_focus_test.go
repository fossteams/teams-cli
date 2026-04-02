package main

import (
	"context"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestFocusComponentUpdatesHelpTextAndBorders(t *testing.T) {
	state := newUITestState()
	treeView := state.components[TrChat].(*tview.TreeView)
	chatView := state.components[ViChat].(*tview.List)
	helpView := state.components[TvHelp].(*tview.TextView)

	state.focusComponent(TrChat)
	if got := helpView.GetText(true); !strings.Contains(got, "Tree") {
		t.Fatalf("expected tree help text, got %q", got)
	}
	if got := treeView.GetBorderColor(); got != tcell.ColorYellow {
		t.Fatalf("expected focused tree border color, got %v", got)
	}

	state.focusComponent(ViChat)
	if got := helpView.GetText(true); !strings.Contains(got, "Msgs") {
		t.Fatalf("expected message help text, got %q", got)
	}
	if got := chatView.GetBorderColor(); got != tcell.ColorYellow {
		t.Fatalf("expected focused message border color, got %v", got)
	}
	if got := treeView.GetBorderColor(); got == tcell.ColorYellow {
		t.Fatalf("expected tree border to be inactive, got %v", got)
	}
}

func TestFocusMessagesPaneRequiresFocusableChatPane(t *testing.T) {
	state := newUITestState()
	state.focusComponent(TrChat)

	state.chatPaneFocusable = false
	state.focusMessagesPane()
	if state.focusedComponent != TrChat {
		t.Fatalf("expected focus to remain on tree, got %s", state.focusedComponent)
	}

	state.chatPaneFocusable = true
	state.focusMessagesPane()
	if state.focusedComponent != ViChat {
		t.Fatalf("expected focus to move to messages, got %s", state.focusedComponent)
	}
}

func TestHandleTreeSelectionChangeParentShowsHintAndClearsConversation(t *testing.T) {
	state := newUITestState()
	state.setCurrentConversation(ConversationTarget{ID: "19:chat", Title: "Incident Review"})
	state.chatPaneFocusable = true

	parentNode := tview.NewTreeNode("CyberSec")
	parentNode.AddChild(tview.NewTreeNode("General"))

	state.handleTreeSelectionChange(parentNode)

	if _, ok, _, _ := state.currentConversationSnapshot(); ok {
		t.Fatal("expected current conversation to be cleared for parent selection")
	}
	if state.chatPaneFocusable {
		t.Fatal("expected chat pane to become unfocusable while showing a hint")
	}

	chatList := state.components[ViChat].(*tview.List)
	mainText, secondaryText := chatList.GetItemText(0)
	if !strings.Contains(mainText, "CyberSec") {
		t.Fatalf("expected team-specific hint text, got %q", mainText)
	}
	if !strings.Contains(secondaryText, "Right expands") {
		t.Fatalf("expected navigation hint, got %q", secondaryText)
	}
}

func TestChatKeyHandlerReturnsFocusToTree(t *testing.T) {
	state := newUITestState()
	state.chatPaneFocusable = true
	state.focusComponent(ViChat)

	handler := state.chatKeyHandler()
	if got := handler(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); got != nil {
		t.Fatalf("expected tab to be handled, got %v", got)
	}
	if state.focusedComponent != TrChat {
		t.Fatalf("expected focus to return to tree, got %s", state.focusedComponent)
	}
}

func newUITestState() *AppState {
	state := &AppState{
		app:          tview.NewApplication(),
		logger:       discardLogger,
		messageLimit: defaultMessageLimit,
		pages:        tview.NewPages(),
		components:   map[string]tview.Primitive{},
	}
	state.initRuntime(context.Background())
	root := state.createMainView()
	state.app.SetRoot(root, true)
	return state
}
