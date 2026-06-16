package portalchrome

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gohttpauto/internal/automation/httpclient"
	"gohttpauto/internal/cookiesession"

	"github.com/go-rod/rod"
)

// SessionConfig controls Chrome portal login + HTTP cookie handoff.
type SessionConfig struct {
	Tag             string // e.g. "[Nox]"
	Profile         string // profile folder name
	MemberURL       string
	LoginURL        string // optional — open if member page has no form/dashboard
	CookieURL       string // base URL for jar, e.g. https://noxtools.com/
	DomainFilter    string // e.g. noxtools.com
	PortalSessionID string // shared_sessions key
	ShowBrowser     bool   // true = visible Chrome window on worker Mac
	HeadlessEnv     string // force headless when env=1, e.g. NOX_HEADLESS
	KeepOpenEnv     string // keep browser open after task when env=1, e.g. NOX_KEEP_OPEN
	VisibleEnv      string // deprecated alias — if set and =1, ShowBrowser=true
}

// EnsureSession restores HTTP cookies when valid; otherwise logs in via Chrome.
func EnsureSession(ctx context.Context, client *httpclient.Client, username, password string, cfg SessionConfig) error {
	if saved, err := cookiesession.Load(ctx, cfg.PortalSessionID); err == nil && len(saved) > 0 {
		_ = client.SetCookies(cfg.CookieURL, cookiesession.ToHTTPCookies(saved))
		log.Printf("%s restored %d portal cookies for HTTP", cfg.Tag, len(saved))
	}

	body, status, _, err := client.GET(cfg.MemberURL, nil)
	if err == nil && status == 200 && isLoggedInHTML(body) && !httpclient.IsCloudflareChallenge(body) {
		log.Printf("%s HTTP session still valid — skipping Chrome login", cfg.Tag)
		return savePortalFromClient(ctx, client, cfg)
	}

	if err == nil && httpclient.IsCloudflareChallenge(body) {
		log.Printf("%s Cloudflare on HTTP — using Chrome login", cfg.Tag)
	} else if needsLoginHTML(body) {
		log.Printf("%s login required — using Chrome login", cfg.Tag)
	}

	cookies, err := loginAndExportCookies(ctx, username, password, cfg)
	if err != nil {
		return err
	}
	if len(cookies) == 0 {
		return fmt.Errorf("no portal cookies after Chrome login")
	}
	if !hasMemberSessionCookies(cookies) {
		return fmt.Errorf("Chrome login did not produce member session cookies (check credentials)")
	}

	applyPortalCookies(client, cfg, cookies)
	if err := cookiesession.Save(ctx, cookiesession.SaveOptions{
		WebsiteID: cfg.PortalSessionID,
		Referer:   cfg.MemberURL,
		Cookies:   cookies,
	}); err != nil {
		log.Printf("%s portal cookie DB save warning: %v", cfg.Tag, err)
	}

	body, status, finalURL, err := client.GET(cfg.MemberURL, map[string]string{"Referer": cfg.CookieURL})
	if err != nil {
		log.Printf("%s HTTP member check skipped (%v) — trusting Chrome session", cfg.Tag, err)
		return nil
	}
	if httpclient.IsCloudflareChallenge(body) {
		// HTTP client often re-triggers CF even with valid portal cookies; Chrome session is trusted.
		log.Printf("%s HTTP verify hit Cloudflare — trusting Chrome session cookies", cfg.Tag)
		return nil
	}
	if needsLoginHTML(body) {
		return fmt.Errorf("portal session invalid after Chrome login (HTTP still shows login form at %s)", finalURL)
	}
	if status != 200 {
		log.Printf("%s HTTP member status %d — trusting Chrome session", cfg.Tag, status)
		return nil
	}
	if !isLoggedInHTML(body) {
		log.Printf("%s HTTP member page has no standard dashboard markers — trusting Chrome session", cfg.Tag)
	}
	log.Printf("%s Chrome login OK — continuing with HTTP", cfg.Tag)
	return nil
}

