package main

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fossteams/teams-api/pkg/csa"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/sirupsen/logrus"
)

func (s *AppState) buildConversationTree(state treeViewState) (*tview.TreeNode, *tview.TreeNode, *tview.TreeNode) {
	rootPath := "Conversations"
	teamsPath := joinTreePath(rootPath, "Teams")
	chatsPath := joinTreePath(rootPath, "Chats")

	rootNode := tview.NewTreeNode("Conversations")
	rootNode.SetExpanded(true)
	teamsNode := tview.NewTreeNode("Teams")
	chatsNode := tview.NewTreeNode("Chats")
	teamsExpanded := len(state.expandedPaths) == 0 || pathExpanded(state.expandedPaths, teamsPath)

	var selectedNode *tview.TreeNode
	var firstNode *tview.TreeNode
	if state.selectedPath == rootPath {
		selectedNode = rootNode
	}
	if state.selectedPath == teamsPath {
		selectedNode = teamsNode
	}
	if state.selectedPath == chatsPath {
		selectedNode = chatsNode
	}

	for _, team := range s.conversations.Teams {
		teamPath := joinTreePath(teamsPath, team.DisplayName)
		teamNode := tview.NewTreeNode(team.DisplayName)
		teamNode.SetColor(tcell.ColorBlue)

		teamExpanded := pathExpanded(state.expandedPaths, teamPath)
		if len(state.expandedPaths) == 0 && state.selectedPath == "" && state.selectedTargetID == "" && firstNode == nil && len(team.Channels) > 0 {
			teamExpanded = true
			teamsExpanded = true
		}
		if state.selectedPath == teamPath {
			selectedNode = teamNode
		}

		for _, channel := range team.Channels {
			title := fmt.Sprintf("%s / %s", team.DisplayName, channel.DisplayName)
			channelNode := tview.NewTreeNode(channel.DisplayName)
			channelNode.SetReference(ConversationTarget{
				ID:    channel.Id,
				Title: title,
				Kind:  targetKindChannel,
			})
			channelNode.SetColor(tcell.ColorGreen)

			if firstNode == nil {
				firstNode = channelNode
			}
			if state.selectedTargetID == channel.Id {
				selectedNode = channelNode
				teamExpanded = true
				teamsExpanded = true
			}

			teamNode.AddChild(channelNode)
		}

		teamNode.SetExpanded(teamExpanded)
		teamsNode.AddChild(teamNode)
	}
	teamsNode.SetExpanded(teamsExpanded)

	meetingsPath := joinTreePath(rootPath, "Meetings")
	meetingsNode := tview.NewTreeNode("Meetings")
	if state.selectedPath == meetingsPath {
		selectedNode = meetingsNode
	}

	var pinnedChats, directChats, meetingChats []csa.Chat
	for _, chat := range s.conversations.Chats {
		switch {
		case s.pins.pinned(chat.Id):
			pinnedChats = append(pinnedChats, chat)
		case isMeetingChat(chat):
			meetingChats = append(meetingChats, chat)
		default:
			directChats = append(directChats, chat)
		}
	}

	freshTree := len(state.expandedPaths) == 0 && state.selectedPath == "" && state.selectedTargetID == ""
	addChat := func(parent *tview.TreeNode, chat csa.Chat, pinned bool, expanded *bool) {
		title := s.chatTitle(chat)
		text := title
		if pinned {
			text = pinMarker + title
		}
		kind := targetKindChat
		if isMeetingChat(chat) {
			kind = targetKindMeeting
		}

		chatNode := tview.NewTreeNode(text)
		chatNode.SetReference(ConversationTarget{
			ID:    chat.Id,
			Title: title,
			Kind:  kind,
		})
		chatNode.SetColor(tcell.ColorTeal)

		if firstNode == nil {
			firstNode = chatNode
		}
		if state.selectedTargetID == chat.Id {
			selectedNode = chatNode
			*expanded = true
		}

		parent.AddChild(chatNode)
	}

	chatsExpanded := pathExpanded(state.expandedPaths, chatsPath)
	if freshTree && firstNode == nil && len(pinnedChats)+len(directChats) > 0 {
		chatsExpanded = true
	}
	for _, chat := range pinnedChats {
		addChat(chatsNode, chat, true, &chatsExpanded)
	}
	for _, chat := range directChats {
		addChat(chatsNode, chat, false, &chatsExpanded)
	}
	chatsNode.SetExpanded(chatsExpanded)

	meetingsExpanded := pathExpanded(state.expandedPaths, meetingsPath)
	if freshTree && firstNode == nil && len(meetingChats) > 0 {
		meetingsExpanded = true
	}
	for _, chat := range meetingChats {
		addChat(meetingsNode, chat, false, &meetingsExpanded)
	}
	meetingsNode.SetExpanded(meetingsExpanded)

	rootNode.AddChild(teamsNode)
	rootNode.AddChild(chatsNode)
	rootNode.AddChild(meetingsNode)

	if selectedNode == nil {
		if firstNode != nil {
			selectedNode = firstNode
		} else {
			selectedNode = rootNode
		}
	}

	return rootNode, selectedNode, firstNode
}

