package main

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
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

	chatsExpanded := pathExpanded(state.expandedPaths, chatsPath)
	if len(state.expandedPaths) == 0 && state.selectedPath == "" && state.selectedTargetID == "" && firstNode == nil && len(s.conversations.Chats) > 0 {
		chatsExpanded = true
	}
	for _, chat := range s.conversations.Chats {
		title := s.chatTitle(chat)
		chatNode := tview.NewTreeNode(title)
		chatNode.SetReference(ConversationTarget{
			ID:    chat.Id,
			Title: title,
		})
		chatNode.SetColor(tcell.ColorTeal)

		if firstNode == nil {
			firstNode = chatNode
		}
		if state.selectedTargetID == chat.Id {
			selectedNode = chatNode
			chatsExpanded = true
		}

		chatsNode.AddChild(chatNode)
	}
	chatsNode.SetExpanded(chatsExpanded)

	rootNode.AddChild(teamsNode)
	rootNode.AddChild(chatsNode)

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
Tree   Up/Down move   Right/l open
       Left/h/Esc back   Enter read
       Tab msgs

Msgs   Up/Down move   PgUp/Dn page
       Home/End jump   Left/h/Esc/Tab back

Live   Background refresh disabled

? help   q quit   Ctrl+C force quit
`)
	}

	return strings.TrimSpace(fmt.Sprintf(`
Tree   Up/Down move   Right/l open
       Left/h/Esc back   Enter read
       Tab msgs

Msgs   Up/Down move   PgUp/Dn page
       Home/End jump   Left/h/Esc/Tab back

Live   Selected conversation refreshes every %s
       Conversation tree refreshes every %s

? help   q quit   Ctrl+C force quit
`, s.messageRefreshInterval(), s.conversationRefreshInterval()))
}

func helpBarText(focusedComponent string) string {
	switch focusedComponent {
	case ViChat:
		return "[::b]Msgs[::-] Up/Down  PgUp/Dn page  Home/End jump  Left/Esc/Tab back  ?  q"
	default:
		return "[::b]Tree[::-] Up/Down  Right open  Left/Esc back  Enter read  Tab msgs  ?  q"
	}
}

func (s *AppState) globalKeyHandler(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() == tcell.KeyCtrlC {
		s.requestStop()
		return nil
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

	helpView.(*tview.TextView).SetText(helpBarText(s.focusedComponent))
	if helpModal, ok := s.components[MoHelp].(*tview.TextView); ok {
		helpModal.SetText(s.keyboardHelpText())
	}
}

func (s *AppState) updatePanelFocus() {
	treeView, treeOK := s.components[TrChat].(*tview.TreeView)
	chatView, chatOK := s.components[ViChat].(*tview.List)
	if !treeOK || !chatOK {
		return
	}

	activeColor := tcell.ColorYellow
	inactiveColor := tview.Styles.BorderColor
	treeView.SetBorderColor(inactiveColor)
	chatView.SetBorderColor(inactiveColor)

	switch s.focusedComponent {
	case ViChat:
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
			}
		}

		return event
	}
}

func (s *AppState) chatKeyHandler() func(event *tcell.EventKey) *tcell.EventKey {
	return func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyBacktab, tcell.KeyEscape, tcell.KeyLeft:
			s.focusComponent(TrChat)
			return nil
		}

		if event.Key() == tcell.KeyRune && event.Rune() == 'h' {
			s.focusComponent(TrChat)
			return nil
		}

		return event
	}
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
		s.loadConversationForNode(node)
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
		return "Choose a chat to read messages", "Up/Down selects. Enter opens."
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
	chatList := s.components[ViChat].(*tview.List)
	chatList.Clear()
	chatList.SetTitle("Messages").
		SetBorder(true).
		SetTitleAlign(tview.AlignCenter)
	chatList.AddItem(mainText, secondaryText, 0, nil)
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

	chatList := s.components[ViChat].(*tview.List)
	chatList.Clear()
	chatList.SetTitle(conversationPaneTitle(target, time.Time{}, "loading...")).
		SetBorder(true).
		SetTitleAlign(tview.AlignCenter)
	s.chatPaneFocusable = true

	loadSeq := atomic.AddUint64(&s.loadSeq, 1)
	atomic.StoreUint64(&s.activeLoadSeq, loadSeq)
	loadCtx, cancel := context.WithCancel(s.appContext())
	s.replaceActiveLoadCancel(loadSeq, cancel)
	loadingStarted := time.Now()
	mainText, secondaryText := loadingStatusText(s.messageLimit, loadingStarted, 0)
	chatList.AddItem(mainText, secondaryText, 0, nil)

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

				chatList := s.components[ViChat].(*tview.List)
				if chatList.GetItemCount() == 0 {
					return
				}

				mainText, secondaryText := loadingStatusText(s.messageLimit, startedAt, step)
				chatList.SetItemText(0, mainText, secondaryText)
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
