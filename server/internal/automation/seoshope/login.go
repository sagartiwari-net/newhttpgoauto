package seoshope

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gohttpauto/internal/cookiesession"
	"gohttpauto/internal/db"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

const (
	memberURL        = "https://app.seoshope.com/member"
	semPageURL       = "https://app.seoshope.com/page/sem"
	portalSessionKey = "seoshope_login"
)

func ensureLoggedIn(ctx context.Context, s *Session, username, password string) error {
	page := s.EnsurePortalPage()

	if cBytes, err := os.ReadFile(loginCookieFile()); err == nil {
		var params []*proto.NetworkCookieParam
		if json.Unmarshal(cBytes, &params) == nil && len(params) > 0 {
			_ = page.SetCookies(params)
			log.Printf("[SEOShope] Restored %d saved portal cookies", len(params))
		}
	}

	_ = page.Timeout(30 * time.Second).Navigate(memberURL)
	time.Sleep(2 * time.Second)
	waitForPageReady(page, 30*time.Second)

	if isAuthenticated(page) || s.HasMemberCookies() {
		log.Println("[SEOShope] Already logged in — no login needed")
		s.MarkLoggedIn()
		_ = savePortalCookies(ctx, page)
		return nil
	}

	if !isLoginPage(page) {
		time.Sleep(2 * time.Second)
		page = s.EnsurePortalPage()
		if isAuthenticated(page) || s.HasMemberCookies() {
			log.Println("[SEOShope] Logged in after tab recovery — skipping login")
			s.MarkLoggedIn()
			return nil
		}
		// Last try: open member URL on fresh tab
		page = s.EnsurePortalPage()
		_ = page.Navigate(memberURL)
		time.Sleep(3 * time.Second)
		if isAuthenticated(page) || s.HasMemberCookies() {
			s.MarkLoggedIn()
			return nil
		}
		saveErrorScreenshot(page, "unexpected_page_before_login")
		return fmt.Errorf("login page not found (url=%s)", pageURL(page))
	}

	// Reload login page so Turnstile hijack runs before widget loads (goauto pattern).
	log.Println("[SEOShope] Reloading login page for Turnstile hijack")
	_ = page.Reload()
	time.Sleep(2 * time.Second)
	waitForPageReady(page, 20*time.Second)

	log.Println("[SEOShope] Performing fresh login...")
	fillLoginForm(page, username, password)
	time.Sleep(2 * time.Second)
	primeTurnstile(page)

	turnstileOK := waitTurnstile(page)
	if !turnstileOK {
		log.Println("[SEOShope] Turnstile token not received after 30s — submitting anyway (goauto)")
	}
	log.Println("[SEOShope] Submitting login form")
	submitLoginForm(page)

	if err := waitForLoginSuccess(page, 45*time.Second); err != nil {
		return err
	}
	if !hasMemberSessionCookie(page) {
		saveErrorScreenshot(page, "login_no_member_cookie")
		return fmt.Errorf("login failed: no member session cookie (amember_nr) — check credentials")
	}

	log.Println("[SEOShope] Login successful")
	s.MarkLoggedIn()
	if err := savePortalCookies(ctx, page); err != nil {
		log.Printf("[SEOShope] portal cookie save warning: %v", err)
	}
	return nil
}

func waitForPageReady(page *rod.Page, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		info, _ := page.Info()
		title := ""
		if info != nil {
			title = info.Title
		}
		if strings.Contains(title, "Just a moment") || strings.Contains(title, "Checking your browser") {
			solveCloudflare(page)
			time.Sleep(3 * time.Second)
			continue
		}
		if strings.Contains(strings.ToLower(title), "error has occurred") {
			clearPortalSession(page)
			_ = page.Navigate(memberURL)
			time.Sleep(2 * time.Second)
			continue
		}
		if isAuthenticated(page) || isLoginPage(page) {
			return true
		}
		time.Sleep(1 * time.Second)
	}
	saveErrorScreenshot(page, "page_ready_timeout")
	return false
}

func waitForLoginSuccess(page *rod.Page, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if isAuthenticated(page) && hasMemberSessionCookie(page) {
			return nil
		}
		if msg := loginErrorMessage(page); msg != "" {
			saveErrorScreenshot(page, "login_rejected")
			return fmt.Errorf("login failed: %s", msg)
		}
		time.Sleep(500 * time.Millisecond)
	}
	saveErrorScreenshot(page, "login_failed")
	return errLoginFailed
}

func isDashboard(page *rod.Page) bool {
	return isAuthenticated(page)
}

// isAuthenticated reports a logged-in member area (custom SEOShope theme or aMember).
func isAuthenticated(page *rod.Page) bool {
	if isLoginPage(page) {
		return false
	}
	if hasMemberSessionCookie(page) {
		return true
	}
	u := strings.ToLower(pageURL(page))
	if strings.Contains(u, "/member") && !strings.Contains(u, "/login") {
		if has, _, _ := page.Has("input[name='amember_login'], input[name='amember_pass']"); !has {
			return true
		}
	}
	if strings.Contains(u, "/page/sem") {
		if has, _, _ := page.Has("button.semmy-btn"); has {
			return true
		}
	}
	selectors := []string{
		"a[href*='logout']",
		"a[href*='log-out']",
		".am-user-info",
		".am-member-layout",
		".am-layout-content",
		"button.semmy-btn",
	}
	for _, sel := range selectors {
		if has, _, _ := page.Has(sel); has {
			return true
		}
	}
	res, err := page.Eval(`() => {
		if (document.querySelector('input[name="amember_login"]') ||
			document.querySelector('input[name="amember_pass"]')) return false;
		const text = document.body ? document.body.innerText : "";
		if (/welcome\\s+/i.test(text)) return true;
		if (/active subscription/i.test(text)) return true;
		if (/your tools/i.test(text)) return true;
		if (document.querySelector('[href*="logout"], [href*="log-out"]')) return true;
		return false;
	}`)
	return err == nil && res.Value.Bool()
}