func captureTreeViewState(root, current *tview.TreeNode) treeViewState {
	state := treeViewState{
		expandedPaths: map[string]struct{}{},
	}
	if root == nil {
		return state
	}

	var walk func(node *tview.TreeNode, path string)
	walk = func(node *tview.TreeNode, path string) {
		if node == nil {
			return
		}
		if len(node.GetChildren()) > 0 && node.IsExpanded() {
			state.expandedPaths[path] = struct{}{}
		}
		if node == current {
			if target, ok := node.GetReference().(ConversationTarget); ok {
				state.selectedTargetID = target.ID
			} else {
				state.selectedPath = path
			}
		}

		for _, child := range node.GetChildren() {
			walk(child, joinTreePath(path, child.GetText()))
		}
	}

	walk(root, root.GetText())
	return state
}

func joinTreePath(prefix, text string) string {
	if prefix == "" {
		return text
	}

	return prefix + "\x1f" + text
}

func pathExpanded(expandedPaths map[string]struct{}, path string) bool {
	if len(expandedPaths) == 0 {
		return false
	}

	_, ok := expandedPaths[path]
	return ok
}

func (s *AppState) keyboardHelpText() string {
	if !s.liveRefreshEnabled() {
		return strings.TrimSpace(`
Tree   Up/Down/j/k move   Right/l open
       Left/h/Esc back   Enter read
       p pin/unpin   Tab msgs

Msgs   Up/Down/j/k scroll   PgUp/Dn page
       g/G or Home/End jump   Left/h/Esc/Tab back
       Enter/i reply   (Enter sends, Esc cancels)

Live   Background refresh disabled

? help   R sign in again   q quit   Ctrl+C force quit
`)
	}

	return strings.TrimSpace(fmt.Sprintf(`
Tree   Up/Down/j/k move   Right/l open
       Left/h/Esc back   Enter read
       p pin/unpin   Tab msgs

Msgs   Up/Down/j/k scroll   PgUp/Dn page
       g/G or Home/End jump   Left/h/Esc/Tab back
       Enter/i reply   (Enter sends, Esc cancels)

Live   Selected conversation refreshes every %s
       Conversation tree refreshes every %s

? help   R sign in again   q quit   Ctrl+C force quit
`, s.messageRefreshInterval(), s.conversationRefreshInterval()))
}

func helpBarText(focusedComponent string) string {
	switch focusedComponent {
	case ViChat:
		return "[::b]Msgs[::-] Up/Down/j/k  PgUp/Dn  g/G jump  Enter/i reply  Left/Esc/Tab back  ?  q"
	case InCompose:
		return "[::b]Reply[::-] Enter send  Esc cancel"
	default:
		return "[::b]Tree[::-] Up/Down/j/k  Right open  Left/Esc back  Enter read  p pin  Tab msgs  ?  q"
	}
}

func (s *AppState) globalKeyHandler(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() == tcell.KeyCtrlC {
		s.requestStop()
		return nil
	}

	// While composing a reply every other key belongs to the input field.
	if s.focusedComponent == InCompose {
		return event
	}

	if event.Key() == tcell.KeyRune {
		switch event.Rune() {
		case '?':
			if s.helpVisible() {
				s.hideHelp()
			} else {
				s.showHelp()
			}
			return nil
		case 'q':
			s.requestStop()
			return nil
		case 'R':
			s.startInteractiveSignIn()
			return nil
		}
	}

	if s.helpVisible() && event.Key() == tcell.KeyEscape {
		s.hideHelp()
		return nil
	}

	return event
}

func (s *AppState) helpVisible() bool {
	name, _ := s.pages.GetFrontPage()
	return name == PageHelp
}

