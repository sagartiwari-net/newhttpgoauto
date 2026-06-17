package gfx

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

const gfxSigninURL = "https://app.gfxtoolz.ai/signin"

// gfxLoggedIn checks the page UI — not just the URL (guest homepage has no /signin but shows login modal).
func gfxLoggedIn(page *rod.Page) bool {
	res, err := page.Eval(`() => {
		const url = location.href || '';
		const body = (document.body && document.body.innerText) ? document.body.innerText : '';
		if (url.includes('signin')) return false;
		if (body.includes('Sign in to GFXToolz') && document.querySelector('input[type="password"]')) {
			return false;
		}
		const texts = Array.from(document.querySelectorAll('button,a')).map(el => (el.textContent||'').trim());
		if (texts.some(t => t === 'Login') && texts.some(t => t === 'Sign up')) {
			if (!document.querySelector('a[href*="/tools/"]') && !document.querySelector('[data-tool-cookie]')) {
				return false;
			}
		}
		if (document.querySelector('a[href*="/tools/"]')) return true;
		if (document.querySelector('[data-tool-cookie]')) return true;
		if (document.querySelector('[data-tool-id]')) return true;
		if (body.includes('Your tools') && body.includes('Premium') && !body.includes('Sign in to GFXToolz')) {
			return true;
		}
		return false;
	}`)
	return err == nil && res.Value.Bool()
}

