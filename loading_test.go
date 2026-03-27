package main

import (
	"strings"
	"testing"
	"time"
)

func TestLoadingBarFrameWidthAndHead(t *testing.T) {
	bar := loadingBarFrame(0, 10)
	if len(bar) != 12 {
		t.Fatalf("expected bar length 12, got %d", len(bar))
	}
	if !strings.Contains(bar, ">") {
		t.Fatalf("expected bar to contain a head marker, got %q", bar)
	}
}

func TestLoadingStatusTextIncludesMessageLimit(t *testing.T) {
	mainText, secondaryText := loadingStatusText(25, time.Now().Add(-2*time.Second), 3)
	if !strings.Contains(mainText, "25") {
		t.Fatalf("expected main text to include the message limit, got %q", mainText)
	}
	if !strings.Contains(secondaryText, "Fetching recent messages from Teams") {
		t.Fatalf("unexpected secondary text: %q", secondaryText)
	}
}
