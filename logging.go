package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	defaultLogFileName = "teams-cli.log"
	redactedLogValue   = "<redacted>"
)

var (
	redactJWTPattern              = regexp.MustCompile(`eyJ[0-9A-Za-z_-]+\.[0-9A-Za-z_-]+\.[0-9A-Za-z_-]+`)
	redactBearerPattern           = regexp.MustCompile(`(?i)\b(Bearer|skypetoken)\s+[^\s,;]+`)
	redactHeaderSecretPattern     = regexp.MustCompile(`(?i)\b(authorization|authentication|cookie)(\s*[:=]\s*)([^\s,;]+)`)
	redactEnvTokenPattern         = regexp.MustCompile(`(?i)\b(MS_TEAMS_[A-Z0-9_]+_TOKEN)(=)([^\s]+)`)
	redactQuerySecretPattern      = regexp.MustCompile(`(?i)([?&](?:access_token|refresh_token|id_token|token|assertion|password|secret)=)([^&\s]+)`)
	redactAssignmentSecretPattern = regexp.MustCompile(`(?i)\b((?:access_token|refresh_token|id_token|assertion|password|secret|session_token))(\s*[:=]\s*)([^\s,;]+)`)
	discardLogger                 = newDiscardLogger()
)

type loggerSetup struct {
	Logger *logrus.Logger
	Path   string
	file   *os.File
}

func (s loggerSetup) Close() error {
	if s.file == nil {
		return nil
	}

	return s.file.Close()
}

type redactingJSONFormatter struct {
	base *logrus.JSONFormatter
}

func newDiscardLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return logger
}

func newRedactingJSONFormatter() logrus.Formatter {
	return &redactingJSONFormatter{
		base: &logrus.JSONFormatter{
			TimestampFormat:   time.RFC3339Nano,
			DisableHTMLEscape: true,
		},
	}
}

func (f *redactingJSONFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	cloned := *entry
	cloned.Message = redactLogString(entry.Message)
	cloned.Data = make(logrus.Fields, len(entry.Data))
	for key, value := range entry.Data {
		cloned.Data[key] = redactLogValue(key, value)
	}

	return f.base.Format(&cloned)
}

func defaultLogFilePath() (string, error) {
	homeDir, _ := os.UserHomeDir()
	cacheDir, _ := os.UserCacheDir()
	path, err := platformLogFilePath(runtime.GOOS, homeDir, strings.TrimSpace(os.Getenv("XDG_STATE_HOME")), cacheDir)
	if err != nil {
		return "", fmt.Errorf("unable to determine default log file path: %v", err)
	}

	return path, nil
}

func platformLogFilePath(goos, homeDir, xdgStateHome, cacheDir string) (string, error) {
	switch goos {
	case "darwin":
		if strings.TrimSpace(homeDir) == "" {
			return "", fmt.Errorf("home directory is unavailable")
		}
		return filepath.Join(homeDir, "Library", "Logs", "teams-cli", defaultLogFileName), nil
	case "windows":
		if strings.TrimSpace(cacheDir) != "" {
			return filepath.Join(cacheDir, "teams-cli", "logs", defaultLogFileName), nil
		}
		if strings.TrimSpace(homeDir) != "" {
			return filepath.Join(homeDir, "AppData", "Local", "teams-cli", "logs", defaultLogFileName), nil
		}
	default:
		if strings.TrimSpace(xdgStateHome) != "" {
			return filepath.Join(xdgStateHome, "teams-cli", defaultLogFileName), nil
		}
		if strings.TrimSpace(homeDir) != "" {
			return filepath.Join(homeDir, ".local", "state", "teams-cli", defaultLogFileName), nil
		}
	}

	if strings.TrimSpace(cacheDir) != "" {
		return filepath.Join(cacheDir, "teams-cli", "logs", defaultLogFileName), nil
	}

	return "", fmt.Errorf("no user-local log directory is available")
}

func openStructuredLogFile(path string) (*os.File, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}

	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
}

func setupLogger(options AppOptions) (loggerSetup, error) {
	logPath, err := defaultLogFilePath()
	if err != nil {
		return loggerSetup{}, err
	}

	logFile, err := openStructuredLogFile(logPath)
	if err != nil {
		return loggerSetup{}, fmt.Errorf("unable to open log file %s: %v", logPath, err)
	}

	logger := logrus.New()
	logger.SetLevel(options.LogLevel)
	logger.SetOutput(logFile)
	logger.SetFormatter(newRedactingJSONFormatter())
	logger.SetReportCaller(options.LogLevel == logrus.DebugLevel)

	return loggerSetup{
		Logger: logger,
		Path:   logPath,
		file:   logFile,
	}, nil
}

func isSensitiveLogKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	if lower == "" {
		return false
	}

	if strings.HasSuffix(lower, "_dir") || strings.Contains(lower, "path") || strings.Contains(lower, "file") || strings.HasSuffix(lower, "_type") {
		return false
	}

	if strings.Contains(lower, "authorization") || strings.Contains(lower, "authentication") || strings.Contains(lower, "cookie") {
		return true
	}

	if strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "jwt") {
		return true
	}

	if lower == "token" || strings.HasSuffix(lower, "_token") || strings.HasPrefix(lower, "token_") {
		return true
	}

	return false
}

func redactLogValue(key string, value any) any {
	if isSensitiveLogKey(key) {
		return redactedLogValue
	}

	switch typed := value.(type) {
	case string:
		return redactLogString(typed)
	case error:
		return redactLogString(typed.Error())
	case []byte:
		if len(typed) == 0 {
			return ""
		}
		return redactedLogValue
	case []string:
		redacted := make([]string, len(typed))
		for idx, item := range typed {
			redacted[idx] = redactLogString(item)
		}
		return redacted
	case []any:
		redacted := make([]any, len(typed))
		for idx, item := range typed {
			redacted[idx] = redactLogValue("", item)
		}
		return redacted
	case logrus.Fields:
		redacted := logrus.Fields{}
		for childKey, childValue := range typed {
			redacted[childKey] = redactLogValue(childKey, childValue)
		}
		return redacted
	case map[string]any:
		redacted := map[string]any{}
		for childKey, childValue := range typed {
			redacted[childKey] = redactLogValue(childKey, childValue)
		}
		return redacted
	case fmt.Stringer:
		return redactLogString(typed.String())
	default:
		return typed
	}
}

func redactLogString(value string) string {
	if value == "" {
		return ""
	}

	redacted := redactJWTPattern.ReplaceAllString(value, redactedLogValue)
	redacted = redactHeaderSecretPattern.ReplaceAllString(redacted, "$1$2"+redactedLogValue)
	redacted = redactEnvTokenPattern.ReplaceAllString(redacted, "$1$2"+redactedLogValue)
	redacted = redactQuerySecretPattern.ReplaceAllString(redacted, "$1"+redactedLogValue)
	redacted = redactAssignmentSecretPattern.ReplaceAllString(redacted, "$1$2"+redactedLogValue)
	redacted = redactBearerPattern.ReplaceAllStringFunc(redacted, func(match string) string {
		parts := strings.Fields(match)
		if len(parts) == 0 {
			return redactedLogValue
		}
		return parts[0] + " " + redactedLogValue
	})

	return redacted
}
