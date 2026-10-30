package main

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	api "github.com/fossteams/teams-api/pkg"
	"github.com/fossteams/teams-api/pkg/csa"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestFocusComponentUpdatesHelpTextAndBorders(t *testing.T) {
	state := newUITestState()
	treeView := state.components[TrChat].(*tview.TreeView)
	chatView := state.components[ViChat].(*tview.TextView)
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

	hint := state.components[ViChat].(*tview.TextView).GetText(true)
	if !strings.Contains(hint, "CyberSec") {
		t.Fatalf("expected team-specific hint text, got %q", hint)
	}
	if !strings.Contains(hint, "Right expands") {
		t.Fatalf("expected navigation hint, got %q", hint)
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

func TestRenderConversationMessagesScrollsToNewestOnFirstLoad(t *testing.T) {
	state := newUITestState()
	messages := []csa.ChatMessage{}
	for idx := 0; idx < 40; idx++ {
		messages = append(messages, csa.ChatMessage{Id: fmt.Sprint(idx), ImDisplayName: "Alice", Content: fmt.Sprintf("message %02d", idx)})
	}

	state.renderConversationMessages(ConversationTarget{ID: "19:chat", Title: "Alice"}, messages, time.Now(), false)

	rows := drawRows(t, state.components[ViChat], 60, 12)
	if !rowsContain(rows, "message 39") {
		t.Fatalf("expected newest message visible after first load, got rows %q", rows)
	}
	if rowsContain(rows, "message 00") {
		t.Fatalf("expected view scrolled away from the oldest message, got rows %q", rows)
	}
}

func TestRenderConversationMessagesWrapsLongLines(t *testing.T) {
	state := newUITestState()
	long := "alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima"
	messages := []csa.ChatMessage{{Id: "1", ImDisplayName: "Alice", Content: "<p>" + long + "</p>"}}

	state.renderConversationMessages(ConversationTarget{ID: "19:chat", Title: "Alice"}, messages, time.Now(), false)

	rows := drawRows(t, state.components[ViChat], 30, 12)
	if !rowsContain(rows, "lima") {
		t.Fatalf("expected long message to wrap onto following rows, got rows %q", rows)
	}
}

func TestTreeKeyHandlerMapsVimKeysToArrows(t *testing.T) {
	state := newUITestState()
	treeView := state.components[TrChat].(*tview.TreeView)
	handler := state.treeKeyHandler(treeView)

	if got := handler(tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone)); got == nil || got.Key() != tcell.KeyDown {
		t.Fatalf("expected j to become Down, got %v", got)
	}
	if got := handler(tcell.NewEventKey(tcell.KeyRune, 'k', tcell.ModNone)); got == nil || got.Key() != tcell.KeyUp {
		t.Fatalf("expected k to become Up, got %v", got)
	}
}

func TestMessageListItemShowsAuthorAndTimeAboveText(t *testing.T) {
	composed := time.Date(2026, time.October, 27, 14, 13, 3, 0, time.UTC)
	message := csa.ChatMessage{
		ImDisplayName: "Sangaraju, Krishna",
		Content:       "<p>sure, ping me in the morning</p><p>if you need a ride let me know</p>",
		ComposeTime:   api.RFC3339Time(composed),
	}

	header, body := messageListItem(message, composed.Add(2*time.Hour), nil)

	wantTime := composed.Local().Format("15:04")
	if !strings.HasPrefix(header, "Sangaraju, Krishna") || !strings.Contains(header, wantTime) {
		t.Fatalf("expected author and time on the header line, got %q", header)
	}
	if body != "sure, ping me in the morning  if you need a ride let me know" {
		t.Fatalf("expected flattened message text on the body line, got %q", body)
	}
}

func TestMessageListItemIncludesDateWhenNotToday(t *testing.T) {
	composed := time.Date(2026, time.October, 20, 9, 5, 0, 0, time.UTC)
	message := csa.ChatMessage{ImDisplayName: "Alice", Content: "hi", ComposeTime: api.RFC3339Time(composed)}

	header, _ := messageListItem(message, composed.Add(72*time.Hour), nil)

	if !strings.Contains(header, composed.Local().Format("Jan 02")) {
		t.Fatalf("expected date for older messages, got %q", header)
	}
}

func TestRenderConversationMessagesPutsAuthorAboveText(t *testing.T) {
	state := newUITestState()
	messages := []csa.ChatMessage{{Id: "1", ImDisplayName: "Alice", Content: "<p>hello</p>"}}

	state.renderConversationMessages(ConversationTarget{ID: "19:chat", Title: "Alice"}, messages, time.Now(), false)

	lines := strings.Split(strings.TrimSpace(state.components[ViChat].(*tview.TextView).GetText(true)), "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "Alice") || strings.TrimSpace(lines[1]) != "hello" {
		t.Fatalf("expected author line followed by message text, got %q", lines)
	}
}

func drawRows(t *testing.T, primitive tview.Primitive, width, height int) []string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("unable to init simulation screen: %v", err)
	}
	screen.SetSize(width, height)
	primitive.SetRect(0, 0, width, height)
	primitive.Draw(screen)
	screen.Show()

	cells, w, h := screen.GetContents()
	rows := make([]string, h)
	for y := 0; y < h; y++ {
		var row strings.Builder
		for x := 0; x < w; x++ {
			cell := cells[y*w+x]
			if len(cell.Runes) > 0 {
				row.WriteString(string(cell.Runes))
			} else {
				row.WriteRune(' ')
			}
		}
		rows[y] = row.String()
	}
	return rows
}

