package main

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/fossteams/teams-api/pkg/csa"
	"github.com/sirupsen/logrus"
)

type doctorStatus string

const (
	doctorPass doctorStatus = "PASS"
	doctorWarn doctorStatus = "WARN"
	doctorFail doctorStatus = "FAIL"
)

type doctorCheck struct {
	Status  doctorStatus
	Name    string
	Details string
}

func runDoctor(out io.Writer, options AppOptions) error {
	checks := []doctorCheck{
		{
			Status:  doctorPass,
			Name:    "Build",
			Details: fmt.Sprintf("teams-cli %s on %s/%s", version, runtime.GOOS, runtime.GOARCH),
		},
	}

	checks = append(checks, doctorTerminalCheck())
	checks = append(checks, doctorLogFileCheck())
	checks = append(checks, doctorRefreshConfigChecks(options)...)
	checks = append(checks, doctorTokenChecks(options)...)
	checks = append(checks, doctorNetworkChecks()...)

	failures := 0
	warnings := 0
	for _, check := range checks {
		fmt.Fprintf(out, "[%s] %s: %s\n", check.Status, check.Name, check.Details)
		switch check.Status {
		case doctorFail:
			failures += 1
		case doctorWarn:
			warnings += 1
		}
	}

	fmt.Fprintf(out, "\nSummary: %d pass, %d warn, %d fail\n", len(checks)-warnings-failures, warnings, failures)
	if failures > 0 {
		return fmt.Errorf("doctor found %d failing checks", failures)
	}

	return nil
}

func doctorTerminalCheck() doctorCheck {
	term := strings.TrimSpace(os.Getenv("TERM"))
	switch {
	case term == "":
		return doctorCheck{Status: doctorWarn, Name: "Terminal", Details: "TERM is not set; interactive TUI rendering may be degraded"}
	case strings.EqualFold(term, "dumb"):
		return doctorCheck{Status: doctorWarn, Name: "Terminal", Details: "TERM=dumb; run teams-cli in a terminal with cursor addressing"}
	default:
		return doctorCheck{Status: doctorPass, Name: "Terminal", Details: fmt.Sprintf("TERM=%s", term)}
	}
}

func doctorRefreshConfigChecks(options AppOptions) []doctorCheck {
	status := doctorPass
	liveDetails := fmt.Sprintf("live refresh enabled, messages=%s, tree=%s", options.RefreshMessagesInterval, options.RefreshConversationInterval)
	if !options.LiveRefresh {
		status = doctorWarn
		liveDetails = "live refresh disabled via --no-live"
	}

	tokenDir := options.TokenDir
	if tokenDir == "" {
		defaultDir, err := defaultTokenDir()
		if err != nil {
			tokenDir = "<unavailable>"
		} else {
			tokenDir = defaultDir
		}
	}

	return []doctorCheck{
		{
			Status:  doctorPass,
			Name:    "Options",
			Details: fmt.Sprintf("msg=%d, log-level=%s, token-dir=%s, debug=%t", options.MessageLimit, options.LogLevel.String(), tokenDir, options.LogLevel == logrus.DebugLevel),
		},
		{
			Status:  status,
			Name:    "Live Refresh",
			Details: liveDetails,
		},
	}
}

func doctorLogFileCheck() doctorCheck {
	logPath, err := defaultLogFilePath()
	if err != nil {
		return doctorCheck{
			Status:  doctorFail,
			Name:    "Logs",
			Details: err.Error(),
		}
	}

	logFile, err := openStructuredLogFile(logPath)
	if err != nil {
		return doctorCheck{
			Status:  doctorFail,
			Name:    "Logs",
			Details: fmt.Sprintf("unable to open %s: %v", logPath, err),
		}
	}
	_ = logFile.Close()

	return doctorCheck{
		Status:  doctorPass,
		Name:    "Logs",
		Details: fmt.Sprintf("structured logs enabled at %s", logPath),
	}
}

