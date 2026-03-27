package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	teams_api "github.com/fossteams/teams-api"
	"github.com/fossteams/teams-api/pkg/csa"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/html"
	"hash/fnv"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type AppState struct {
	app         *tview.Application
	pages       *tview.Pages
	logger      *logrus.Logger
	logFilePath string
	ctx         context.Context
	cancel      context.CancelFunc

	TeamsState
	components                   map[string]tview.Primitive
	loadSeq                      uint64
	activeLoadSeq                uint64
	messageLimit                 int
	focusedComponent             string
	previousFocus                string
	chatPaneFocusable            bool
	httpClient                   *http.Client
	liveRefreshDisabled          bool
	liveMessageRefreshEvery      time.Duration
	liveConversationRefreshEvery time.Duration
	stateMu                      sync.RWMutex
	currentConversation          ConversationTarget
	currentConversationSet       bool
	currentMessagesSignature     string
	lastMessageSyncAt            time.Time
	liveLoopStarted              uint32
	liveMessageRefreshActive     uint32
	liveTreeRefreshActive        uint32
	loadCancelMu                 sync.Mutex
	activeLoadCancel             context.CancelFunc
	activeLoadCancelSeq          uint64
	shutdownOnce                 sync.Once
}

const (
	loadingBarWidth                        = 18
	loadingPulseWidth                      = 5
	loadingTickInterval                    = 120 * time.Millisecond
	helpBarHeight                          = 1
	defaultLiveMessageRefreshInterval      = 5 * time.Second
	defaultLiveConversationRefreshInterval = 15 * time.Second
	messageRequestTimeout                  = 5 * time.Second
	messageFetchMaxAttempts                = 3
	messageFetchRetryBaseDelay             = 250 * time.Millisecond
	messageFetchRetryMaxDelay              = time.Second
)

type treeViewState struct {
	expandedPaths    map[string]struct{}
	selectedPath     string
	selectedTargetID string
}

type messageFetchStatusError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *messageFetchStatusError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Body) == "" {
		return fmt.Sprintf("unable to fetch messages: unexpected status %s", e.Status)
	}

	return fmt.Sprintf("unable to fetch messages: unexpected status %s: %s", e.Status, strings.TrimSpace(e.Body))
}

func newMessageHTTPClient() *http.Client {
	return &http.Client{
		Timeout: messageRequestTimeout,
	}
}

func (s *AppState) initRuntime(parent context.Context) {
	if parent == nil {
		parent = context.Background()
	}

	s.ctx, s.cancel = context.WithCancel(parent)
	if s.httpClient == nil {
		s.httpClient = newMessageHTTPClient()
	}
}

func (s *AppState) ensureRuntime() {
	if s.ctx != nil && s.cancel != nil && s.httpClient != nil {
		return
	}

	s.initRuntime(context.Background())
}

func (s *AppState) appContext() context.Context {
	if s.ctx != nil {
		return s.ctx
	}

	return context.Background()
}

func (s *AppState) appLogger() *logrus.Logger {
	if s != nil && s.logger != nil {
		return s.logger
	}

	return discardLogger
}

func (s *AppState) requestStop() {
	s.shutdownOnce.Do(func() {
		s.appLogger().WithField("log_file", s.logFilePath).Info("shutdown requested")
		s.cancelActiveLoad()
		if s.cancel != nil {
			s.cancel()
		}
		if s.app != nil {
			s.app.Stop()
		}
	})
}

func (s *AppState) replaceActiveLoadCancel(loadSeq uint64, cancel context.CancelFunc) {
	s.loadCancelMu.Lock()
	previousCancel := s.activeLoadCancel
	s.activeLoadCancel = cancel
	s.activeLoadCancelSeq = loadSeq
	s.loadCancelMu.Unlock()

	if previousCancel != nil {
		previousCancel()
	}
}

func (s *AppState) clearActiveLoadCancel(loadSeq uint64) {
	s.loadCancelMu.Lock()
	defer s.loadCancelMu.Unlock()

	if s.activeLoadCancelSeq != loadSeq {
		return
	}

	s.activeLoadCancel = nil
	s.activeLoadCancelSeq = 0
}