func (s *AppState) showHelp() {
	if s.helpVisible() {
		return
	}

	s.previousFocus = s.focusedComponent
	s.pages.ShowPage(PageHelp)
	s.pages.SendToFront(PageHelp)
	if modal, ok := s.components[MoHelp]; ok {
		s.app.SetFocus(modal)
	}
}

func (s *AppState) hideHelp() {
	s.pages.HidePage(PageHelp)
	target := s.previousFocus
	if target == "" {
		if pageName, _ := s.pages.GetFrontPage(); pageName != PageMain {
			s.app.SetFocus(s.pages)
			return
		}
		target = TrChat
	}
	s.focusComponent(target)
}

func (s *AppState) focusComponent(name string) {
	primitive, ok := s.components[name]
	if !ok || primitive == nil {
		return
	}

	s.focusedComponent = name
	s.updateHelpBar()
	s.updatePanelFocus()
	s.app.SetFocus(primitive)
}

func (s *AppState) updateHelpBar() {
	helpView, ok := s.components[TvHelp]
	if !ok {
		return
	}

	text := helpBarText(s.focusedComponent)
	if notice := s.noticeText(); notice != "" {
		text = "[yellow::b]" + tview.Escape(notice) + "[-:-:-]  " + text
	}
	helpView.(*tview.TextView).SetText(text)
	if helpModal, ok := s.components[MoHelp].(*tview.TextView); ok {
		helpModal.SetText(s.keyboardHelpText())
	}
}

func (s *AppState) updatePanelFocus() {
	treeView, treeOK := s.components[TrChat].(*tview.TreeView)
	chatView, chatOK := s.components[ViChat].(*tview.TextView)
	if !treeOK || !chatOK {
		return
	}

	activeColor := tcell.ColorYellow
	inactiveColor := tview.Styles.BorderColor
	treeView.SetBorderColor(inactiveColor)
	chatView.SetBorderColor(inactiveColor)

	switch s.focusedComponent {
	case ViChat, InCompose:
		chatView.SetBorderColor(activeColor)
	default:
		treeView.SetBorderColor(activeColor)
	}
}

func (s *AppState) treeKeyHandler(treeView *tview.TreeView) func(event *tcell.EventKey) *tcell.EventKey {
	return func(event *tcell.EventKey) *tcell.EventKey {
		current := treeView.GetCurrentNode()
		switch event.Key() {
		case tcell.KeyRight:
			s.handleTreeRight(treeView, current)
			return nil
		case tcell.KeyLeft:
			s.handleTreeLeft(treeView, current)
			return nil
		case tcell.KeyEnter:
			s.activateTreeNode(current)
			return nil
		}

		if event.Key() == tcell.KeyRune {
			switch event.Rune() {
			case 'h':
				s.handleTreeLeft(treeView, current)
				return nil
			case 'l':
				s.handleTreeRight(treeView, current)
				return nil
			case 'p':
				s.togglePinForNode(treeView, current)
				return nil
			case 'j':
				return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
			case 'k':
				return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
			}
		}

		return event
	}
}

// togglePinForNode pins or unpins the chat under the cursor and redraws the
// tree so it moves to (or from) the top of the Chats branch.
func (s *AppState) togglePinForNode(treeView *tview.TreeView, node *tview.TreeNode) {
	if node == nil || s.pins == nil {
		return
	}

	target, ok := node.GetReference().(ConversationTarget)
	if !ok || target.Kind == targetKindChannel {
		return
	}

	pinned, err := s.pins.toggle(target.ID)
	if err != nil {
		s.appLogger().WithError(err).WithField("conversation_id", target.ID).Warn("unable to update pins")
		return
	}

	s.appLogger().WithFields(logrus.Fields{
		"conversation_id":    target.ID,
		"conversation_title": target.Title,
		"pinned":             pinned,
	}).Info("pin updated")
	s.rebuildConversationTree(treeView)
}

// rebuildConversationTree redraws the tree from current state while keeping
// expansion and the selected conversation.
func (s *AppState) rebuildConversationTree(treeView *tview.TreeView) {
	state := captureTreeViewState(treeView.GetRoot(), treeView.GetCurrentNode())
	rootNode, selectedNode, _ := s.buildConversationTree(state)
	treeView.SetRoot(rootNode)
	treeView.SetCurrentNode(selectedNode)
}

