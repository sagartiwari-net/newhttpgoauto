package portalchrome

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gohttpauto/internal/automation/httpclient"
	"gohttpauto/internal/cookiesession"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
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
	VisibleEnv      string // e.g. NOX_VISIBLE=1 → set env var name only: NOX_VISIBLE
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

	cookies, err := chromeLogin(ctx, username, password, cfg)
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

func chromeLogin(ctx context.Context, username, password string, cfg SessionConfig) ([]cookiesession.Cookie, error) {
	pDir := profileDir(cfg.Profile)
	_ = os.MkdirAll(pDir, 0755)
	for _, lf := range []string{"SingletonLock", "SingletonCookie", "SingletonSocket"} {
		_ = os.Remove(filepath.Join(pDir, lf))
	}

	headless := true
	if cfg.VisibleEnv != "" && os.Getenv(cfg.VisibleEnv) == "1" {
		headless = false
	}
	log.Printf("%s launching Chrome (headless=%v, profile=%s)", cfg.Tag, headless, pDir)

	l := launcher.New().
		Headless(headless).
		Set("no-sandbox").
		Set("disable-setuid-sandbox").
		Set("disable-dev-shm-usage").
		Set("disable-gpu").
		Set("disable-blink-features", "AutomationControlled").
		Set("mute-audio").
		Set("disable-popup-blocking").
		Set("window-size", "1366,768").
		UserDataDir(pDir)

	u, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("chrome launch: %w", err)
	}

	browser := rod.New().ControlURL(u).MustConnect().Context(ctx)
	defer browser.MustClose()

	page := stealth.MustPage(browser)
	page.MustSetViewport(1366, 768, 1, false)
	page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	})

	cookieFile := loginCookieFile(cfg.Profile)
	if cBytes, err := os.ReadFile(cookieFile); err == nil {
		var params []*proto.NetworkCookieParam
		if json.Unmarshal(cBytes, &params) == nil && len(params) > 0 {
			_ = page.SetCookies(params)
			log.Printf("%s restored %d Chrome profile cookies", cfg.Tag, len(params))
		}
	}

	if err := page.Timeout(30 * time.Second).Navigate(cfg.MemberURL); err != nil {
		log.Printf("%s navigation warning: %v", cfg.Tag, err)
	}
	time.Sleep(2 * time.Second)

	if waitCloudflare(page, cfg.Tag) {
		time.Sleep(2 * time.Second)
	}

	loggedIn, needForm := pollLoginState(page, 30*time.Second)
	if !loggedIn && !needForm && cfg.LoginURL != "" {
		log.Printf("%s opening login page %s", cfg.Tag, cfg.LoginURL)
		_ = page.Timeout(30 * time.Second).Navigate(cfg.LoginURL)
		time.Sleep(2 * time.Second)
		loggedIn, needForm = pollLoginState(page, 20*time.Second)
	}
	if loggedIn {
		log.Printf("%s already logged in via Chrome profile", cfg.Tag)
		return exportCookies(page, cfg)
	}
	if !needForm {
		return nil, fmt.Errorf("login form not found on portal")
	}

	log.Printf("%s filling login form", cfg.Tag)
	res, err := page.Eval(`(u, p) => {
		const loginEl = document.querySelector('input[name="amember_login"]')
			|| document.querySelector('#amember-login')
			|| document.querySelector('input[type="email"]');
		const passEl = document.querySelector('input[name="amember_pass"]')
			|| document.querySelector('#amember-pass')
			|| document.querySelector('input[type="password"]');
		if (!loginEl || !passEl) return false;
		loginEl.value = u;
		passEl.value = p;
		['input', 'change'].forEach(ev => {
			loginEl.dispatchEvent(new Event(ev, { bubbles: true }));
			passEl.dispatchEvent(new Event(ev, { bubbles: true }));
		});
		const rememberEl = document.querySelector('input[name="remember_login"]') || document.querySelector('input[type="checkbox"]');
		if (rememberEl) {
			rememberEl.checked = true;
			rememberEl.dispatchEvent(new Event('change', { bubbles: true }));
		}
		return true;
	}`, username, password)
	if err != nil || res == nil || !res.Value.Bool() {
		return nil, fmt.Errorf("could not fill login form")
	}

	time.Sleep(1 * time.Second)
	_, _ = page.Eval(`() => {
		const btn = document.querySelector('#loginBtn')
			|| document.querySelector('#login-submit-button')
			|| document.querySelector('#submit')
			|| document.querySelector('button[type="submit"]')
			|| document.querySelector('input[type="submit"]');
		if (btn) { btn.click(); return true; }
		const loginInput = document.querySelector('input[name="amember_login"]');
		if (loginInput && loginInput.form) { loginInput.form.submit(); return true; }
		return false;
	}`)

	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if logged, _ := pollLoginState(page, 2*time.Second); logged {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if logged, _ := pollLoginState(page, 2*time.Second); !logged {
		return nil, fmt.Errorf("login failed — still on login page")
	}

	cookies, err := exportCookies(page, cfg)
	if err != nil {
		return nil, err
	}
	if raw, err := page.Cookies([]string{}); err == nil {
		if b, err := json.Marshal(proto.CookiesToParams(raw)); err == nil {
			_ = os.MkdirAll(filepath.Dir(cookieFile), 0755)
			_ = os.WriteFile(cookieFile, b, 0644)
		}
	}
	return cookies, nil
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