func doctorTokenChecks(options AppOptions) []doctorCheck {
	checks := make([]doctorCheck, 0, len(runtimeTokenTypes)+len(optionalTokenTypes)+1)
	if options.TokenDir != "" {
		info, err := os.Stat(options.TokenDir)
		if err != nil {
			checks = append(checks, doctorCheck{
				Status:  doctorFail,
				Name:    "Token Directory",
				Details: fmt.Sprintf("unable to access %s: %v", options.TokenDir, err),
			})
		} else if !info.IsDir() {
			checks = append(checks, doctorCheck{
				Status:  doctorFail,
				Name:    "Token Directory",
				Details: fmt.Sprintf("%s is not a directory", options.TokenDir),
			})
		} else {
			checks = append(checks, doctorCheck{
				Status:  doctorPass,
				Name:    "Token Directory",
				Details: fmt.Sprintf("using explicit token directory %s", options.TokenDir),
			})
		}
	}

	for _, tokenType := range runtimeTokenTypes {
		checks = append(checks, doctorTokenCheck(tokenType, options.TokenDir, true))
	}
	for _, tokenType := range optionalTokenTypes {
		checks = append(checks, doctorTokenCheck(tokenType, options.TokenDir, false))
	}

	return checks
}

func doctorTokenCheck(tokenType, tokenDir string, required bool) doctorCheck {
	label := "Token " + tokenType
	token, err := resolveToken(tokenType, tokenDir)
	if err != nil {
		status := doctorWarn
		if required {
			status = doctorFail
		}
		return doctorCheck{
			Status:  status,
			Name:    label,
			Details: err.Error(),
		}
	}

	meta, err := parseJWTMetadata(token.Value)
	if err != nil {
		return doctorCheck{
			Status:  doctorFail,
			Name:    label,
			Details: fmt.Sprintf("%s: %v", token.Location, err),
		}
	}

	parts := []string{fmt.Sprintf("source=%s", token.Location)}
	if meta.Principal != "" {
		parts = append(parts, "principal="+meta.Principal)
	}
	if meta.Audience != "" {
		parts = append(parts, "aud="+meta.Audience)
	}

	status := doctorPass
	if meta.HasExpiry {
		parts = append(parts, "exp="+meta.ExpiresAt.Format(time.RFC3339))
		if time.Now().After(meta.ExpiresAt) {
			status = doctorWarn
			if required {
				status = doctorFail
			}
			parts = append(parts, "expired")
		}
	} else {
		status = doctorWarn
		parts = append(parts, "exp=<missing>")
	}

	return doctorCheck{
		Status:  status,
		Name:    label,
		Details: strings.Join(parts, ", "),
	}
}

func doctorNetworkChecks() []doctorCheck {
	targets := []string{
		"https://teams.microsoft.com/",
		csa.MessagesHost,
	}

	checks := make([]doctorCheck, 0, len(targets))
	for _, target := range targets {
		checks = append(checks, doctorNetworkCheck(target))
	}

	return checks
}

func doctorNetworkCheck(rawURL string) doctorCheck {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return doctorCheck{
			Status:  doctorFail,
			Name:    "Network " + rawURL,
			Details: fmt.Sprintf("invalid URL: %v", err),
		}
	}

	host := parsed.Hostname()
	if host == "" {
		return doctorCheck{
			Status:  doctorFail,
			Name:    "Network " + rawURL,
			Details: "host is empty",
		}
	}

	started := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, "443"), 3*time.Second)
	if err != nil {
		return doctorCheck{
			Status:  doctorFail,
			Name:    "Network " + host,
			Details: err.Error(),
		}
	}
	_ = conn.Close()

	return doctorCheck{
		Status:  doctorPass,
		Name:    "Network " + host,
		Details: fmt.Sprintf("reachable in %s", time.Since(started).Round(time.Millisecond)),
	}
}