func (s *AppState) chatKeyHandler() func(event *tcell.EventKey) *tcell.EventKey {
	return func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyBacktab, tcell.KeyEscape, tcell.KeyLeft:
			s.focusComponent(TrChat)
			return nil
		case tcell.KeyEnter:
			s.openCompose()
			return nil
		}

		// j/k, g/G, PgUp/PgDn and Home/End are handled by the text view itself.
		if event.Key() == tcell.KeyRune {
			switch event.Rune() {
			case 'h':
				s.focusComponent(TrChat)
				return nil
			case 'i':
				s.openCompose()
				return nil
			}
		}

		return event
	}
}

// openCompose reveals the reply line for the current conversation. Any text
// left from a failed send is kept so it can be edited and retried.
func (s *AppState) openCompose() {
	if _, ok, _, _ := s.currentConversationSnapshot(); !ok {
		return
	}
	compose, ok := s.components[InCompose].(*tview.InputField)
	if !ok || s.messageColumn == nil {
		return
	}

	s.messageColumn.ResizeItem(compose, 1, 0)
	s.focusComponent(InCompose)
}

func (s *AppState) closeCompose(keepText bool) {
	compose, ok := s.components[InCompose].(*tview.InputField)
	if !ok || s.messageColumn == nil {
		return
	}

	if !keepText {
		compose.SetText("")
	}
	s.messageColumn.ResizeItem(compose, 0, 0)
	s.focusComponent(ViChat)
}

func (s *AppState) composeDone(key tcell.Key) {
	switch key {
	case tcell.KeyEscape:
		s.closeCompose(false)
	case tcell.KeyEnter:
		s.submitCompose()
	}
}

func (s *AppState) submitCompose() {
	compose, ok := s.components[InCompose].(*tview.InputField)
	if !ok {
		return
	}

	text := strings.TrimSpace(compose.GetText())
	target, hasTarget, _, _ := s.currentConversationSnapshot()
	if text == "" || !hasTarget {
		s.closeCompose(false)
		return
	}

	s.closeCompose(false)
	s.updateConversationPaneTitle(target, time.Time{}, "sending...")
	go s.deliverMessage(target, text)
}

// deliverMessage sends in the background, then either triggers an immediate
// refresh so the message shows up, or puts the text back for another try.
func (s *AppState) deliverMessage(target ConversationTarget, text string) {
	sender := s.sendFunc
	if sender == nil {
		sender = s.sendMessage
	}

	ctx, cancel := context.WithTimeout(s.appContext(), sendMessageTimeout)
	err := sender(ctx, target, text)
	cancel()

	if err != nil {
		s.noteAuthFailure(err)
		s.appLogger().WithError(err).WithFields(logrus.Fields{
			"conversation_id":    target.ID,
			"conversation_title": target.Title,
		}).Warn("message send failed")
		s.app.QueueUpdateDraw(func() { s.restoreFailedSend(target, text) })
		return
	}

	s.appLogger().WithFields(logrus.Fields{
		"conversation_id":    target.ID,
		"conversation_title": target.Title,
	}).Info("message sent")
	s.startMessageRefresh()
}

// restoreFailedSend puts the text of a failed send back into the compose
// field for a retry, but only while the same conversation is still selected
// and nothing newer has been typed. The compose field is shared by every
// conversation, so restoring blindly could send the text to the wrong chat
// or wipe out a draft the user started in the meantime.
func (s *AppState) restoreFailedSend(target ConversationTarget, text string) {
	if !s.currentConversationIs(target.ID) {
		s.appLogger().WithFields(logrus.Fields{
			"conversation_id":    target.ID,
			"conversation_title": target.Title,
		}).Warn("failed message not restored: another conversation is selected")
		return
	}

	compose, ok := s.components[InCompose].(*tview.InputField)
	if ok && strings.TrimSpace(compose.GetText()) == "" {
		compose.SetText(text)
		s.updateConversationPaneTitle(target, time.Time{}, "send failed, press Enter to retry (see log)")
		return
	}
	s.updateConversationPaneTitle(target, time.Time{}, "send failed (see log)")
}

func (s *AppState) activateTreeNode(node *tview.TreeNode) {
	if node == nil {
		return
	}

	if len(node.GetChildren()) == 0 {
		s.focusMessagesPane()
		return
	}

	node.SetExpanded(!node.IsExpanded())
}