func (s *AppState) cancelActiveLoad() {
	s.loadCancelMu.Lock()
	cancel := s.activeLoadCancel
	s.activeLoadCancel = nil
	s.activeLoadCancelSeq = 0
	s.loadCancelMu.Unlock()

	if cancel != nil {
		cancel()
	}
}

func (s *AppState) createApp() {
	s.ensureRuntime()
	s.pages = tview.NewPages()
	s.components = map[string]tview.Primitive{}

	// Add pages
	s.pages.AddPage(PageLogin, s.createLoginPage(), true, false)
	s.pages.AddPage(PageMain, s.createMainView(), true, false)
	s.pages.AddPage(PageError, s.createErrorView(), true, false)
	s.pages.AddPage(PageHelp, s.createHelpPage(), true, false)

	frame := tview.NewFrame(s.pages)
	frame.SetBorder(true)
	frame.SetTitle("teams-cli")
	frame.SetBorder(true)
	frame.SetTitleAlign(tview.AlignCenter)
	frame.AddText("? Help", false, tview.AlignLeft, tcell.ColorDarkCyan)
	frame.AddText("q Quit", false, tview.AlignRight, tcell.ColorDarkCyan)

	s.app.SetRoot(frame, true)
	s.app.SetInputCapture(s.globalKeyHandler)

	// Set main page
	s.pages.SwitchToPage(PageLogin)
	s.app.SetFocus(s.pages)

	go s.start()
}

func (s *AppState) createMainView() tview.Primitive {
	// Top: User information
	// Left side: Tree view (Teams _ Channels / Conversations)
	// Right side: Chat view
	// Bottom: Navigation bar

	treeView := tview.NewTreeView()
	treeView.SetBorder(true)
	treeView.SetTitle("Conversations")
	treeView.SetTitleAlign(tview.AlignLeft)
	chatView := tview.NewList()
	chatView.SetBackgroundColor(tcell.ColorBlack)
	chatView.SetBorder(true)
	chatView.SetTitle("Messages")
	chatView.SetTitleAlign(tview.AlignCenter)
	chatView.SetSelectedFocusOnly(true)
	helpView := tview.NewTextView()
	helpView.SetDynamicColors(true)
	helpView.SetWrap(true)
	helpView.SetTextAlign(tview.AlignLeft)
	helpView.SetBackgroundColor(tcell.ColorBlack)
	helpView.SetText(helpBarText(TrChat))

	s.components[TrChat] = treeView
	s.components[ViChat] = chatView
	s.components[TvHelp] = helpView

	contentFlex := tview.NewFlex().
		AddItem(treeView, 0, 1, false).
		AddItem(chatView, 0, 2, false)

	treeView.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyTab, tcell.KeyBacktab:
			s.focusMessagesPane()
		case tcell.KeyEscape:
			s.handleTreeLeft(treeView, treeView.GetCurrentNode())
		}
	})
	treeView.SetInputCapture(s.treeKeyHandler(treeView))
	chatView.SetDoneFunc(func() {
		s.focusComponent(TrChat)
	})
	chatView.SetInputCapture(s.chatKeyHandler())

	return tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(contentFlex, 0, 1, false).
		AddItem(helpView, helpBarHeight, 0, false)
}

func (s *AppState) createLoginPage() tview.Primitive {
	p := tview.NewTextView()
	p.SetTitle("Log-in")
	p.SetText("Logging in...")
	p.SetBackgroundColor(tcell.ColorBlue)
	p.SetTextAlign(tview.AlignCenter)
	p.SetBorder(true)
	p.SetBorderPadding(1, 1, 1, 1)

	s.components[TvLoginStatus] = p

	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, 10, 1, false).
			AddItem(nil, 0, 1, false), 30, 1, false).
		AddItem(nil, 0, 1, false)
}

func (s *AppState) start() {
	// Initialize Teams client
	var err error
	s.teamsClient, err = teams_api.New()
	if err != nil {
		s.appLogger().WithError(err).Error("unable to initialize teams client")
		s.showError(err)
		return
	}

	// Initialize Teams State
	err = s.TeamsState.init(s.teamsClient)
	if err != nil {
		s.appLogger().WithError(err).Error("unable to initialize teams state")
		s.showError(err)
		return
	}

	s.appLogger().WithFields(logrus.Fields{
		"teams_count": len(s.conversations.Teams),
		"chat_count":  len(s.conversations.Chats),
	}).Info("teams state initialized")

	select {
	case <-s.appContext().Done():
		return
	default:
	}

	go s.fillMainWindow()
}

