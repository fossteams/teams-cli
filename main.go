package main

import (
	"fmt"
	"github.com/rivo/tview"
	"github.com/sirupsen/logrus"
	"os"
)

func main() {
	options, err := parseAppOptions(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "usage: %s [msg=<count>]\nerror: %v\n", os.Args[0], err)
		os.Exit(2)
	}

	app := tview.NewApplication()
	logger := logrus.New()

	state := AppState{
		app:          app,
		logger:       logger,
		messageLimit: options.MessageLimit,
	}

	state.createApp()
	if err := app.EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}