func (s *AppState) handleTreeRight(treeView *tview.TreeView, node *tview.TreeNode) {
	if node == nil {
		return
	}

	if len(node.GetChildren()) == 0 {
		s.enterConversationForNode(node)
		s.focusMessagesPane()
		return
	}

	if !node.IsExpanded() {
		node.SetExpanded(true)
		return
	}

	if child := firstSelectableChild(node); child != nil {
		s.selectTreeNode(child)
	}
}

func (s *AppState) handleTreeLeft(treeView *tview.TreeView, node *tview.TreeNode) {
	if node == nil {
		return
	}

	if len(node.GetChildren()) > 0 && node.IsExpanded() {
		node.SetExpanded(false)
		return
	}

	if parent := findParentNode(treeView.GetRoot(), node); parent != nil {
		s.selectTreeNode(parent)
	}
}

func (s *AppState) focusMessagesPane() {
	if !s.chatPaneFocusable {
		return
	}

	s.focusComponent(ViChat)
}

func (s *AppState) selectTreeNode(node *tview.TreeNode) {
	treeView, ok := s.components[TrChat].(*tview.TreeView)
	if !ok || node == nil {
		return
	}

	treeView.SetCurrentNode(node)
	s.handleTreeSelectionChange(node)
}

func (s *AppState) handleTreeSelectionChange(node *tview.TreeNode) {
	if node == nil {
		return
	}

	if len(node.GetChildren()) == 0 {
		s.loadConversationForNode(node)
		return
	}

	s.showConversationHint(node)
}

func firstSelectableChild(node *tview.TreeNode) *tview.TreeNode {
	if node == nil {
		return nil
	}

	children := node.GetChildren()
	if len(children) == 0 {
		return nil
	}

	return children[0]
}

func findParentNode(root, target *tview.TreeNode) *tview.TreeNode {
	if root == nil || target == nil || root == target {
		return nil
	}

	var parent *tview.TreeNode
	root.Walk(func(node, nodeParent *tview.TreeNode) bool {
		if node == target {
			parent = nodeParent
			return false
		}
		return true
	})

	return parent
}

func conversationHintText(node *tview.TreeNode) (string, string) {
	if node == nil {
		return "Choose a channel or chat to read messages", "Right expands. Left or Esc goes back."
	}

	name := strings.TrimSpace(node.GetText())
	switch name {
	case "Chats":
		return "Choose a chat to read messages", "Up/Down selects. Enter opens. p pins."
	case "Meetings":
		return "Choose a meeting chat to read messages", "Up/Down selects. Enter opens. p pins."
	case "Teams", "Conversations":
		return "Choose a channel or chat to read messages", "Right expands. Left or Esc goes back."
	default:
		return fmt.Sprintf("Choose a channel in %s", name), "Right expands. Left or Esc goes back."
	}
}

func (s *AppState) showConversationHint(node *tview.TreeNode) {
	s.cancelActiveLoad()
	atomic.AddUint64(&s.loadSeq, 1)
	atomic.StoreUint64(&s.activeLoadSeq, 0)
	s.chatPaneFocusable = false
	s.clearCurrentConversation()

	mainText, secondaryText := conversationHintText(node)
	chatView := s.components[ViChat].(*tview.TextView)
	chatView.SetTitle("Messages").
		SetBorder(true).
		SetTitleAlign(tview.AlignCenter)
	chatView.SetText(tview.Escape(mainText) + "\n" + tview.Escape(secondaryText))
	chatView.ScrollToBeginning()
}

// conversationEntryAction describes what stepping into a conversation should
// do with the messages pane.
type conversationEntryAction int

const (
	// entryKeepPane leaves the pane alone: a load is already in flight or the
	// messages were synced moments ago.
	entryKeepPane conversationEntryAction = iota
	// entryRefreshInPlace refetches in the background and redraws only if
	// something changed, so the current messages stay on screen.
	entryRefreshInPlace
	// entryFullLoad replaces the pane with the loading status. Used for a
	// conversation that is not the current one or whose last load failed.
	entryFullLoad
)

func (a conversationEntryAction) String() string {
	switch a {
	case entryKeepPane:
		return "keep"
	case entryRefreshInPlace:
		return "refresh"
	case entryFullLoad:
		return "load"
	}
	return fmt.Sprintf("conversationEntryAction(%d)", int(a))
}