func (s *AppState) showError(err error) {
	s.appLogger().WithError(err).WithField("log_file", s.logFilePath).Error("showing error view")

	val, ok := s.components[TvError]
	if !ok {
		s.appLogger().WithError(err).Error("error view is unavailable")
		return
	}
	s.app.QueueUpdateDraw(func() {
		message := err.Error()
		if strings.TrimSpace(s.logFilePath) != "" {
			message += "\n\nSee log: " + s.logFilePath
		}
		val.(*tview.TextView).SetText(message)
		s.pages.SwitchToPage(PageError)
	})
}

func (s *AppState) createErrorView() tview.Primitive {
	p := tview.NewTextView()
	p.SetTitle("ERROR")
	p.SetText("An error has occurred")
	p.SetBackgroundColor(tcell.ColorRed)
	p.SetTextAlign(tview.AlignCenter)
	p.SetBorder(true)
	p.SetBorderPadding(1, 1, 1, 1)

	s.components[TvError] = p

	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, 10, 1, false).
			AddItem(nil, 0, 1, false), 60, 1, false).
		AddItem(nil, 0, 1, false)
}

func (s *AppState) createHelpPage() tview.Primitive {
	helpText := tview.NewTextView()
	helpText.SetDynamicColors(true)
	helpText.SetText(s.keyboardHelpText())
	helpText.SetWrap(true)
	helpText.SetBorder(true)
	helpText.SetTitle("Keyboard Help")
	helpText.SetTitleAlign(tview.AlignCenter)
	helpText.SetBackgroundColor(tcell.ColorBlack)

	s.components[MoHelp] = helpText

	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(helpText, 11, 1, true).
			AddItem(nil, 0, 1, false), 58, 1, true).
		AddItem(nil, 0, 1, false)
}

func (s *AppState) fillMainWindow() {
	treeView := s.components[TrChat].(*tview.TreeView)
	rootNode, selectedNode, _ := s.buildConversationTree(treeViewState{})

	treeView.SetChangedFunc(func(node *tview.TreeNode) {
		s.handleTreeSelectionChange(node)
	})

	treeView.SetSelectedFunc(func(node *tview.TreeNode) {
		s.activateTreeNode(node)
	})

	treeView.SetRoot(rootNode)

	s.app.QueueUpdateDraw(func() {
		select {
		case <-s.appContext().Done():
			return
		default:
		}

		s.selectTreeNode(selectedNode)

		s.pages.SwitchToPage(PageMain)
		s.focusComponent(TrChat)
		if s.liveRefreshEnabled() && atomic.CompareAndSwapUint32(&s.liveLoopStarted, 0, 1) {
			s.appLogger().WithFields(logrus.Fields{
				"message_refresh":      s.messageRefreshInterval().String(),
				"conversation_refresh": s.conversationRefreshInterval().String(),
			}).Info("starting live refresh loop")
			go s.runLiveRefreshLoop()
		}
	})
}

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

func textMessage(input string) string {
	output := ""
	z := html.NewTokenizer(bytes.NewBuffer([]byte(input)))
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}

		switch tt {
		case html.TextToken:
			text := string(z.Text())
			if strings.TrimSpace(text) == "" {
				continue
			}
			output += fmt.Sprintf("\t%v\n", text)
		}
		if tt == html.ErrorToken {
			break
		}
	}
	return output
}

func (s *AppState) chatTitle(chat csa.Chat) string {
	selfMri := ""
	selfDisplayName := ""
	if s.me != nil {
		selfMri = s.me.Mri
		selfDisplayName = s.me.DisplayName
	}

	return resolveChatTitle(chat, selfMri, selfDisplayName)
}

