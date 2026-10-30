package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const defaultMessageLimit = 200

type AppOptions struct {
	MessageLimit                int
	LogLevel                    logrus.Level
	TokenDir                    string
	LiveRefresh                 bool
	RefreshMessagesInterval     time.Duration
	RefreshConversationInterval time.Duration
	ShowHelp                    bool
	ShowVersion                 bool
	DoctorMode                  bool
	RefreshTokens               bool
	ForceHeaded                 bool
	BrowserPath                 string
	ProfileDir                  string
	Logout                      bool
	SilentOnly                  bool
}

func defaultAppOptions() AppOptions {
	return AppOptions{
		MessageLimit:                defaultMessageLimit,
		LogLevel:                    logrus.InfoLevel,
		LiveRefresh:                 true,
		RefreshMessagesInterval:     defaultLiveMessageRefreshInterval,
		RefreshConversationInterval: defaultLiveConversationRefreshInterval,
	}
}

func parseAppOptions(args []string) (AppOptions, error) {
	options := defaultAppOptions()

	for idx := 0; idx < len(args); idx++ {
		arg := args[idx]

		switch {
		case arg == "doctor" || arg == "--doctor":
			options.DoctorMode = true
		case arg == "refresh-token" || arg == "refresh-tokens" || arg == "login":
			options.RefreshTokens = true
		case arg == "--headed":
			options.ForceHeaded = true
		case arg == "--silent":
			options.SilentOnly = true
		case arg == "--logout":
			options.Logout = true
		case arg == "--browser" || strings.HasPrefix(arg, "--browser="):
			raw, nextIdx, err := optionValue(args, idx, "--browser")
			if err != nil {
				return options, err
			}
			options.BrowserPath = strings.TrimSpace(raw)
			idx = nextIdx
		case arg == "--profile-dir" || strings.HasPrefix(arg, "--profile-dir="):
			raw, nextIdx, err := optionValue(args, idx, "--profile-dir")
			if err != nil {
				return options, err
			}
			options.ProfileDir = strings.TrimSpace(raw)
			idx = nextIdx
		case arg == "--help" || arg == "-h":
			options.ShowHelp = true
		case arg == "--version":
			options.ShowVersion = true
		case arg == "--debug":
			options.LogLevel = logrus.DebugLevel
		case arg == "--no-live":
			options.LiveRefresh = false
		case strings.HasPrefix(arg, "msg="):
			limit, err := parseMessageLimit(strings.TrimPrefix(arg, "msg="))
			if err != nil {
				return options, err
			}
			options.MessageLimit = limit
		case arg == "--msg" || strings.HasPrefix(arg, "--msg="):
			raw, nextIdx, err := optionValue(args, idx, "--msg")
			if err != nil {
				return options, err
			}
			limit, err := parseMessageLimit(raw)
			if err != nil {
				return options, err
			}
			options.MessageLimit = limit
			idx = nextIdx
		case arg == "--log-level" || strings.HasPrefix(arg, "--log-level="):
			raw, nextIdx, err := optionValue(args, idx, "--log-level")
			if err != nil {
				return options, err
			}
			level, err := logrus.ParseLevel(strings.ToLower(strings.TrimSpace(raw)))
			if err != nil {
				return options, fmt.Errorf("invalid log level %q", raw)
			}
			options.LogLevel = level
			idx = nextIdx
		case arg == "--token-dir" || strings.HasPrefix(arg, "--token-dir="):
			raw, nextIdx, err := optionValue(args, idx, "--token-dir")
			if err != nil {
				return options, err
			}
			raw = strings.TrimSpace(raw)
			if raw == "" {
				return options, fmt.Errorf("invalid token directory %q", raw)
			}
			options.TokenDir = raw
			idx = nextIdx
		case arg == "--refresh-messages" || strings.HasPrefix(arg, "--refresh-messages="):
			raw, nextIdx, err := optionValue(args, idx, "--refresh-messages")
			if err != nil {
				return options, err
			}
			duration, err := parseRefreshInterval(raw, "--refresh-messages")
			if err != nil {
				return options, err
			}
			options.RefreshMessagesInterval = duration
			idx = nextIdx
		case arg == "--refresh-tree" || strings.HasPrefix(arg, "--refresh-tree="):
			raw, nextIdx, err := optionValue(args, idx, "--refresh-tree")
			if err != nil {
				return options, err
			}
			duration, err := parseRefreshInterval(raw, "--refresh-tree")
			if err != nil {
				return options, err
			}
			options.RefreshConversationInterval = duration
			idx = nextIdx
		default:
			return options, fmt.Errorf("unknown argument %q", arg)
		}
	}

	if options.SilentOnly && options.ForceHeaded {
		return options, fmt.Errorf("--silent and --headed cannot be combined")
	}

	return options, nil
}

