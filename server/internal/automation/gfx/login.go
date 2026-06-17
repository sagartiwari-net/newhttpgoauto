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

func gfxSessionActive(page *rod.Page) bool {
	res, err := page.Eval(`() => {
		const body = (document.body && document.body.innerText) ? document.body.innerText : '';
		const url = location.href || '';
		if (url.includes('signin')) return false;
		if (body.includes('SIGN-IN REQUIRED') || body.includes('Sign in to launch')) return false;
		if (body.includes('Sign in to GFXToolz') && document.querySelector('input[type="password"]')) return false;
		const labels = Array.from(document.querySelectorAll('button,a')).map(el => (el.textContent||'').trim());
		if (labels.includes('Login') && labels.includes('Sign up') && !document.querySelector('[data-tool-cookie="true"]')) return false;
		if (url.includes('/tools/')) {
			if (document.querySelector('[data-tool-cookie="true"]')) return true;
			const t = (s) => (s||'').replace(/\s+/g,' ').trim().toLowerCase();
			return [...document.querySelectorAll('button,a,[role="button"]')].some(el => {
				const txt = t(el.textContent);
				return txt.includes('access now') || txt.includes('get access') || txt.includes('launch') || txt.includes('open tool') || txt === 'access';
			});
		}
		if (document.querySelector('[data-tool-cookie="true"]')) return true;
		return !labels.includes('Login') || !labels.includes('Sign up');
	}`)
	return err == nil && res.Value.Bool()
}

func gfxGuestState(page *rod.Page) bool {
	res, err := page.Eval(`() => {
		const body = (document.body && document.body.innerText) ? document.body.innerText : '';
		const url = location.href || '';
		if (url.includes('signin')) return true;
		if (body.includes('SIGN-IN REQUIRED') || body.includes('Sign in to launch')) return true;
		if (body.includes('Sign in to GFXToolz') && document.querySelector('input[type="password"]')) return true;
		const labels = Array.from(document.querySelectorAll('button,a')).map(el => (el.textContent||'').trim());
		if (labels.includes('Login') && labels.includes('Sign up') && !document.querySelector('button[data-tool-cookie="true"]')) return true;
		return false;
	}`)
	return err == nil && res.Value.Bool()
}

// ensureGFXLogin opens the tool page, logs in if needed, leaves page ready for access button.
// The bool return is true when credentials were used (fresh login) vs an existing cookie session.
func ensureGFXLogin(ctx context.Context, session *Session, startURL string) (*rod.Page, bool, error) {
	page := session.newPage()
	slot := session.Slot()
	username := slot.Account.Username
	password := slot.Account.Password
	accountID := slot.Account.WebsiteID
	freshLogin := false

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
		return nil, false, ctx.Err()
	}

	// Wait for React — do NOT trust URL-only "already logged in" on empty/loading page.
	for i := 0; i < 30; i++ {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		if gfxSessionActive(page) {
			log.Printf("[gfx_%s] Session active (%s)", accountID, safeURL())
			return page, false, nil
		}
		title := safeTitle()
		if strings.Contains(title, "Just a moment") || strings.Contains(title, "Checking your browser") {
			solveCloudflare(page)
			time.Sleep(2 * time.Second)
			continue
		}
		if gfxGuestState(page) {
			break
		}
		hasEmail, _, _ := page.Has("input[type='email']")
		if hasEmail {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	if gfxSessionActive(page) {
		return page, false, nil
	}

	if !strings.Contains(safeURL(), "signin") {
		log.Printf("[gfx_%s] Not logged in — opening sign-in page", accountID)
		_ = page.Timeout(30 * time.Second).Navigate(gfxSigninURL)
	}

	for i := 0; i < 40; i++ {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		if gfxSessionActive(page) {
			break
		}
		hasEmail, _, _ := page.Has("input[type='email']")
		if hasEmail {
			break
		}
		title := safeTitle()
		if strings.Contains(title, "Just a moment") || strings.Contains(title, "Checking your browser") {
			solveCloudflare(page)
			time.Sleep(2 * time.Second)
			continue
		}
		time.Sleep(250 * time.Millisecond)
	}

	if gfxSessionActive(page) {
		log.Printf("[gfx_%s] Logged in after redirect", accountID)
		if startURL != gfxSigninURL && startURL != "" {
			_ = page.Timeout(30 * time.Second).Navigate(startURL)
			waitSessionOnToolPage(ctx, page, accountID)
		}
		return page, false, nil
	}

	log.Printf("[gfx_%s] Logging in with credentials...", accountID)
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
		return nil, false, fmt.Errorf("failed to fill credentials: %w", err)
	}
	if filled != nil && !filled.Value.Bool() {
		shot := saveErrorScreenshot(page, accountID, "login_form_missing")
		msg := fmt.Sprintf("login form not found for %s", accountID)
		if shot != "" {
			msg += " | screenshot: " + shot
		}
		return nil, false, fmt.Errorf("%s", msg)
	}

	_, _ = page.Eval(`() => {
		const btn = document.querySelector('button[type="submit"]') ||
			[...document.querySelectorAll('button')].find(b => (b.textContent||'').trim() === 'Sign in');
		if (btn) { btn.click(); return true; }
		return false;
	}`)

	loggedIn := false
	showDeviceLimit := false
	for i := 0; i < 40; i++ {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		urlNow := safeURL()
		if urlNow != "" && !strings.Contains(urlNow, "signin") {
			loggedIn = true
			break
		}
		if gfxSessionActive(page) {
			loggedIn = true
			break
		}
		status, body, seen := loginAPI.snapshot()
		if seen && status >= 200 && status < 300 && loginBodyHasToken(body) {
			loggedIn = true
			break
		}
		hasLimit, _ := page.Eval(`() => {
			return [...document.querySelectorAll('button')].some(b => b.textContent && b.textContent.includes('Sign In Again'));
		}`)
		if hasLimit != nil && hasLimit.Value.Bool() {
			showDeviceLimit = true
			break
		}
		time.Sleep(300 * time.Millisecond)
	}

	if showDeviceLimit {
		log.Printf("[gfx_%s] Device limit — clicking Sign In Again", accountID)
		_, _ = page.Eval(`() => {
			const btn = [...document.querySelectorAll('button')].find(b => b.textContent && b.textContent.includes('Sign In Again'));
			if (btn) { btn.click(); return true; }
			return false;
		}`)
		for k := 0; k < 20; k++ {
			if gfxSessionActive(page) {
				loggedIn = true
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
	}

	if !loggedIn {
		pageErr := readPageLoginError(page)
		shot := saveErrorScreenshot(page, accountID, "signin_failed")
		errMsg := formatGFXLoginFailure(accountID, loginAPI, pageErr)
		if shot != "" {
			errMsg += " | screenshot: " + shot
		}
		return nil, false, fmt.Errorf("%s", errMsg)
	}

	freshLogin = true
	log.Printf("[gfx_%s] Credential login successful → %s (Chrome relaunch next)", accountID, safeURL())
	return page, freshLogin, nil
}

func waitSessionOnToolPage(ctx context.Context, page *rod.Page, accountID string) error {
	for i := 0; i < 40; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if gfxAccessButtonReady(page) || gfxSessionActive(page) {
			return nil
		}
		if i == 5 {
			scrollPageForButtons(page)
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("tool page loaded but access button not ready for %s (login may have failed)", accountID)
}

func loginBodyHasToken(body string) bool {
	body = strings.TrimSpace(body)
	if body == "" {
		return false
	}
	return strings.Contains(body, "accessToken") || strings.Contains(body, "eyJ")
}