func resolveChatTitle(chat csa.Chat, selfMri, selfDisplayName string) string {
	if title := strings.TrimSpace(chat.Title); title != "" && !sameDisplayName(title, selfDisplayName) {
		return title
	}

	if subject := strings.TrimSpace(chat.MeetingInformation.Subject); subject != "" {
		return subject
	}

	names := chatParticipantNames(chat, selfMri)
	switch {
	case len(names) == 0:
	case len(names) <= 3:
		return strings.Join(names, ", ")
	default:
		return fmt.Sprintf("%s +%d", strings.Join(names[:3], ", "), len(names)-3)
	}

	if title := strings.TrimSpace(chat.LastMessage.ImDisplayName); title != "" && !sameDisplayName(title, selfDisplayName) {
		return title
	}

	return anonymousChatTitle(chat, selfMri)
}

func chatParticipantNames(chat csa.Chat, selfMri string) []string {
	names := []string{}
	seen := map[string]struct{}{}

	for _, member := range chat.Members {
		name := strings.TrimSpace(member.FriendlyName)
		if name == "" || member.Mri == selfMri {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		names = append(names, name)
	}

	sort.Slice(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})

	return names
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

func (s *AppState) setCurrentConversation(target ConversationTarget) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	s.currentConversation = target
	s.currentConversationSet = true
	s.currentMessagesSignature = ""
	s.lastMessageSyncAt = time.Time{}
}

func (s *AppState) clearCurrentConversation() {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	s.currentConversation = ConversationTarget{}
	s.currentConversationSet = false
	s.currentMessagesSignature = ""
	s.lastMessageSyncAt = time.Time{}
}

func (s *AppState) currentConversationSnapshot() (ConversationTarget, bool, string, time.Time) {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	return s.currentConversation, s.currentConversationSet, s.currentMessagesSignature, s.lastMessageSyncAt
}

func (s *AppState) currentConversationIs(id string) bool {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	return s.currentConversationSet && s.currentConversation.ID == id
}

func (s *AppState) updateCurrentConversationTarget(target ConversationTarget) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	if s.currentConversationSet && s.currentConversation.ID == target.ID {
		s.currentConversation = target
	}
}

func (s *AppState) rememberConversationSync(target ConversationTarget, signature string, syncedAt time.Time) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	if !s.currentConversationSet || s.currentConversation.ID != target.ID {
		return
	}

	s.currentConversation = target
	s.currentMessagesSignature = signature
	s.lastMessageSyncAt = syncedAt
}

func conversationPaneTitle(target ConversationTarget, syncedAt time.Time, status string) string {
	if status == "" {
		if syncedAt.IsZero() {
			return target.Title
		}
		status = "live " + syncedAt.Format("15:04:05")
	}

	if target.Title == "" {
		return status
	}

	return target.Title + " | " + status
}

func messagesSignature(messages []csa.ChatMessage) string {
	hasher := fnv.New64a()
	for _, message := range messages {
		_, _ = hasher.Write([]byte(message.Id))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(message.Version))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(message.ImDisplayName))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(message.Content))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(time.Time(message.ComposeTime).Format(time.RFC3339Nano)))
		_, _ = hasher.Write([]byte{'\n'})
	}

	return fmt.Sprintf("%x", hasher.Sum64())
}

func (s *AppState) renderConversationMessages(target ConversationTarget, messages []csa.ChatMessage, syncedAt time.Time, preserveSelection bool) {
	chatList := s.components[ViChat].(*tview.List)
	previousCount := chatList.GetItemCount()
	previousIndex := chatList.GetCurrentItem()
	followTail := previousCount > 0 && previousIndex >= previousCount-1
	status := ""
	if !syncedAt.IsZero() {
		status = s.conversationSyncStatus(syncedAt)
	}

	chatList.Clear()
	chatList.SetTitle(conversationPaneTitle(target, time.Time{}, status)).
		SetBorder(true).
		SetTitleAlign(tview.AlignCenter)
	s.chatPaneFocusable = true

	for _, message := range messages {
		if message.ImDisplayName == "" {
			continue
		}
		chatList.AddItem(textMessage(message.Content), message.ImDisplayName, 0, nil)
	}

	if chatList.GetItemCount() == 0 {
		chatList.AddItem("No recent messages found", fmt.Sprintf("Loaded the latest %d messages for this conversation.", s.messageLimit), 0, nil)
	}

	if !preserveSelection || chatList.GetItemCount() == 0 {
		return
	}

	switch {
	case followTail:
		chatList.SetCurrentItem(chatList.GetItemCount() - 1)
	case previousIndex < chatList.GetItemCount():
		chatList.SetCurrentItem(previousIndex)
	default:
		chatList.SetCurrentItem(chatList.GetItemCount() - 1)
	}
}

