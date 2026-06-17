package gfx

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

const (
	gfxSigninURL    = "https://app.gfxtoolz.ai/signin"
	gfxPortalURL    = "https://app.gfxtoolz.ai/"
	reactSettleWait = 2 * time.Second
)

func gfxLoginFormVisible(page *rod.Page) bool {
	res, err := page.Eval(`() => {
		return !!document.querySelector('input[type="email"]') &&
			!!document.querySelector('input[type="password"]');
	}`)
	return err == nil && res.Value.Bool()
}

func gfxGuestState(page *rod.Page) bool {
	res, err := page.Eval(`() => {
		const body = (document.body && document.body.innerText) ? document.body.innerText : '';
		if (body.includes('SIGN-IN REQUIRED') || body.includes('Sign in to launch')) return true;
		if (body.includes('Sign in to GFXToolz') && document.querySelector('input[type="password"]')) return true;
		const labels = Array.from(document.querySelectorAll('button,a')).map(el => (el.textContent||'').trim());
		if (labels.includes('Login') && labels.includes('Sign up')) return true;
		return false;
	}`)
	return err == nil && res.Value.Bool()
}

func gfxLoggedInPortal(page *rod.Page) bool {
	res, err := page.Eval(`() => {
		const url = location.href || '';
		if (url.includes('signin')) return false;
		const body = (document.body && document.body.innerText) ? document.body.innerText : '';
		if (body.includes('SIGN-IN REQUIRED') || body.includes('Sign in to launch')) return false;
		if (body.includes('Sign in to GFXToolz') && document.querySelector('input[type="password"]')) return false;
		if (body.includes('Welcome back') || body.includes('What would you like to do today')) return true;
		const labels = Array.from(document.querySelectorAll('button,a')).map(el => (el.textContent||'').trim());
		if (labels.includes('Login') && labels.includes('Sign up')) return false;
		if (document.querySelector('[data-tool-cookie="true"]')) return true;
		return false;
	}`)
	return err == nil && res.Value.Bool()
}

func gfxClickLoginEntry(page *rod.Page) bool {
	res, err := page.Eval(`() => {
		const pick = [...document.querySelectorAll('button,a,[role="button"]')].find(el => {
			const t = (el.textContent||'').replace(/\s+/g,' ').trim().toLowerCase();
			return t === 'login' || t === 'sign in' || t.startsWith('sign in');
		});
		if (pick) { pick.click(); return true; }
		return false;
	}`)
	return err == nil && res.Value.Bool()
}

func waitReactSettle(ctx context.Context) error {
	deadline := time.Now().Add(reactSettleWait)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil
}

func solveCloudflareIfNeeded(page *rod.Page, accountID string, titleFn func() string) {
	title := titleFn()
	if strings.Contains(title, "Just a moment") || strings.Contains(title, "Checking your browser") {
		log.Printf("[gfx_%s] Cloudflare challenge — solving...", accountID)
		solveCloudflare(page)
		time.Sleep(2 * time.Second)
	}
}

