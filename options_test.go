package main

import (
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestParseAppOptionsDefaults(t *testing.T) {
	options, err := parseAppOptions(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if options.MessageLimit != defaultMessageLimit {
		t.Fatalf("expected default message limit %d, got %d", defaultMessageLimit, options.MessageLimit)
	}
	if options.LogLevel != logrus.InfoLevel {
		t.Fatalf("expected default log level info, got %s", options.LogLevel)
	}
	if !options.LiveRefresh {
		t.Fatal("expected live refresh to be enabled by default")
	}
	if options.RefreshMessagesInterval != defaultLiveMessageRefreshInterval {
		t.Fatalf("expected default message refresh interval %s, got %s", defaultLiveMessageRefreshInterval, options.RefreshMessagesInterval)
	}
	if options.RefreshConversationInterval != defaultLiveConversationRefreshInterval {
		t.Fatalf("expected default tree refresh interval %s, got %s", defaultLiveConversationRefreshInterval, options.RefreshConversationInterval)
	}
}

func TestParseAppOptionsExtendedFlags(t *testing.T) {
	options, err := parseAppOptions([]string{
		"--msg", "25",
		"--debug",
		"--token-dir", "/tmp/tokens",
		"--refresh-messages", "10",
		"--refresh-tree=45s",
		"--no-live",
		"doctor",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if options.MessageLimit != 25 {
		t.Fatalf("expected message limit 25, got %d", options.MessageLimit)
	}
	if options.LogLevel != logrus.DebugLevel {
		t.Fatalf("expected debug log level, got %s", options.LogLevel)
	}
	if options.TokenDir != "/tmp/tokens" {
		t.Fatalf("expected token dir /tmp/tokens, got %q", options.TokenDir)
	}
	if options.RefreshMessagesInterval != 10*time.Second {
		t.Fatalf("expected message refresh 10s, got %s", options.RefreshMessagesInterval)
	}
	if options.RefreshConversationInterval != 45*time.Second {
		t.Fatalf("expected tree refresh 45s, got %s", options.RefreshConversationInterval)
	}
	if options.LiveRefresh {
		t.Fatal("expected live refresh to be disabled")
	}
	if !options.DoctorMode {
		t.Fatal("expected doctor mode to be enabled")
	}
}

func TestParseAppOptionsLegacyMessageLimit(t *testing.T) {
	options, err := parseAppOptions([]string{"msg=25"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if options.MessageLimit != 25 {
		t.Fatalf("expected message limit 25, got %d", options.MessageLimit)
	}
}

func TestParseAppOptionsHelpAndVersion(t *testing.T) {
	options, err := parseAppOptions([]string{"--help", "--version"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !options.ShowHelp {
		t.Fatal("expected help mode")
	}
	if !options.ShowVersion {
		t.Fatal("expected version mode")
	}
}

func TestParseAppOptionsDebugFlagCanBeOverridden(t *testing.T) {
	options, err := parseAppOptions([]string{"--debug", "--log-level", "error"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if options.LogLevel != logrus.ErrorLevel {
		t.Fatalf("expected error log level, got %s", options.LogLevel)
	}
}

func TestParseAppOptionsSupportsEqualsForms(t *testing.T) {
	options, err := parseAppOptions([]string{
		"--msg=12",
		"--log-level=warn",
		"--token-dir=/tmp/teams-cli-tokens",
		"--refresh-messages=12s",
		"--refresh-tree=30",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if options.MessageLimit != 12 {
		t.Fatalf("expected message limit 12, got %d", options.MessageLimit)
	}
	if options.LogLevel != logrus.WarnLevel {
		t.Fatalf("expected warn level, got %s", options.LogLevel)
	}
	if options.TokenDir != "/tmp/teams-cli-tokens" {
		t.Fatalf("expected token dir to be parsed, got %q", options.TokenDir)
	}
	if options.RefreshMessagesInterval != 12*time.Second {
		t.Fatalf("expected 12s message refresh interval, got %s", options.RefreshMessagesInterval)
	}
	if options.RefreshConversationInterval != 30*time.Second {
		t.Fatalf("expected 30s tree refresh interval, got %s", options.RefreshConversationInterval)
	}
}

func TestParseFlexibleDurationNumericSeconds(t *testing.T) {
	got, err := parseFlexibleDuration("30")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 30*time.Second {
		t.Fatalf("expected 30s, got %s", got)
	}
}

func TestParseAppOptionsRejectsInvalidMessageLimit(t *testing.T) {
	_, err := parseAppOptions([]string{"msg=0"})
	if err == nil {
		t.Fatal("expected an error for msg=0")
	}
}

func TestParseAppOptionsRejectsInvalidRefreshInterval(t *testing.T) {
	_, err := parseAppOptions([]string{"--refresh-messages", "0"})
	if err == nil {
		t.Fatal("expected an error for zero refresh interval")
	}
}

func TestParseAppOptionsRejectsMissingFlagValues(t *testing.T) {
	testCases := []string{
		"--msg",
		"--log-level",
		"--token-dir",
		"--refresh-messages",
		"--refresh-tree",
	}

	for _, arg := range testCases {
		if _, err := parseAppOptions([]string{arg}); err == nil {
			t.Fatalf("expected an error for %s without a value", arg)
		}
	}
}

func TestParseAppOptionsRejectsUnknownArgument(t *testing.T) {
	_, err := parseAppOptions([]string{"foo=bar"})
	if err == nil {
		t.Fatal("expected an error for an unknown argument")
	}
}

func TestUsageTextIncludesDiagnosticsAndRefreshFlags(t *testing.T) {
	text := usageText("teams-cli")

	for _, needle := range []string{"--doctor", "--no-live", "--refresh-messages", "--refresh-tree"} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected usage text to mention %s", needle)
		}
	}
}
