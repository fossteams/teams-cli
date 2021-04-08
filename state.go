package main

import tea "github.com/charmbracelet/bubbletea"

type State interface {
	Name() string
	Init() tea.Cmd
	Update(msg tea.Msg) (tea.Model, tea.Cmd)
	View() string
}