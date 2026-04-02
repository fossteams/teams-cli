package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestPlatformLogFilePathPrefersXDGStateHomeOnLinux(t *testing.T) {
	got, err := platformLogFilePath("linux", "/home/alice", "/state", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join("/state", "teams-cli", defaultLogFileName)
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestPlatformLogFilePathUsesMacLibraryLogs(t *testing.T) {
	got, err := platformLogFilePath("darwin", "/Users/alice", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join("/Users/alice", "Library", "Logs", "teams-cli", defaultLogFileName)
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestOpenStructuredLogFileCreatesParentDirectory(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "nested", "logs", "teams-cli.log")
	logFile, err := openStructuredLogFile(logPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer logFile.Close()

	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("expected log file to exist: %v", err)
	}
}

func TestRedactingJSONFormatterRedactsSensitiveValues(t *testing.T) {
	logger := logrus.New()
	entry := logrus.NewEntry(logger)
	entry.Message = "Authorization=Bearer eyJhbGciOiJIUzI1NiJ9.payload.signature query=?token=abc123"
	entry.Data = logrus.Fields{
		"authorization": "Bearer secret-value",
		"log_file":      "/tmp/teams-cli.log",
		"error":         "MS_TEAMS_SKYPE_TOKEN=raw-token-value",
	}

	formatted, err := newRedactingJSONFormatter().Format(entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := string(formatted)
	if strings.Contains(output, "secret-value") || strings.Contains(output, "raw-token-value") || strings.Contains(output, "abc123") {
		t.Fatalf("expected sensitive values to be redacted, got %q", output)
	}
	if !strings.Contains(output, redactedLogValue) {
		t.Fatalf("expected redaction marker in output, got %q", output)
	}
	if !strings.Contains(output, "/tmp/teams-cli.log") {
		t.Fatalf("expected non-sensitive log path to remain visible, got %q", output)
	}
}