func (s *AppState) updateConversationPaneTitle(target ConversationTarget, syncedAt time.Time, status string) {
	chatList, ok := s.components[ViChat].(*tview.List)
	if !ok {
		return
	}
	if status == "" && !syncedAt.IsZero() {
		status = s.conversationSyncStatus(syncedAt)
	}

	chatList.SetTitle(conversationPaneTitle(target, time.Time{}, status)).
		SetBorder(true).
		SetTitleAlign(tview.AlignCenter)
}

func (s *AppState) runLiveRefreshLoop() {
	messageTicker := time.NewTicker(s.messageRefreshInterval())
	conversationTicker := time.NewTicker(s.conversationRefreshInterval())
	defer messageTicker.Stop()
	defer conversationTicker.Stop()
	defer s.appLogger().Debug("live refresh loop stopped")

	for {
		select {
		case <-messageTicker.C:
			if !atomic.CompareAndSwapUint32(&s.liveMessageRefreshActive, 0, 1) {
				continue
			}
			go func() {
				defer atomic.StoreUint32(&s.liveMessageRefreshActive, 0)
				s.refreshSelectedConversationMessages()
			}()
		case <-conversationTicker.C:
			if !atomic.CompareAndSwapUint32(&s.liveTreeRefreshActive, 0, 1) {
				continue
			}
			go func() {
				defer atomic.StoreUint32(&s.liveTreeRefreshActive, 0)
				s.refreshConversationTree()
			}()
		case <-s.appContext().Done():
			return
		}
	}
}

func (s *AppState) refreshSelectedConversationMessages() {
	select {
	case <-s.appContext().Done():
		return
	default:
	}

	if atomic.LoadUint64(&s.activeLoadSeq) != 0 {
		return
	}

	target, ok, previousSignature, _ := s.currentConversationSnapshot()
	if !ok {
		return
	}

	messages, err := s.fetchMessages(s.appContext(), target)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		s.appLogger().WithFields(logrus.Fields{
			"conversation_id":    target.ID,
			"conversation_title": target.Title,
		}).WithError(err).Warn("live message refresh failed")
		s.app.QueueUpdateDraw(func() {
			select {
			case <-s.appContext().Done():
				return
			default:
			}

			if !s.currentConversationIs(target.ID) {
				return
			}
			s.updateConversationPaneTitle(target, time.Time{}, "live retry "+time.Now().Format("15:04:05"))
		})
		return
	}

	signature := messagesSignature(messages)
	syncedAt := time.Now()
	s.app.QueueUpdateDraw(func() {
		select {
		case <-s.appContext().Done():
			return
		default:
		}

		currentTarget, ok, _, _ := s.currentConversationSnapshot()
		if !ok || currentTarget.ID != target.ID {
			return
		}

		s.rememberConversationSync(currentTarget, signature, syncedAt)
		if signature == previousSignature {
			s.updateConversationPaneTitle(currentTarget, syncedAt, "")
			return
		}

		s.renderConversationMessages(currentTarget, messages, syncedAt, true)
	})
}

