package main

import (
	"bytes"
	"fmt"
	teams_api "github.com/fossteams/teams-api"
	"github.com/fossteams/teams-api/pkg/csa"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/html"
	"strings"
	"time"
)

type AppState struct {
	app   *tview.Application
	pages *tview.Pages
	logger *logrus.Logger

	TeamsState
	components map[string]tview.Primitive
}

func (s AppState) createApp() {
	s.pages = tview.NewPages()
	s.components = map[string]tview.Primitive{}

	// Add pages
	s.pages.AddPage(PageLogin, s.createLoginPage(), true, false)
	s.pages.AddPage(PageMain, s.createMainView(), true, false)
	s.pages.AddPage(PageError, s.createErrorView(), true, false)

	frame := tview.NewFrame(s.pages)
	frame.SetBorder(true)
	frame.SetTitle("teams-cli")
	frame.SetBorder(true)
	frame.SetTitleAlign(tview.AlignCenter)

	s.app.SetRoot(frame, true)

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
	chatView := tview.NewList()
	chatView.SetBackgroundColor(tcell.ColorBlack)

	s.components[TrChat] = treeView
	s.components[ViChat] = chatView

	flex := tview.NewFlex().
		AddItem(treeView, 0, 1, false).
		AddItem(chatView, 0, 2, false)

	return flex
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
			AddItem(p,  10, 1, false).
			AddItem(nil, 0, 1, false), 30, 1, false).
		AddItem(nil, 0, 1, false)
}

func (s *AppState) start() {
	// Initialize Teams client
	var err error
	s.teamsClient, err = teams_api.New()
	if err != nil {
		s.showError(err)
		return
	}

	// Initialize Teams State
	err = s.TeamsState.init(s.teamsClient)
	if err != nil {
		s.showError(err)
		return
	}

	go s.fillMainWindow()
}

func (s *AppState) showError(err error) {
	val, ok := s.components[TvError]
	if !ok {
		s.logger.Fatalf("unable to show error on screen: %v", err)
		return
	}
	val.(*tview.TextView).SetText(err.Error())
	s.pages.SwitchToPage(PageError)
	s.app.Draw()
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
			AddItem(p,  10, 1, false).
			AddItem(nil, 0, 1, false), 60, 1, false).
		AddItem(nil, 0, 1, false)
}

func (s *AppState) fillMainWindow() {
	treeView := s.components[TrChat].(*tview.TreeView)
	teamsNode := tview.NewTreeNode("Teams")

	var firstNode *tview.TreeNode
	for _, t := range s.conversations.Teams {
		currentTeamTreeNode := tview.NewTreeNode(t.DisplayName)
		currentTeamTreeNode.SetReference(t)
		if firstNode == nil {
			firstNode = currentTeamTreeNode
		}

		for _, c := range t.Channels {
			currentChannelTreeNode := tview.NewTreeNode(c.DisplayName)
			currentChannelTreeNode.SetReference(c)
			currentChannelTreeNode.SetColor(tcell.ColorGreen)
			currentTeamTreeNode.AddChild(currentChannelTreeNode)
		}
		currentTeamTreeNode.CollapseAll()
		currentTeamTreeNode.SetColor(tcell.ColorBlue)

		teamsNode.AddChild(currentTeamTreeNode)
	}

	treeView.SetSelectedFunc(func(node *tview.TreeNode) {
		reference := node.GetReference()
		if reference == nil {
			return
		}

		children := node.GetChildren()
		if len(children) == 0 {
			channelRef := reference.(csa.Channel)
			s.components[ViChat].(*tview.List).
				SetTitle(channelRef.DisplayName).
				SetBorder(true).
				SetTitleAlign(tview.AlignCenter)

			// Load Conversations here
			go s.loadConversations(&channelRef)
		} else {
			// Collapse if visible, expand if collapsed.
			node.SetExpanded(!node.IsExpanded())
		}
	})

	treeView.SetRoot(teamsNode)
	treeView.SetCurrentNode(firstNode)

	s.pages.SwitchToPage(PageMain)
	s.app.SetFocus(treeView)
	s.app.Draw()
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

func (s *AppState) loadConversations(c *csa.Channel) {
	messages, err := s.teamsClient.GetMessages(c)
	if err != nil {
		s.showError(err)
		time.Sleep(5 * time.Second)
		s.pages.SwitchToPage(PageMain)
		s.app.Draw()
		return
	}

	// Clear chat
	chatList := s.components[ViChat].(*tview.List)
	chatList.Clear()

	for _, message := range messages {
		chatList.AddItem(textMessage(message.Content), message.From, 0, nil)
	}
	s.app.Draw()
}
