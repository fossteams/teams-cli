package main

import (
	"fmt"
	"strings"
	"sync/atomic"

	teams_api "github.com/fossteams/teams-api"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/sirupsen/logrus"
)

func (s *AppState) createApp() {
	s.ensureRuntime()
	s.pages = tview.NewPages()
	s.components = map[string]tview.Primitive{}

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
	s.pages.SwitchToPage(PageLogin)
	s.app.SetFocus(s.pages)

	go s.start()
}

func (s *AppState) createMainView() tview.Primitive {
	treeView := tview.NewTreeView()
	treeView.SetBorder(true)
	treeView.SetTitle("Conversations")
	treeView.SetTitleAlign(tview.AlignLeft)

	chatView := tview.NewTextView()
	chatView.SetDynamicColors(true)
	chatView.SetWrap(true)
	chatView.SetWordWrap(true)
	chatView.SetScrollable(true)
	chatView.SetBackgroundColor(tcell.ColorBlack)
	chatView.SetBorder(true)
	chatView.SetTitle("Messages")
	chatView.SetTitleAlign(tview.AlignCenter)

	helpView := tview.NewTextView()
	helpView.SetDynamicColors(true)
	helpView.SetWrap(true)
	helpView.SetTextAlign(tview.AlignLeft)
	helpView.SetBackgroundColor(tcell.ColorBlack)
	helpView.SetText(helpBarText(TrChat))

	compose := tview.NewInputField()
	compose.SetLabel("> ")
	compose.SetLabelColor(tcell.ColorYellow)
	compose.SetFieldBackgroundColor(tcell.ColorBlack)
	compose.SetFieldTextColor(tcell.ColorWhite)
	compose.SetDoneFunc(s.composeDone)

	s.components[TrChat] = treeView
	s.components[ViChat] = chatView
	s.components[TvHelp] = helpView
	s.components[InCompose] = compose

	// The compose line is collapsed to zero height until a reply is started.
	s.messageColumn = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(chatView, 0, 1, false).
		AddItem(compose, 0, 0, false)

	contentFlex := tview.NewFlex().
		AddItem(treeView, 0, 1, false).
		AddItem(s.messageColumn, 0, 2, false)

	treeView.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyTab, tcell.KeyBacktab:
			s.focusMessagesPane()
		case tcell.KeyEscape:
			s.handleTreeLeft(treeView, treeView.GetCurrentNode())
		}
	})
	treeView.SetInputCapture(s.treeKeyHandler(treeView))
	chatView.SetDoneFunc(func(key tcell.Key) {
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
	var err error
	s.teamsClient, err = teams_api.New()
	if err != nil {
		s.appLogger().WithError(err).Error("unable to initialize teams client")
		s.showError(err)
		return
	}

	err = s.TeamsState.init(s.teamsClient)
	if err != nil {
		s.appLogger().WithError(err).Error("unable to initialize teams state")
		s.showError(err)
		return
	}

	rootToken, err := resolveToken(tokenTypeSkype, s.tokenDir)
	if err != nil {
		s.appLogger().WithError(err).Error("unable to read skype token for messaging")
		s.showError(err)
		return
	}
	s.messaging, err = newMessagingSession(s.appContext(), s.httpClient, rootToken.Value, authzEndpoint)
	if err != nil {
		s.appLogger().WithError(err).Error("unable to start messaging session")
		s.showError(fmt.Errorf("unable to start messaging session: %v", err))
		return
	}
	s.appLogger().WithField("messages_host", s.messaging.host.String()).Info("messaging session ready")

	s.appLogger().WithFields(logrus.Fields{
		"teams_count":   len(s.conversations.Teams),
		"chat_count":    len(s.conversations.Chats),
		"display_names": len(s.displayNameMap()),
	}).Info("teams state initialized")
	s.logConversationDataIssues()

	select {
	case <-s.appContext().Done():
		return
	default:
	}

	go s.fillMainWindow()
}

func (s *AppState) logConversationDataIssues() {
	if s.nameLookupErr != nil {
		s.appLogger().WithError(s.nameLookupErr).Warn("some chat participant names could not be resolved")
	}
	if len(s.duplicateChatIDs) > 0 {
		s.appLogger().WithField("duplicate_chat_ids", s.duplicateChatIDs).Warn("conversations response listed the same chat more than once; duplicates dropped")
	}
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