func (s *AppState) refreshConversationTree() {
	select {
	case <-s.appContext().Done():
		return
	default:
	}

	data, err := s.fetchConversationData()
	if err != nil {
		s.appLogger().WithError(err).Warn("live conversation refresh failed")
		return
	}

	s.app.QueueUpdateDraw(func() {
		select {
		case <-s.appContext().Done():
			return
		default:
		}

		treeView, ok := s.components[TrChat].(*tview.TreeView)
		if !ok {
			return
		}

		state := captureTreeViewState(treeView.GetRoot(), treeView.GetCurrentNode())
		s.applyConversationData(data)

		rootNode, selectedNode, _ := s.buildConversationTree(state)
		treeView.SetRoot(rootNode)
		treeView.SetCurrentNode(selectedNode)
		if selectedNode == nil {
			return
		}

		if target, ok := selectedNode.GetReference().(ConversationTarget); ok && s.currentConversationIs(target.ID) {
			_, _, _, syncedAt := s.currentConversationSnapshot()
			s.updateCurrentConversationTarget(target)
			s.updateConversationPaneTitle(target, syncedAt, "")
			return
		}

		s.handleTreeSelectionChange(selectedNode)
	})
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

func (s *AppState) liveRefreshEnabled() bool {
	return !s.liveRefreshDisabled
}

func (s *AppState) messageRefreshInterval() time.Duration {
	if s.liveMessageRefreshEvery > 0 {
		return s.liveMessageRefreshEvery
	}

	return defaultLiveMessageRefreshInterval
}

func (s *AppState) conversationRefreshInterval() time.Duration {
	if s.liveConversationRefreshEvery > 0 {
		return s.liveConversationRefreshEvery
	}

	return defaultLiveConversationRefreshInterval
}

func (s *AppState) conversationSyncStatus(syncedAt time.Time) string {
	if syncedAt.IsZero() {
		return ""
	}
	if s.liveRefreshEnabled() {
		return "live " + syncedAt.Format("15:04:05")
	}

	return "loaded " + syncedAt.Format("15:04:05")
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

func (s *AppState) loadMessages(ctx context.Context, target ConversationTarget, loadSeq uint64, done chan struct{}) {
	defer close(done)
	defer s.clearActiveLoadCancel(loadSeq)

	s.appLogger().WithFields(logrus.Fields{
		"conversation_id":    target.ID,
		"conversation_title": target.Title,
		"message_limit":      s.messageLimit,
		"load_seq":           loadSeq,
	}).Debug("loading conversation messages")

	messages, err := s.fetchMessages(ctx, target)

	if err != nil {
		atomic.CompareAndSwapUint64(&s.activeLoadSeq, loadSeq, 0)
		if errors.Is(err, context.Canceled) {
			s.appLogger().WithFields(logrus.Fields{
				"conversation_id": target.ID,
				"load_seq":        loadSeq,
			}).Debug("conversation message load cancelled")
			return
		}
		if atomic.LoadUint64(&s.loadSeq) != loadSeq {
			return
		}
		s.appLogger().WithFields(logrus.Fields{
			"conversation_id":    target.ID,
			"conversation_title": target.Title,
			"load_seq":           loadSeq,
		}).WithError(err).Warn("conversation message load failed")
		s.app.QueueUpdateDraw(func() {
			select {
			case <-s.appContext().Done():
				return
			default:
			}

			currentTarget, ok, _, _ := s.currentConversationSnapshot()
			if !ok || currentTarget.ID != target.ID {
				return
			}

			s.renderConversationLoadError(currentTarget, err)
		})
		return
	}

	if atomic.LoadUint64(&s.loadSeq) != loadSeq {
		return
	}

	atomic.CompareAndSwapUint64(&s.activeLoadSeq, loadSeq, 0)
	syncedAt := time.Now()
	signature := messagesSignature(messages)
	s.appLogger().WithFields(logrus.Fields{
		"conversation_id":    target.ID,
		"conversation_title": target.Title,
		"message_count":      len(messages),
		"load_seq":           loadSeq,
	}).Debug("conversation messages loaded")
	s.app.QueueUpdateDraw(func() {
		select {
		case <-s.appContext().Done():
			return
		default:
		}

		currentTarget, ok, _, _ := s.currentConversationSnapshot()
		if !ok || currentTarget.ID != target.ID {
			return
		}

		s.rememberConversationSync(currentTarget, signature, syncedAt)
		s.renderConversationMessages(currentTarget, messages, syncedAt, false)
	})
}

func (s *AppState) renderConversationLoadError(target ConversationTarget, err error) {
	chatList, ok := s.components[ViChat].(*tview.List)
	if !ok {
		return
	}

	mainText, secondaryText := conversationLoadErrorText(err)
	chatList.Clear()
	chatList.SetTitle(conversationPaneTitle(target, time.Time{}, "load failed")).
		SetBorder(true).
		SetTitleAlign(tview.AlignCenter)
	chatList.AddItem(mainText, secondaryText, 0, nil)
	chatList.AddItem("Retry", "Press Right or l to retry this conversation. Esc returns to the list.", 0, nil)
	s.chatPaneFocusable = true
}

func conversationLoadErrorText(err error) (string, string) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "Timed out while loading recent messages", "Teams did not respond before the request timeout. Press Right or l to retry."
	case errors.Is(err, context.Canceled):
		return "Loading was cancelled", "Move to another conversation or press Right or l to retry."
	}

	var statusErr *messageFetchStatusError
	if errors.As(err, &statusErr) {
		switch statusErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return "Teams rejected the request", "Your Teams tokens may have expired. Refresh them and try again."
		case http.StatusTooManyRequests:
			return "Teams is rate limiting requests", "Wait a few seconds, then press Right or l to retry."
		default:
			if statusErr.StatusCode >= http.StatusInternalServerError {
				return "Teams returned a server error", "The service is temporarily unavailable. Press Right or l to retry."
			}
		}
	}

	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "An unexpected error occurred while loading recent messages."
	}

	return "Unable to load recent messages", message
}