func rowsContain(rows []string, text string) bool {
	for _, row := range rows {
		if strings.Contains(row, text) {
			return true
		}
	}
	return false
}

func TestMessagesPaneEnterAndIOpenCompose(t *testing.T) {
	for _, key := range []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, 'i', tcell.ModNone),
	} {
		state := newUITestState()
		state.setCurrentConversation(ConversationTarget{ID: "19:chat", Title: "Alice", Kind: targetKindChat})
		state.chatPaneFocusable = true
		state.focusComponent(ViChat)

		if got := state.chatKeyHandler()(key); got != nil {
			t.Fatalf("expected key %v to be consumed", key.Name())
		}
		if state.focusedComponent != InCompose {
			t.Fatalf("expected compose line focused after %v, got %s", key.Name(), state.focusedComponent)
		}
		if got := state.components[TvHelp].(*tview.TextView).GetText(true); !strings.Contains(got, "send") {
			t.Fatalf("expected compose help text, got %q", got)
		}
	}
}

func TestMessagesPaneEnterWithoutConversationDoesNothing(t *testing.T) {
	state := newUITestState()
	state.focusComponent(ViChat)

	state.chatKeyHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))

	if state.focusedComponent == InCompose {
		t.Fatal("expected compose to stay closed without a conversation")
	}
}

func TestComposeEscCancelsAndReturnsFocus(t *testing.T) {
	state := newUITestState()
	state.setCurrentConversation(ConversationTarget{ID: "19:chat", Title: "Alice", Kind: targetKindChat})
	state.chatPaneFocusable = true
	state.openCompose()
	compose := state.components[InCompose].(*tview.InputField)
	compose.SetText("draft")

	state.composeDone(tcell.KeyEscape)

	if state.focusedComponent != ViChat {
		t.Fatalf("expected focus back on messages, got %s", state.focusedComponent)
	}
	if compose.GetText() != "" {
		t.Fatalf("expected draft discarded, got %q", compose.GetText())
	}
}

func TestComposeEnterSendsTextAndClears(t *testing.T) {
	state := newUITestState()
	target := ConversationTarget{ID: "19:chat", Title: "Alice", Kind: targetKindChat}
	state.setCurrentConversation(target)
	state.chatPaneFocusable = true
	sent := make(chan string, 1)
	state.sendFunc = func(ctx context.Context, got ConversationTarget, text string) error {
		if got.ID != target.ID {
			t.Errorf("expected send to %s, got %s", target.ID, got.ID)
		}
		sent <- text
		return nil
	}
	state.openCompose()
	compose := state.components[InCompose].(*tview.InputField)
	compose.SetText("  hello there  ")

	state.composeDone(tcell.KeyEnter)

	select {
	case text := <-sent:
		if text != "hello there" {
			t.Fatalf("expected trimmed text sent, got %q", text)
		}
	case <-time.After(time.Second):
		t.Fatal("expected message to be sent")
	}
	if compose.GetText() != "" {
		t.Fatalf("expected compose cleared after send, got %q", compose.GetText())
	}
	if state.focusedComponent != ViChat {
		t.Fatalf("expected focus back on messages, got %s", state.focusedComponent)
	}
}

func TestComposeEnterWithBlankTextJustCloses(t *testing.T) {
	state := newUITestState()
	state.setCurrentConversation(ConversationTarget{ID: "19:chat", Kind: targetKindChat})
	state.chatPaneFocusable = true
	state.sendFunc = func(ctx context.Context, got ConversationTarget, text string) error {
		t.Error("expected no send for blank text")
		return nil
	}
	state.openCompose()

	state.composeDone(tcell.KeyEnter)

	if state.focusedComponent != ViChat {
		t.Fatalf("expected compose closed, got focus %s", state.focusedComponent)
	}
}

