package main

import (
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
		"--log-level=debug",
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

func TestParseAppOptionsRejectsUnknownArgument(t *testing.T) {
	_, err := parseAppOptions([]string{"foo=bar"})
	if err == nil {
		t.Fatal("expected an error for an unknown argument")
	}
}