func shouldRetryMessageFetch(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var statusErr *messageFetchStatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode == http.StatusTooManyRequests || statusErr.StatusCode >= http.StatusInternalServerError
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	return false
}

func messageFetchBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}

	backoff := messageFetchRetryBaseDelay * time.Duration(1<<(attempt-1))
	if backoff > messageFetchRetryMaxDelay {
		return messageFetchRetryMaxDelay
	}

	return backoff
}

func (s *AppState) waitForMessageFetchRetry(ctx context.Context, attempt int) error {
	timer := time.NewTimer(messageFetchBackoff(attempt))
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *AppState) fetchMessages(ctx context.Context, target ConversationTarget) ([]csa.ChatMessage, error) {
	if s.teamsClient == nil {
		return nil, fmt.Errorf("teams client is nil")
	}

	baseURL, err := url.Parse(csa.MessagesHost)
	if err != nil {
		return nil, fmt.Errorf("unable to parse messages host: %v", err)
	}

	endpointURL, err := baseURL.Parse("v1/users/ME/conversations/" + url.QueryEscape(target.ID) + "/messages")
	if err != nil {
		return nil, fmt.Errorf("unable to parse messages endpoint: %v", err)
	}

	values := endpointURL.Query()
	values.Add("view", "msnp24Equivalent|supportsMessageProperties")
	values.Add("pageSize", strconv.Itoa(s.messageLimit))
	values.Add("startTime", "1")
	endpointURL.RawQuery = values.Encode()

	s.appLogger().WithFields(logrus.Fields{
		"conversation_id":    target.ID,
		"conversation_title": target.Title,
		"message_limit":      s.messageLimit,
	}).Debug("fetching conversation messages")

	var lastErr error
	for attempt := 1; attempt <= messageFetchMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		attemptCtx, cancel := context.WithTimeout(ctx, messageRequestTimeout)
		messages, err := s.fetchMessagesOnce(attemptCtx, endpointURL.String())
		cancel()
		if err == nil {
			return messages, nil
		}
		if errors.Is(err, context.Canceled) {
			return nil, err
		}

		lastErr = err
		if attempt == messageFetchMaxAttempts || !shouldRetryMessageFetch(err) {
			return nil, err
		}

		s.appLogger().WithFields(logrus.Fields{
			"conversation_id": target.ID,
			"attempt":         attempt,
			"max_attempts":    messageFetchMaxAttempts,
			"backoff":         messageFetchBackoff(attempt).String(),
		}).WithError(err).Warn("retrying message fetch")
		if err := s.waitForMessageFetchRetry(ctx, attempt); err != nil {
			return nil, err
		}
	}

	return nil, lastErr
}

func (s *AppState) fetchMessagesOnce(ctx context.Context, endpoint string) ([]csa.ChatMessage, error) {
	req, err := s.teamsClient.ChatSvc().AuthenticatedRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)

	client := s.httpClient
	if client == nil {
		client = newMessageHTTPClient()
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, &messageFetchStatusError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       strings.TrimSpace(string(bodyBytes)),
		}
	}

	var msgResponse csa.MessagesResponse
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&msgResponse); err != nil {
		return nil, fmt.Errorf("unable to decode messages: %v", err)
	}

	messages := msgResponse.Messages
	sort.Sort(csa.SortMessageByTime(messages))
	if len(messages) > s.messageLimit {
		messages = messages[len(messages)-s.messageLimit:]
	}

	return messages, nil
}