// ensureGFXLogin opens GFX portal (or tool page) and logs in if needed.
func ensureGFXLogin(ctx context.Context, session *Session, startURL string) (*rod.Page, error) {
	page := session.newPage()
	slot := session.Slot()
	username := slot.Account.Username
	password := slot.Account.Password
	accountID := slot.Account.WebsiteID

	if startURL == "" {
		startURL = gfxSigninURL
	}

	safeURL := func() string {
		if ctx.Err() != nil {
			return ""
		}
		info, err := page.Info()
		if err != nil {
			return ""
		}
		return info.URL
	}
	safeTitle := func() string {
		if ctx.Err() != nil {
			return ""
		}
		info, err := page.Info()
		if err != nil {
			return ""
		}
		return info.Title
	}

	log.Printf("[gfx_%s] Opening GFX portal: %s", accountID, startURL)
	if err := page.Timeout(45 * time.Second).Navigate(startURL); err != nil {
		log.Printf("[gfx_%s] Navigation warning: %v", accountID, err)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	for i := 0; i < 25; i++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if gfxLoggedIn(page) {
			log.Printf("[gfx_%s] Already logged in (%s)", accountID, safeURL())
			if startURL != "" && !strings.Contains(safeURL(), "/tools/") && strings.Contains(startURL, "/tools/") {
				_ = page.Timeout(30 * time.Second).Navigate(startURL)
			}
			return page, nil
		}
		title := safeTitle()
		if strings.Contains(title, "Just a moment") || strings.Contains(title, "Checking your browser") {
			solveCloudflare(page)
			time.Sleep(2 * time.Second)
			continue
		}
		hasEmail, _, _ := page.Has("input[type='email']")
		if hasEmail {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}

	if !gfxLoggedIn(page) {
		log.Printf("[gfx_%s] Not logged in — opening sign-in page", accountID)
		_ = page.Timeout(30 * time.Second).Navigate(gfxSigninURL)
		for i := 0; i < 30; i++ {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			hasEmail, _, _ := page.Has("input[type='email']")
			if hasEmail {
				break
			}
			if gfxLoggedIn(page) {
				log.Printf("[gfx_%s] Session became active during sign-in navigation", accountID)
				if startURL != gfxSigninURL {
					_ = page.Timeout(30 * time.Second).Navigate(startURL)
				}
				return page, nil
			}
			title := safeTitle()
			if strings.Contains(title, "Just a moment") || strings.Contains(title, "Checking your browser") {
				solveCloudflare(page)
				time.Sleep(2 * time.Second)
				continue
			}
			time.Sleep(300 * time.Millisecond)
		}
	}

	if gfxLoggedIn(page) {
		log.Printf("[gfx_%s] Logged in after sign-in navigation", accountID)
		if startURL != "" && startURL != gfxSigninURL && !strings.Contains(safeURL(), "/tools/") && strings.Contains(startURL, "/tools/") {
			_ = page.Timeout(30 * time.Second).Navigate(startURL)
		}
		return page, nil
	}

	log.Printf("[gfx_%s] Logging in...", accountID)
	stopWatch, loginAPI := watchGFXLoginAPI(page)
	defer stopWatch()

	filled, err := page.Eval(`(u, p) => {
		const loginEl = document.querySelector('input[type="email"]');
		const passEl  = document.querySelector('input[type="password"]');
		if (!loginEl || !passEl) return false;
		const nativeInputValueSetter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value").set;
		nativeInputValueSetter.call(loginEl, u);
		loginEl.dispatchEvent(new Event('input', { bubbles: true }));
		nativeInputValueSetter.call(passEl, p);
		passEl.dispatchEvent(new Event('input', { bubbles: true }));
		return true;
	}`, username, password)
	if err != nil {
		return nil, fmt.Errorf("failed to fill credentials: %w", err)
	}
	if filled != nil && !filled.Value.Bool() {
		saveErrorScreenshot(page, accountID, "login_form_missing")
		return nil, fmt.Errorf("login form not found for %s", accountID)
	}

	_, _ = page.Eval(`() => {
		const btn = document.querySelector('button[type="submit"]') ||
			Array.from(document.querySelectorAll('button')).find(b => (b.textContent||'').trim() === 'Sign in');
		if (btn) { btn.click(); return true; }
		return false;
	}`)

	loggedIn := false
	showDeviceLimit := false
	for i := 0; i < 40; i++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if gfxLoggedIn(page) {
			loggedIn = true
			break
		}
		status, body, seen := loginAPI.snapshot()
		if seen && status >= 200 && status < 300 && loginBodyHasToken(body) {
			loggedIn = true
			break
		}
		hasLimit, _ := page.Eval(`() => {
			return Array.from(document.querySelectorAll('button')).some(b => b.textContent && b.textContent.includes('Sign In Again'));
		}`)
		if hasLimit != nil && hasLimit.Value.Bool() {
			showDeviceLimit = true
			break
		}
		time.Sleep(300 * time.Millisecond)
	}

	if showDeviceLimit {
		log.Printf("[gfx_%s] Device limit modal — clicking Sign In Again", accountID)
		_, _ = page.Eval(`() => {
			const btn = Array.from(document.querySelectorAll('button')).find(b => b.textContent && b.textContent.includes('Sign In Again'));
			if (btn) { btn.click(); return true; }
			return false;
		}`)
		for k := 0; k < 20; k++ {
			if gfxLoggedIn(page) {
				loggedIn = true
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
		if !loggedIn {
			saveErrorScreenshot(page, accountID, "device_limit")
			return nil, fmt.Errorf("%s", formatGFXLoginFailure(accountID, loginAPI, "device limit — Sign In Again did not work"))
		}
	}

	if !loggedIn {
		time.Sleep(1 * time.Second)
		pageErr := readPageLoginError(page)
		saveErrorScreenshot(page, accountID, "signin_failed")
		return nil, fmt.Errorf("%s", formatGFXLoginFailure(accountID, loginAPI, pageErr))
	}

	if startURL != "" && startURL != gfxSigninURL {
		_ = page.Timeout(30 * time.Second).Navigate(startURL)
		for i := 0; i < 20; i++ {
			if gfxLoggedIn(page) && (strings.Contains(safeURL(), "/tools/") || strings.Contains(startURL, safeURL())) {
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
	}

	log.Printf("[gfx_%s] Login successful → %s", accountID, safeURL())
	return page, nil
}

func loginBodyHasToken(body string) bool {
	body = strings.TrimSpace(body)
	if body == "" {
		return false
	}
	return strings.Contains(body, "accessToken") || strings.Contains(body, "eyJ")
}
