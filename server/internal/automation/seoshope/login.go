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
	portalSessionKey = "seoshope_login"
)

func ensureLoggedIn(ctx context.Context, s *Session, username, password string) error {
	if s.LoggedIn() {
		return nil
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
	time.Sleep(1 * time.Second)

	for i := 0; i < 30; i++ {
		info, _ := page.Info()
		title, currentURL := "", ""
		if info != nil {
			title = info.Title
			currentURL = info.URL
		}

		if strings.Contains(title, "Just a moment") || strings.Contains(title, "Checking your browser") {
			solveCloudflare(page)
			time.Sleep(3 * time.Second)
			continue
		}

		if strings.Contains(strings.ToLower(title), "error has occurred") {
			_ = os.Remove(loginCookieFile())
			_ = proto.NetworkClearBrowserCookies{}.Call(page)
			_ = page.Navigate(memberURL)
			time.Sleep(2 * time.Second)
			continue
		}

		hasLogin, _, _ := page.Has("input[name='amember_login']")
		hasPass, _, _ := page.Has("input[type='password']")
		if !hasLogin && !hasPass && strings.Contains(currentURL, "member") && !strings.Contains(currentURL, "login") {
			log.Println("[SEOShope] Already logged in via profile/session")
			s.MarkLoggedIn()
			_ = savePortalCookies(ctx, page)
			return nil
		}
		if hasPass {
			break
		}
		time.Sleep(1 * time.Second)
	}

	log.Println("[SEOShope] Performing fresh login...")
	fillLoginForm(page, username, password)
	time.Sleep(2 * time.Second)
	waitTurnstile(page, shots)
	submitLoginForm(page)

	for i := 0; i < 24; i++ {
		hasPass, _, _ := page.Has("input[type='password']")
		if !hasPass {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	hasPass, _, _ := page.Has("input[type='password']")
	if hasPass {
		takeScreenshot(page, "login_failed", shots)
		return errLoginFailed
	}

	log.Println("[SEOShope] Login successful")
	s.MarkLoggedIn()
	if err := savePortalCookies(ctx, page); err != nil {
		log.Printf("[SEOShope] portal cookie save warning: %v", err)
	}
	return nil
}

var errLoginFailed = &loginError{}

type loginError struct{}

func (e *loginError) Error() string { return "login failed: still on login page (Turnstile or credentials)" }

func savePortalCookies(ctx context.Context, page *rod.Page) error {
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
