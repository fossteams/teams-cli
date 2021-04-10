package main

import (
	"github.com/rivo/tview"
	"github.com/sirupsen/logrus"
)

func main(){
	app := tview.NewApplication()
	logger := logrus.New()

	state := AppState{
		app: app,
		logger: logger,
	}

	state.createApp()
	if err := app.EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}