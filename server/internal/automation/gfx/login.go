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

// gfxLoggedIn checks UI state — tool URLs can load for guests with "SIGN-IN REQUIRED".
func gfxLoggedIn(page *rod.Page) bool {
	res, err := page.Eval(`() => {
		const body = (document.body && document.body.innerText) ? document.body.innerText : '';
		const url = location.href || '';
		if (url.includes('signin')) return false;
		if (body.includes('SIGN-IN REQUIRED') || body.includes('Sign in to launch this tool')) return false;
		if (body.includes('Sign in to GFXToolz') && document.querySelector('input[type="password"]')) return false;
		const labels = Array.from(document.querySelectorAll('button,a')).map(el => (el.textContent||'').trim());
		if (labels.includes('Login') && labels.includes('Sign up') && !document.querySelector('button[data-tool-cookie="true"]')) {
			return false;
		}
		if (document.querySelector('button[data-tool-cookie="true"]')) return true;
		if (document.querySelector('[data-tool-cookie="true"]')) return true;
		if (document.querySelector('a[href*="/tools/"]') && !body.includes('Sign in to continue')) return true;
		return false;
	}`)
	return err == nil && res.Value.Bool()
}

func gfxGuestVisible(page *rod.Page) bool {
	res, err := page.Eval(`() => {
		const body = (document.body && document.body.innerText) ? document.body.innerText : '';
		const url = location.href || '';
		if (url.includes('signin')) return true;
		if (body.includes('SIGN-IN REQUIRED') || body.includes('Sign in to launch this tool')) return true;
		if (body.includes('Sign in to GFXToolz') && document.querySelector('input[type="password"]')) return true;
		const labels = Array.from(document.querySelectorAll('button,a')).map(el => (el.textContent||'').trim());
		return labels.includes('Login') && labels.includes('Sign up');
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

	needsLogin := false
	for i := 0; i < 24; i++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if gfxLoggedIn(page) {
			log.Printf("[gfx_%s] Session active (%s)", accountID, safeURL())
			return page, nil
		}
		if gfxGuestVisible(page) {
			needsLogin = true
			log.Printf("[gfx_%s] Guest session detected — will sign in", accountID)
			break
		}
		title := safeTitle()
		if strings.Contains(title, "Just a moment") || strings.Contains(title, "Checking your browser") {
			solveCloudflare(page)
			time.Sleep(1500 * time.Millisecond)
			continue
		}
		hasEmail, _, _ := page.Has("input[type='email']")
		if hasEmail {
			needsLogin = true
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	if !needsLogin && gfxLoggedIn(page) {
		return page, nil
	}
	if !needsLogin && !gfxGuestVisible(page) {
		// Page still loading — one more short wait
		for i := 0; i < 8; i++ {
			if gfxLoggedIn(page) {
				return page, nil
			}
			if gfxGuestVisible(page) {
				needsLogin = true
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
	}

	if gfxLoggedIn(page) {
		return page, nil
	}

	if !strings.Contains(safeURL(), "signin") {
		log.Printf("[gfx_%s] Opening sign-in page", accountID)
		_ = page.Timeout(30 * time.Second).Navigate(gfxSigninURL)
		for i := 0; i < 20; i++ {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if gfxLoggedIn(page) {
				break
			}
			hasEmail, _, _ := page.Has("input[type='email']")
			if hasEmail {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
	}

	if gfxLoggedIn(page) {
		log.Printf("[gfx_%s] Logged in after redirect", accountID)
		if startURL != gfxSigninURL && startURL != "" {
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
		shot := saveErrorScreenshot(page, accountID, "login_form_missing")
		msg := fmt.Sprintf("login form not found for %s", accountID)
		if shot != "" {
			msg += " | screenshot: " + shot
		}
		return nil, fmt.Errorf("%s", msg)
	}

	_, _ = page.Eval(`() => {
		const btn = document.querySelector('button[type="submit"]') ||
			Array.from(document.querySelectorAll('button')).find(b => (b.textContent||'').trim() === 'Sign in');
		if (btn) { btn.click(); return true; }
		return false;
	}`)

	loggedIn := false
	showDeviceLimit := false
	for i := 0; i < 30; i++ {
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
		time.Sleep(250 * time.Millisecond)
	}

	if showDeviceLimit {
		log.Printf("[gfx_%s] Device limit modal — clicking Sign In Again", accountID)
		_, _ = page.Eval(`() => {
			const btn = Array.from(document.querySelectorAll('button')).find(b => b.textContent && b.textContent.includes('Sign In Again'));
			if (btn) { btn.click(); return true; }
			return false;
		}`)
		for k := 0; k < 15; k++ {
			if gfxLoggedIn(page) {
				loggedIn = true
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		if !loggedIn {
			shot := saveErrorScreenshot(page, accountID, "device_limit")
			errMsg := formatGFXLoginFailure(accountID, loginAPI, "device limit — Sign In Again did not work")
			if shot != "" {
				errMsg += " | screenshot: " + shot
			}
			return nil, fmt.Errorf("%s", errMsg)
		}
	}

	if !loggedIn {
		time.Sleep(500 * time.Millisecond)
		pageErr := readPageLoginError(page)
		shot := saveErrorScreenshot(page, accountID, "signin_failed")
		errMsg := formatGFXLoginFailure(accountID, loginAPI, pageErr)
		if shot != "" {
			errMsg += " | screenshot: " + shot
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	if startURL != "" && startURL != gfxSigninURL {
		_ = page.Timeout(30 * time.Second).Navigate(startURL)
		for i := 0; i < 20; i++ {
			if gfxLoggedIn(page) {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		if !gfxLoggedIn(page) {
			shot := saveErrorScreenshot(page, accountID, "post_login_guest")
			msg := "login succeeded but tool page still shows sign-in required"
			if shot != "" {
				msg += " | screenshot: " + shot
			}
			return nil, fmt.Errorf("%s", msg)
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
