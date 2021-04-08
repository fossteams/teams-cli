package main

import (
	"fmt"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	teams_api "github.com/fossteams/teams-api"
	"github.com/fossteams/teams-api/pkg/csa"
	"github.com/fossteams/teams-api/pkg/mt"
	"github.com/logrusorgru/aurora"
	"github.com/muesli/termenv"
)

type StateLogin struct {
	spinner       spinner.Model
	err           error
	quitting      bool
	teamsClient   *teams_api.TeamsClient
	conversations *csa.ConversationResponse
	ready         bool
	user          *mt.User
	status        string
}

func NewStateLogin() StateLogin {
	m := StateLogin{
		spinner: spinner.NewModel(),
		ready: false,
		user: nil,
	}
	m.spinner.Spinner = spinner.Points
	return m
}

func (m *StateLogin) Init() tea.Cmd {
	m.status = "logging in"
	go func() {
		var err error
		m.teamsClient, err = teams_api.New()
		if err != nil {
			m.err = fmt.Errorf("unable to initialize Teams library: %v", err)
			return
		}
		m.status = "getting conversations"
		m.conversations, err = m.teamsClient.GetConversations()
		if err != nil {
			m.err = fmt.Errorf("unable to get conversations: %v", err)
			return
		}
		m.status = "getting your profile"
		m.user, err = m.teamsClient.GetMe()
		if err != nil {
			m.err = fmt.Errorf("unable to get profile: %v", err)
		} else {
			m.ready = true
		}
	}()
	return spinner.Tick
}

func (m *StateLogin) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "q", "esc", "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			default:
				return m, nil
			}

		case error:
			m.err = msg
			return m, nil

		default:
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
	}
}

func (m *StateLogin) Name() string {
	return "login"
}

func (m *StateLogin) View() string {
	if m.ready && m.user != nil {
		return fmt.Sprintf("%s %s %s",
			aurora.Black(aurora.BgGreen(" SUCCESS ")),
			aurora.Green("log-in successful, welcome"),
			aurora.Magenta(m.user.DisplayName),
		)
	}
	if m.err != nil {
		return fmt.Sprintf("%s %s\nPress q to exit.",
			aurora.BgRed( " ERROR "),
			aurora.Red(m.err.Error()),
		)
	}
	s := termenv.String(m.spinner.View()).Foreground(color("205")).String()
	str := fmt.Sprintf("\n\n   %s %s...\n\n", s, m.status)
	return str
}

var _ State = &StateLogin{}