func optionValue(args []string, idx int, flagName string) (string, int, error) {
	arg := args[idx]
	if arg == flagName {
		if idx+1 >= len(args) {
			return "", idx, fmt.Errorf("missing value for %s", flagName)
		}
		return args[idx+1], idx + 1, nil
	}

	prefix := flagName + "="
	if strings.HasPrefix(arg, prefix) {
		return strings.TrimPrefix(arg, prefix), idx, nil
	}

	return "", idx, fmt.Errorf("missing value for %s", flagName)
}

func parseMessageLimit(raw string) (int, error) {
	limit, err := parsePositiveInt(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid msg value %q: expected a positive integer", raw)
	}

	return limit, nil
}

func parseRefreshInterval(raw, flagName string) (time.Duration, error) {
	duration, err := parseFlexibleDuration(raw)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("invalid value %q for %s: expected a positive duration", raw, flagName)
	}

	return duration, nil
}

func parseFlexibleDuration(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("duration cannot be empty")
	}

	if value, err := parsePositiveInt(raw); err == nil {
		return time.Duration(value) * time.Second, nil
	}

	return time.ParseDuration(raw)
}

func parsePositiveInt(raw string) (int, error) {
	var value int
	_, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &value)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("expected a positive integer")
	}

	if fmt.Sprintf("%d", value) != strings.TrimSpace(raw) {
		return 0, fmt.Errorf("expected a positive integer")
	}

	return value, nil
}

func usageText(program string) string {
	return fmt.Sprintf(`Usage:
  %s [options]
  %s doctor [options]
  %s refresh-token [--headed|--silent] [--browser <path>] [--profile-dir <dir>] [--logout]

Options:
  -h, --help                  Show this help text
      --debug                 Shortcut for --log-level debug
      --version               Show version information
      --msg <count>           Limit each conversation to the most recent N messages
      --log-level <level>     Set log level (debug, info, warn, error)
      --token-dir <dir>       Read token-teams.jwt, token-skype.jwt, and token-chatsvcagg.jwt from a custom directory
      --refresh-messages <d>  Poll interval for the selected conversation (seconds or Go duration, default %s)
      --refresh-tree <d>      Poll interval for the conversation tree (seconds or Go duration, default %s)
      --no-live               Disable background refresh polling
      --doctor                Run diagnostics instead of launching the TUI

refresh-token signs in to Teams with a Chrome-family browser and saves fresh
tokens. It first tries silently with the saved browser profile and only shows
a window when a sign-in is needed. The TUI runs the same refresh on its own
when tokens expire; press R inside it to sign in again by hand.
      --headed                Always show the browser window
      --silent                Never show a window; fail if a sign-in is needed
      --browser <path>        Browser executable (default: Chrome, Chromium, Edge, or a Playwright Chromium)
      --profile-dir <dir>     Browser profile that remembers the sign-in
      --logout                Delete the browser profile and exit

Examples:
  %s --msg 20
  %s --token-dir ~/.config/fossteams --debug
  %s doctor --token-dir ~/.config/fossteams
  %s refresh-token
`, program, program, program, defaultLiveMessageRefreshInterval, defaultLiveConversationRefreshInterval, program, program, program, program)
}
