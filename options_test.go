package main

import "testing"

func TestParseAppOptionsDefaults(t *testing.T) {
	options, err := parseAppOptions(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if options.MessageLimit != defaultMessageLimit {
		t.Fatalf("expected default message limit %d, got %d", defaultMessageLimit, options.MessageLimit)
	}
}

func TestParseAppOptionsMessageLimit(t *testing.T) {
	options, err := parseAppOptions([]string{"msg=25"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if options.MessageLimit != 25 {
		t.Fatalf("expected message limit 25, got %d", options.MessageLimit)
	}
}

func TestParseAppOptionsRejectsInvalidMessageLimit(t *testing.T) {
	_, err := parseAppOptions([]string{"msg=0"})
	if err == nil {
		t.Fatal("expected an error for msg=0")
	}
}

func TestParseAppOptionsRejectsUnknownArgument(t *testing.T) {
	_, err := parseAppOptions([]string{"foo=bar"})
	if err == nil {
		t.Fatal("expected an error for an unknown argument")
	}
}
