package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
)

func TestCookieJarRoundTripPromotesSessionCookies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "cookies.json")
	now := time.Unix(1_700_000_000, 0)
	cookies := []*network.Cookie{
		{Name: "ESTSAUTH", Value: "v1", Domain: ".login.microsoftonline.com", Path: "/", Secure: true, HTTPOnly: true, Session: true},
		{Name: "persist", Value: "v2", Domain: "teams.microsoft.com", Path: "/", Expires: float64(now.Add(48 * time.Hour).Unix())},
	}

	if err := saveCookies(path, cookies); err != nil {
		t.Fatalf("save: %v", err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600 cookie file, got %v err=%v", info, err)
	}

	params, err := loadCookieParams(path, now, 24*time.Hour)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(params) != 2 {
		t.Fatalf("expected 2 cookies restored, got %d", len(params))
	}

	var session, persistent *network.CookieParam
	for _, p := range params {
		switch p.Name {
		case "ESTSAUTH":
			session = p
		case "persist":
			persistent = p
		}
	}
	if session == nil || persistent == nil {
		t.Fatalf("expected both cookies, got %+v", params)
	}
	// The session cookie is given a concrete future expiry so Chrome writes it to disk.
	wantExpiry := float64(now.Add(24 * time.Hour).Unix())
	if float64(session.Expires) != wantExpiry {
		t.Fatalf("expected session cookie promoted to %v, got %v", wantExpiry, session.Expires)
	}
	if session.Domain != ".login.microsoftonline.com" || !session.Secure || !session.HTTPOnly {
		t.Fatalf("expected cookie attributes preserved, got %+v", session)
	}
	// A cookie that already had a far expiry keeps it.
	if float64(persistent.Expires) != float64(now.Add(48*time.Hour).Unix()) {
		t.Fatalf("expected persistent cookie expiry preserved, got %v", persistent.Expires)
	}
}

func TestLoadCookieParamsDropsExpiredCookies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookies.json")
	now := time.Unix(1_700_000_000, 0)
	cookies := []*network.Cookie{
		{Name: "old", Value: "v", Domain: "x.example", Path: "/", Expires: float64(now.Add(-time.Hour).Unix())},
		{Name: "fresh", Value: "v", Domain: "x.example", Path: "/", Expires: float64(now.Add(time.Hour).Unix())},
	}
	if err := saveCookies(path, cookies); err != nil {
		t.Fatal(err)
	}

	params, err := loadCookieParams(path, now, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(params) != 1 || params[0].Name != "fresh" {
		t.Fatalf("expected only the unexpired cookie, got %+v", params)
	}
}

func TestLoadCookieParamsMissingFileIsEmpty(t *testing.T) {
	params, err := loadCookieParams(filepath.Join(t.TempDir(), "none.json"), time.Now(), time.Hour)
	if err != nil || len(params) != 0 {
		t.Fatalf("expected empty result for missing file, got %v err=%v", params, err)
	}
}

func TestHasAuthCookiesRecognisesSignIn(t *testing.T) {
	without := []*network.Cookie{{Name: "misc", Domain: "teams.microsoft.com"}}
	with := append(without, &network.Cookie{Name: "ESTSAUTHPERSISTENT", Domain: ".login.microsoftonline.com"})
	if hasAuthCookies(without) {
		t.Fatal("did not expect auth cookies")
	}
	if !hasAuthCookies(with) {
		t.Fatal("expected ESTSAUTHPERSISTENT to count as an auth cookie")
	}
}