func TestGlobalKeyHandlerPassesTypingToCompose(t *testing.T) {
	state := newUITestState()
	state.setCurrentConversation(ConversationTarget{ID: "19:chat", Kind: targetKindChat})
	state.chatPaneFocusable = true
	state.openCompose()

	for _, r := range []rune{'q', '?'} {
		if got := state.globalKeyHandler(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)); got == nil {
			t.Fatalf("expected %q to reach the compose field while typing", r)
		}
	}
	if got := state.globalKeyHandler(tcell.NewEventKey(tcell.KeyCtrlC, 0, tcell.ModNone)); got != nil {
		t.Fatal("expected Ctrl+C to still be handled globally")
	}
}

func TestConversationEntryActionChoosesFullLoadForNewConversation(t *testing.T) {
	state := newUITestState()
	target := ConversationTarget{ID: "19:chat", Title: "Alice"}

	if got := state.conversationEntryAction(target, time.Now()); got != entryFullLoad {
		t.Fatalf("expected full load when nothing is selected, got %v", got)
	}

	state.setCurrentConversation(ConversationTarget{ID: "19:other", Title: "Bob"})
	if got := state.conversationEntryAction(target, time.Now()); got != entryFullLoad {
		t.Fatalf("expected full load for a different conversation, got %v", got)
	}
}

func TestConversationEntryActionRetriesAfterFailedLoad(t *testing.T) {
	state := newUITestState()
	target := ConversationTarget{ID: "19:chat", Title: "Alice"}
	state.setCurrentConversation(target)

	if got := state.conversationEntryAction(target, time.Now()); got != entryFullLoad {
		t.Fatalf("expected full load when the conversation never synced, got %v", got)
	}
}

func TestConversationEntryActionKeepsPaneWhileLoading(t *testing.T) {
	state := newUITestState()
	target := ConversationTarget{ID: "19:chat", Title: "Alice"}
	state.setCurrentConversation(target)
	state.activeLoadSeq = 7

	if got := state.conversationEntryAction(target, time.Now()); got != entryKeepPane {
		t.Fatalf("expected to keep the pane while a load is in flight, got %v", got)
	}
}

func TestConversationEntryActionKeepsPaneWhenJustSynced(t *testing.T) {
	state := newUITestState()
	target := ConversationTarget{ID: "19:chat", Title: "Alice"}
	state.setCurrentConversation(target)
	now := time.Now()
	state.rememberConversationSync(target, "sig", now.Add(-time.Second))

	if got := state.conversationEntryAction(target, now); got != entryKeepPane {
		t.Fatalf("expected to keep the pane right after a sync, got %v", got)
	}
}

func TestConversationEntryActionRefreshesInPlaceWhenStale(t *testing.T) {
	state := newUITestState()
	target := ConversationTarget{ID: "19:chat", Title: "Alice"}
	state.setCurrentConversation(target)
	now := time.Now()
	state.rememberConversationSync(target, "sig", now.Add(-2*state.messageRefreshInterval()))

	if got := state.conversationEntryAction(target, now); got != entryRefreshInPlace {
		t.Fatalf("expected an in-place refresh for a stale conversation, got %v", got)
	}
}

func TestHandleTreeRightKeepsMessagesWhenJustSynced(t *testing.T) {
	state := newUITestState()
	target := ConversationTarget{ID: "19:chat", Title: "Alice"}
	messages := []csa.ChatMessage{{Id: "1", ImDisplayName: "Alice", Content: "hello there"}}
	state.setCurrentConversation(target)
	state.renderConversationMessages(target, messages, time.Now(), false)
	state.rememberConversationSync(target, "sig", time.Now())

	node := tview.NewTreeNode("Alice").SetReference(target)
	treeView := state.components[TrChat].(*tview.TreeView)
	state.handleTreeRight(treeView, node)

	chatView := state.components[ViChat].(*tview.TextView)
	if got := chatView.GetText(true); !strings.Contains(got, "hello there") {
		t.Fatalf("expected messages to stay on screen, got %q", got)
	}
	if state.focusedComponent != ViChat {
		t.Fatalf("expected focus to move to messages, got %s", state.focusedComponent)
	}
	if atomic.LoadUint64(&state.activeLoadSeq) != 0 {
		t.Fatal("expected no full reload to start")
	}
}
