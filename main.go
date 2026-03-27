package main

import (
	"context"
	"fmt"
	"github.com/rivo/tview"
	"github.com/sirupsen/logrus"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	options, err := parseAppOptions(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "usage: %s [msg=<count>]\nerror: %v\n", os.Args[0], err)
		os.Exit(2)
	}

	app := tview.NewApplication()
	logger := logrus.New()
	rootCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	state := AppState{
		app:          app,
		logger:       logger,
		messageLimit: options.MessageLimit,
	}
	state.initRuntime(rootCtx)
	defer state.requestStop()

	go func() {
		<-state.appContext().Done()
		app.Stop()
	}()

	state.createApp()
	if err := app.EnableMouse(true).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "teams-cli failed: %v\n", err)
		os.Exit(1)
	}
}
