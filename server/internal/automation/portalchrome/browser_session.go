package portalchrome

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"gohttpauto/internal/cookiesession"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

// BrowserSession is a logged-in Chrome kept open for portal + tool capture in one run.
type BrowserSession struct {
	Browser *rod.Browser
	Page    *rod.Page
	cfg     SessionConfig
	cancel  context.CancelFunc
}

// OpenLoggedIn launches visible Chrome (by default), logs into the portal, and returns the live session.
func OpenLoggedIn(ctx context.Context, username, password string, cfg SessionConfig) (*BrowserSession, error) {
	cfg = normalizeCfg(cfg)
	browser, page, cancel, err := launchBrowser(ctx, cfg)
	if err != nil {
		return nil, err
	}
	sess := &BrowserSession{Browser: browser, Page: page, cfg: cfg, cancel: cancel}

	if err := loginOnPage(page, username, password, cfg); err != nil {
		sess.Close()
		return nil, err
	}

	cookies, err := exportCookies(page, cfg)
	if err != nil {
		sess.Close()
		return nil, err
	}
	if !hasMemberSessionCookies(cookies) {
		sess.Close()
		return nil, fmt.Errorf("login did not produce member session cookies (check credentials)")
	}

	if raw, err := page.Cookies([]string{}); err == nil {
		cookieFile := loginCookieFile(cfg.Profile)
		if b, err := json.Marshal(proto.CookiesToParams(raw)); err == nil {
			_ = os.MkdirAll(filepath.Dir(cookieFile), 0755)
			_ = os.WriteFile(cookieFile, b, 0644)
		}
	}

	_ = cookiesession.Save(ctx, cookiesession.SaveOptions{
		WebsiteID: cfg.PortalSessionID,
		Referer:   cfg.MemberURL,
		Cookies:   cookies,
	})

	log.Printf("%s portal login OK (browser visible=%v)", cfg.Tag, !headless(cfg))
	return sess, nil
}

func (s *BrowserSession) Close() {
	if s == nil {
		return
	}
	if keepOpen(s.cfg) {
		log.Printf("%s keeping browser open 45s for inspection...", s.cfg.Tag)
		time.Sleep(45 * time.Second)
	}
	if s.Browser != nil {
		_ = s.Browser.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
}

func headless(cfg SessionConfig) bool {
	if cfg.HeadlessEnv != "" && os.Getenv(cfg.HeadlessEnv) == "1" {
		return true
	}
	return !cfg.ShowBrowser
}

func keepOpen(cfg SessionConfig) bool {
	if cfg.KeepOpenEnv != "" && os.Getenv(cfg.KeepOpenEnv) == "1" {
		return true
	}
	return cfg.ShowBrowser
}

func launchBrowser(ctx context.Context, cfg SessionConfig) (*rod.Browser, *rod.Page, context.CancelFunc, error) {
	pDir := profileDir(cfg.Profile)
	_ = os.MkdirAll(pDir, 0755)
	for _, lf := range []string{"SingletonLock", "SingletonCookie", "SingletonSocket"} {
		_ = os.Remove(filepath.Join(pDir, lf))
	}

	hl := headless(cfg)
	log.Printf("%s launching Chrome (headless=%v, profile=%s)", cfg.Tag, hl, pDir)

	l := launcher.New().
		Headless(hl).
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
		return nil, nil, nil, fmt.Errorf("chrome launch: %w", err)
	}

	sessCtx, cancel := context.WithCancel(ctx)
	browser := rod.New().ControlURL(u).MustConnect().Context(sessCtx)
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
	return browser, page, cancel, nil
}

func loginOnPage(page *rod.Page, username, password string, cfg SessionConfig) error {
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
		return nil
	}
	if !needForm {
		return fmt.Errorf("login form not found on portal")
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
		return fmt.Errorf("could not fill login form")
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
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	if logged, _ := pollLoginState(page, 2*time.Second); !logged {
		return fmt.Errorf("login failed — still on login page")
	}
	return nil
}