func loginAndExportCookies(ctx context.Context, username, password string, cfg SessionConfig) ([]cookiesession.Cookie, error) {
	cfg = normalizeCfg(cfg)
	browser, page, cancel, err := launchBrowser(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer browser.MustClose()
	defer cancel()

	if err := loginOnPage(page, username, password, cfg); err != nil {
		return nil, err
	}
	return exportCookies(page, cfg)
}

func normalizeCfg(cfg SessionConfig) SessionConfig {
	if cfg.VisibleEnv != "" && os.Getenv(cfg.VisibleEnv) == "1" {
		cfg.ShowBrowser = true
	}
	return cfg
}

func applyPortalCookies(client *httpclient.Client, cfg SessionConfig, cookies []cookiesession.Cookie) {
	httpCookies := cookiesession.ToHTTPCookies(cookies)
	seen := map[string]bool{}
	for _, raw := range []string{cfg.CookieURL, cfg.MemberURL} {
		if raw == "" || seen[raw] {
			continue
		}
		seen[raw] = true
		_ = client.SetCookies(raw, httpCookies)
	}
}

func hasMemberSessionCookies(cookies []cookiesession.Cookie) bool {
	for _, c := range cookies {
		name := strings.ToLower(c.Name)
		if name == "amember_nr" || name == "amember_ru" || strings.HasPrefix(name, "amember_auth") {
			return true
		}
	}
	return false
}

func exportCookies(page *rod.Page, cfg SessionConfig) ([]cookiesession.Cookie, error) {
	raw, err := page.Cookies([]string{})
	if err != nil {
		return nil, err
	}
	filter := strings.ToLower(cfg.DomainFilter)
	var out []cookiesession.Cookie
	for _, c := range raw {
		if filter != "" && !strings.Contains(strings.ToLower(c.Domain), filter) {
			continue
		}
		ss := string(c.SameSite)
		exp := float64(c.Expires)
		out = append(out, cookiesession.Cookie{
			Domain:         c.Domain,
			ExpirationDate: exp,
			HostOnly:       !strings.HasPrefix(c.Domain, "."),
			HTTPOnly:       c.HTTPOnly,
			Name:           c.Name,
			Path:           c.Path,
			SameSite:       &ss,
			Secure:         c.Secure,
			Session:        exp == 0,
			StoreID:        "0",
			Value:          c.Value,
		})
	}
	return out, nil
}

func pollLoginState(page *rod.Page, timeout time.Duration) (loggedIn, formFound bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if waitCloudflare(page, "") {
			time.Sleep(2 * time.Second)
			continue
		}
		if has, _, _ := page.Has("a[href*='logout'], #menu-member, .amember-dashboard, .am-layout-content"); has {
			return true, false
		}
		if has, _, _ := page.Has("input[name='amember_login'], input[name='amember_pass'], input[type='password']"); has {
			return false, true
		}
		time.Sleep(1 * time.Second)
	}
	return false, false
}

func waitCloudflare(page *rod.Page, tag string) bool {
	info, err := page.Info()
	if err != nil || info == nil {
		return false
	}
	title := info.Title
	if strings.Contains(title, "Just a moment") || strings.Contains(title, "Checking your browser") {
		if tag != "" {
			log.Printf("%s Cloudflare challenge — waiting...", tag)
		}
		time.Sleep(5 * time.Second)
		return true
	}
	return false
}

func needsLoginHTML(body string) bool {
	lower := strings.ToLower(body)
	return strings.Contains(lower, `name="amember_login"`) ||
		strings.Contains(lower, `id="amember-login"`)
}

func isLoggedInHTML(body string) bool {
	if httpclient.IsCloudflareChallenge(body) {
		return false
	}
	if needsLoginHTML(body) {
		return false
	}
	lower := strings.ToLower(body)
	return strings.Contains(lower, "amember-dashboard") ||
		strings.Contains(lower, "menu-member") ||
		strings.Contains(lower, "secure/member") ||
		strings.Contains(lower, "/logout") ||
		strings.Contains(lower, "your tools") ||
		strings.Contains(lower, "your wallet")
}

func savePortalFromClient(ctx context.Context, client *httpclient.Client, cfg SessionConfig) error {
	raw, err := client.CookiesFor(cfg.CookieURL)
	if err != nil {
		return err
	}
	cookies := make([]cookiesession.Cookie, 0, len(raw))
	for _, c := range raw {
		if c == nil || c.Name == "" {
			continue
		}
		if cfg.DomainFilter != "" && !strings.Contains(strings.ToLower(c.Domain), strings.ToLower(cfg.DomainFilter)) {
			continue
		}
		exp := c.Expires
		if exp.IsZero() {
			exp = time.Now().Add(24 * time.Hour)
		}
		cookies = append(cookies, cookiesession.SimpleCookie(c.Domain, c.Name, c.Value, c.Secure, c.HttpOnly, exp))
	}
	if len(cookies) == 0 {
		return nil
	}
	return cookiesession.Save(ctx, cookiesession.SaveOptions{
		WebsiteID: cfg.PortalSessionID,
		Referer:   cfg.MemberURL,
		Cookies:   cookies,
	})
}
