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
	if s.LoggedIn() {
		if isDashboard(s.Page()) {
			return nil
		}
		s.logged = false
	}

	page := s.Page()
	shots := screenshotDir()

	if cBytes, err := os.ReadFile(loginCookieFile()); err == nil {
		var params []*proto.NetworkCookieParam
		if json.Unmarshal(cBytes, &params) == nil && len(params) > 0 {
			_ = page.SetCookies(params)
			log.Printf("[SEOShope] Restored %d saved portal cookies", len(params))
		}
	}

	_ = page.Timeout(30 * time.Second).Navigate(memberURL)
	time.Sleep(2 * time.Second)

	if waitForPageReady(page, shots, 30*time.Second) && isDashboard(page) {
		log.Println("[SEOShope] Already logged in via profile/session")
		if hasMemberSessionCookie(page) {
			s.MarkLoggedIn()
			_ = savePortalCookies(ctx, page)
			return nil
		}
		log.Println("[SEOShope] Stale session (only PHPSESSID) — clearing and re-login")
		clearPortalSession(page)
	}

	if !isLoginPage(page) {
		takeScreenshot(page, "unexpected_page_before_login", shots)
		return fmt.Errorf("login page not found (url=%s)", pageURL(page))
	}

	log.Println("[SEOShope] Performing fresh login...")
	fillLoginForm(page, username, password)
	time.Sleep(2 * time.Second)
	takeScreenshot(page, "after_form_fill", shots)

	if !waitTurnstile(page, shots) {
		takeScreenshot(page, "turnstile_timeout", shots)
		return fmt.Errorf("login failed: Cloudflare Turnstile not solved (wait up to 30s on Mac)")
	}
	log.Println("[SEOShope] Turnstile token ready — submitting login")
	submitLoginForm(page)

	if err := waitForLoginSuccess(page, shots, 45*time.Second); err != nil {
		return err
	}
	if !hasMemberSessionCookie(page) {
		takeScreenshot(page, "login_no_member_cookie", shots)
		return fmt.Errorf("login failed: no member session cookie (amember_nr) — check credentials")
	}

	log.Println("[SEOShope] Login successful")
	s.MarkLoggedIn()
	if err := savePortalCookies(ctx, page); err != nil {
		log.Printf("[SEOShope] portal cookie save warning: %v", err)
	}
	return nil
}

func waitForPageReady(page *rod.Page, shots string, timeout time.Duration) bool {
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
		if isDashboard(page) || isLoginPage(page) {
			return true
		}
		time.Sleep(1 * time.Second)
	}
	takeScreenshot(page, "page_ready_timeout", shots)
	return false
}

func waitForLoginSuccess(page *rod.Page, shots string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if isDashboard(page) && hasMemberSessionCookie(page) {
			return nil
		}
		if msg := loginErrorMessage(page); msg != "" {
			takeScreenshot(page, "login_rejected", shots)
			return fmt.Errorf("login failed: %s", msg)
		}
		time.Sleep(500 * time.Millisecond)
	}
	takeScreenshot(page, "login_failed", shots)
	return errLoginFailed
}

func isDashboard(page *rod.Page) bool {
	selectors := []string{
		"a[href*='logout']",
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
	return hasMemberSessionCookie(page)
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
	cookies, err := page.Cookies([]string{"https://app.seoshope.com"})
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