// ensureGFXLogin always opens /signin first, logs in if needed, returns freshLogin when credentials were used.
// Tool page navigation happens after optional Chrome relaunch in runner.go.
func ensureGFXLogin(ctx context.Context, session *Session, _ string) (*rod.Page, bool, error) {
	page := session.newPage()
	slot := session.Slot()
	username := slot.Account.Username
	password := slot.Account.Password
	accountID := slot.Account.WebsiteID

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

	log.Printf("[gfx_%s] Step 1: opening sign-in page %s", accountID, gfxSigninURL)
	if err := page.Timeout(45 * time.Second).Navigate(gfxSigninURL); err != nil {
		log.Printf("[gfx_%s] Sign-in navigation warning: %v", accountID, err)
	}
	if ctx.Err() != nil {
		return nil, false, ctx.Err()
	}

	log.Printf("[gfx_%s] Waiting %s for React to settle...", accountID, reactSettleWait)
	if err := waitReactSettle(ctx); err != nil {
		return nil, false, err
	}
	solveCloudflareIfNeeded(page, accountID, safeTitle)

	if gfxLoggedInPortal(page) {
		log.Printf("[gfx_%s] Cookie session — already logged in at %s", accountID, safeURL())
		return page, false, nil
	}

	// Still on signin or dashboard with login prompt — wait for form or login button.
	for i := 0; i < 15; i++ {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		if gfxLoggedInPortal(page) {
			log.Printf("[gfx_%s] Logged in (redirect) at %s", accountID, safeURL())
			return page, false, nil
		}
		if gfxLoginFormVisible(page) {
			break
		}
		if gfxClickLoginEntry(page) {
			log.Printf("[gfx_%s] Clicked Login entry — waiting for form...", accountID)
			time.Sleep(reactSettleWait)
			if gfxLoginFormVisible(page) {
				break
			}
		}
		solveCloudflareIfNeeded(page, accountID, safeTitle)
		time.Sleep(400 * time.Millisecond)
	}

	if gfxLoggedInPortal(page) {
		return page, false, nil
	}

	if !gfxLoginFormVisible(page) {
		shot := saveErrorScreenshot(page, accountID, "login_form_missing")
		msg := fmt.Sprintf("login form not found on sign-in page for %s (url: %s)", accountID, safeURL())
		if shot != "" {
			msg += " | screenshot: " + shot
		}
		return nil, false, fmt.Errorf("%s", msg)
	}

	log.Printf("[gfx_%s] Step 2: filling credentials...", accountID)
	stopWatch, loginAPI := watchGFXLoginAPI(page)

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
		stopWatch()
		return nil, false, fmt.Errorf("failed to fill credentials: %w", err)
	}
	if filled != nil && !filled.Value.Bool() {
		stopWatch()
		return nil, false, fmt.Errorf("failed to fill credentials for %s", accountID)
	}

	_, _ = page.Eval(`() => {
		const btn = document.querySelector('button[type="submit"]') ||
			[...document.querySelectorAll('button')].find(b => (b.textContent||'').trim() === 'Sign in');
		if (btn) { btn.click(); return true; }
		return false;
	}`)

	loggedIn := false
	showDeviceLimit := false
	for i := 0; i < 50; i++ {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		if gfxLoggedInPortal(page) {
			loggedIn = true
			break
		}
		urlNow := safeURL()
		if urlNow != "" && !strings.Contains(urlNow, "signin") && strings.Contains(urlNow, "gfxtoolz.ai") {
			time.Sleep(reactSettleWait)
			if gfxLoggedInPortal(page) {
				loggedIn = true
				break
			}
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
		for k := 0; k < 25; k++ {
			if gfxLoggedInPortal(page) {
				loggedIn = true
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
	}

	if !loggedIn {
		stopWatch()
		pageErr := readPageLoginError(page)
		shot := saveErrorScreenshot(page, accountID, "signin_failed")
		errMsg := formatGFXLoginFailure(accountID, loginAPI, pageErr)
		if shot != "" {
			errMsg += " | screenshot: " + shot
		}
		return nil, false, fmt.Errorf("%s", errMsg)
	}

	log.Printf("[gfx_%s] Step 3: credential login OK → %s (Chrome relaunch next)", accountID, safeURL())
	stopWatch()
	return page, true, nil
}

func prepareToolPage(ctx context.Context, page *rod.Page, toolURL, accountID string) error {
	log.Printf("[gfx_%s] Navigating to tool page: %s", accountID, toolURL)
	if err := page.Timeout(45 * time.Second).Navigate(toolURL); err != nil {
		log.Printf("[gfx_%s] Tool navigation warning: %v", accountID, err)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	time.Sleep(reactSettleWait)

	if !waitExtensionInject(page, 24) {
		log.Printf("[gfx_%s] Extension not on tool page — reloading", accountID)
		_ = page.Timeout(30 * time.Second).Reload()
		time.Sleep(reactSettleWait)
		if !waitExtensionInject(page, 16) {
			return fmt.Errorf("GFX extension not active on tool page for %s", accountID)
		}
	}

	dismissNonAuthDialogs(page)
	scrollPageForButtons(page)

	for i := 0; i < 40; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if gfxAccessButtonReady(page) {
			log.Printf("[gfx_%s] Access button ready on tool page", accountID)
			return nil
		}
		if i == 6 || i == 18 {
			scrollPageForButtons(page)
		}
		if i == 12 {
			_ = page.Timeout(30 * time.Second).Reload()
			time.Sleep(reactSettleWait)
			waitExtensionInject(page, 12)
			dismissNonAuthDialogs(page)
			scrollPageForButtons(page)
		}
		time.Sleep(400 * time.Millisecond)
	}
	return fmt.Errorf("access button not ready on tool page for %s", accountID)
}

func loginBodyHasToken(body string) bool {
	body = strings.TrimSpace(body)
	if body == "" {
		return false
	}
	return strings.Contains(body, "accessToken") || strings.Contains(body, "eyJ")
}
