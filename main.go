package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/rivo/tview"
	"github.com/sirupsen/logrus"
)

func main() {
	program := filepath.Base(os.Args[0])
	options, err := parseAppOptions(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\nerror: %v\n", usageText(program), err)
		os.Exit(2)
	}
	if options.ShowHelp {
		printUsage(os.Stdout, program)
		return
	}
	if options.ShowVersion {
		printVersion(os.Stdout)
		return
	}
	if options.DoctorMode {
		if err := runDoctor(os.Stdout, options); err != nil {
			fmt.Fprintf(os.Stderr, "doctor: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if err := applyTokenDirToEnv(options.TokenDir); err != nil {
		fmt.Fprintf(os.Stderr, "token setup failed: %v\n", err)
		os.Exit(1)
	}

	app := tview.NewApplication()
	logger := logrus.New()
	logger.SetLevel(options.LogLevel)
	rootCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	state := AppState{
		app:                          app,
		logger:                       logger,
		messageLimit:                 options.MessageLimit,
		liveRefreshDisabled:          !options.LiveRefresh,
		liveMessageRefreshEvery:      options.RefreshMessagesInterval,
		liveConversationRefreshEvery: options.RefreshConversationInterval,
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

func printUsage(out io.Writer, program string) {
	fmt.Fprint(out, usageText(program))
}

func printVersion(out io.Writer) {
	fmt.Fprintf(out, "teams-cli %s\n", version)
}