// conversationEntryAction decides how to treat target when the user moves
// into it. Selecting the node already loaded it, so a second full load would
// only blank the pane and refetch what is already on screen.
func (s *AppState) conversationEntryAction(target ConversationTarget, now time.Time) conversationEntryAction {
	current, ok, _, syncedAt := s.currentConversationSnapshot()
	if !ok || current.ID != target.ID {
		return entryFullLoad
	}
	if atomic.LoadUint64(&s.activeLoadSeq) != 0 {
		return entryKeepPane
	}
	if syncedAt.IsZero() {
		// The initial load failed; the error text offers Right/l as the retry.
		return entryFullLoad
	}
	if now.Sub(syncedAt) < s.messageRefreshInterval() {
		return entryKeepPane
	}
	return entryRefreshInPlace
}

// enterConversationForNode is the Right/l handler for a leaf node. It keeps
// the messages that are already drawn and refreshes them in place when they
// are stale, falling back to a full load only when nothing usable is shown.
func (s *AppState) enterConversationForNode(node *tview.TreeNode) {
	if node == nil || len(node.GetChildren()) != 0 {
		return
	}

	target, ok := node.GetReference().(ConversationTarget)
	if !ok {
		return
	}

	switch s.conversationEntryAction(target, time.Now()) {
	case entryFullLoad:
		s.loadConversationForNode(node)
	case entryRefreshInPlace:
		s.startMessageRefresh()
	}
}

func (s *AppState) loadConversationForNode(node *tview.TreeNode) {
	if node == nil || len(node.GetChildren()) != 0 {
		return
	}

	reference := node.GetReference()
	if reference == nil {
		return
	}

	target, ok := reference.(ConversationTarget)
	if !ok {
		return
	}
	s.setCurrentConversation(target)
	s.cancelActiveLoad()

	chatView := s.components[ViChat].(*tview.TextView)
	chatView.SetTitle(conversationPaneTitle(target, time.Time{}, "loading...")).
		SetBorder(true).
		SetTitleAlign(tview.AlignCenter)
	s.chatPaneFocusable = true

	loadSeq := atomic.AddUint64(&s.loadSeq, 1)
	atomic.StoreUint64(&s.activeLoadSeq, loadSeq)
	loadCtx, cancel := context.WithCancel(s.appContext())
	s.replaceActiveLoadCancel(loadSeq, cancel)
	loadingStarted := time.Now()
	mainText, secondaryText := loadingStatusText(s.messageLimit, loadingStarted, 0)
	chatView.SetText(mainText + "\n" + secondaryText)
	chatView.ScrollToBeginning()

	done := make(chan struct{})
	go s.animateMessageLoading(loadSeq, loadingStarted, done)
	go s.loadMessages(loadCtx, target, loadSeq, done)
}

func (s *AppState) animateMessageLoading(loadSeq uint64, startedAt time.Time, done <-chan struct{}) {
	ticker := time.NewTicker(loadingTickInterval)
	defer ticker.Stop()

	step := 0
	for {
		select {
		case <-done:
			return
		case <-s.appContext().Done():
			return
		case <-ticker.C:
			step += 1
			s.app.QueueUpdateDraw(func() {
				select {
				case <-s.appContext().Done():
					return
				default:
				}

				if atomic.LoadUint64(&s.activeLoadSeq) != loadSeq {
					return
				}

				chatView, ok := s.components[ViChat].(*tview.TextView)
				if !ok {
					return
				}

				mainText, secondaryText := loadingStatusText(s.messageLimit, startedAt, step)
				chatView.SetText(mainText + "\n" + secondaryText)
			})
		}
	}
}

func loadingStatusText(limit int, startedAt time.Time, step int) (string, string) {
	elapsedSeconds := int(time.Since(startedAt).Seconds())
	mainText := fmt.Sprintf("Loading last %d messages %s", limit, loadingBarFrame(step, loadingBarWidth))
	secondaryText := fmt.Sprintf("Fetching recent messages from Teams... %ds", elapsedSeconds)
	return mainText, secondaryText
}

func loadingBarFrame(step, width int) string {
	if width < 3 {
		width = 3
	}

	pulseWidth := loadingPulseWidth
	if pulseWidth > width {
		pulseWidth = width
	}

	bar := []rune(strings.Repeat("-", width))
	cycleWidth := width + pulseWidth
	start := (step % cycleWidth) - pulseWidth
	for idx := 0; idx < pulseWidth; idx++ {
		position := start + idx
		if position >= 0 && position < width {
			bar[position] = '='
		}
	}

	head := start + pulseWidth
	if head >= 0 && head < width {
		bar[head] = '>'
	}

	return "[" + string(bar) + "]"
}