func isLoginPage(page *rod.Page) bool {
	if has, _, _ := page.Has("input[name='amember_login']"); has {
		return true
	}
	if has, _, _ := page.Has("input[name='amember_pass']"); has {
		return true
	}
	if has, _, _ := page.Has("#login-submit-button, .frm-submit"); has {
		return true
	}
	return false
}

func hasMemberSessionCookie(page *rod.Page) bool {
	cookies, err := page.Cookies([]string{})
	if err != nil {
		return false
	}
	for _, c := range cookies {
		name := strings.ToLower(c.Name)
		if name == "amember_nr" || name == "amember_ru" || strings.HasPrefix(name, "amember_auth") {
			return true
		}
	}
	return false
}

func loginErrorMessage(page *rod.Page) string {
	res, err := page.Eval(`() => {
		const sel = ['.am-error', '.am-form-error', '.error', '.alert-danger', '.am-alert'];
		for (const s of sel) {
			const el = document.querySelector(s);
			if (el && el.textContent.trim()) return el.textContent.trim();
		}
		return "";
	}`)
	if err != nil || res.Value.Str() == "" {
		return ""
	}
	if isLoginPage(page) {
		return res.Value.Str()
	}
	return ""
}

func clearPortalSession(page *rod.Page) {
	_ = os.Remove(loginCookieFile())
	_ = proto.NetworkClearBrowserCookies{}.Call(page)
}

func pageURL(page *rod.Page) string {
	info, err := page.Info()
	if err != nil || info == nil {
		return ""
	}
	return info.URL
}

var errLoginFailed = &loginError{}

type loginError struct{}

func (e *loginError) Error() string {
	return "login failed: still on login page after submit (Turnstile or credentials)"
}

func savePortalCookies(ctx context.Context, page *rod.Page) error {
	if !hasMemberSessionCookie(page) {
		return fmt.Errorf("refusing to save portal cookies without member session")
	}
	cookies, err := page.Cookies([]string{})
	if err != nil {
		return err
	}
	params := proto.CookiesToParams(cookies)
	if b, err := json.Marshal(params); err == nil {
		_ = os.MkdirAll(filepath.Dir(loginCookieFile()), 0755)
		_ = os.WriteFile(loginCookieFile(), b, 0644)
	}

	var stored []cookiesession.Cookie
	for _, c := range cookies {
		if !strings.Contains(strings.ToLower(c.Domain), "seoshope.com") {
			continue
		}
		ss := "lax"
		stored = append(stored, cookiesession.Cookie{
			Domain:   c.Domain,
			Name:     c.Name,
			Value:    c.Value,
			Path:     c.Path,
			HTTPOnly: c.HTTPOnly,
			Secure:   c.Secure,
			SameSite: &ss,
		})
	}
	if len(stored) == 0 {
		return nil
	}
	return cookiesession.Save(ctx, cookiesession.SaveOptions{
		WebsiteID: portalSessionKey,
		Referer:   memberURL,
		Cookies:   stored,
	})
}

// runPortalHome logs in and saves portal cookies to seoshopehome without Semrush capture.
func runPortalHome(ctx context.Context, s *Session) (string, string) {
	var username, password string
	err := db.DB.QueryRowContext(ctx,
		`SELECT username, password_enc FROM credentials WHERE website_id='seoshope' AND is_enabled=1`).
		Scan(&username, &password)
	if err != nil {
		return "failed", "seoshope credentials not found"
	}
	if err := ensureLoggedIn(ctx, s, username, password); err != nil {
		return "failed", err.Error()
	}

	cookies, err := s.Page().Cookies([]string{})
	if err != nil {
		return "failed", "portal cookie read failed: " + err.Error()
	}
	var stored []cookiesession.Cookie
	for _, c := range cookies {
		if !strings.Contains(strings.ToLower(c.Domain), "seoshope.com") {
			continue
		}
		ss := "lax"
		stored = append(stored, cookiesession.Cookie{
			Domain:   c.Domain,
			Name:     c.Name,
			Value:    c.Value,
			Path:     c.Path,
			HTTPOnly: c.HTTPOnly,
			Secure:   c.Secure,
			SameSite: &ss,
		})
	}
	if !hasMemberSessionCookie(s.Page()) {
		return "failed", "portal login incomplete — missing amember_nr session cookie"
	}
	if len(stored) == 0 {
		return "failed", "no portal cookies captured"
	}
	if err := cookiesession.Save(ctx, cookiesession.SaveOptions{
		WebsiteID: "seoshopehome",
		Referer:   memberURL,
		Cookies:   stored,
	}); err != nil {
		return "failed", "db save failed: " + err.Error()
	}
	return "success", fmt.Sprintf("portal login saved (%d cookies)", len(stored))
}
