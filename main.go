package main

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"
	"os"
)

var InitialState = NewStateLogin()
var color = termenv.ColorProfile().Color

func main(){
	p := tea.NewProgram(&InitialState)
	if err := p.Start(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}