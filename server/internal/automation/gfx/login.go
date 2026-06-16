package gfx

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

func ensureGFXLogin(ctx context.Context, session *Session) error {
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

	log.Printf("[gfx_%s] Navigating to GFXToolz login...", slot.Account.WebsiteID)
	if err := page.Timeout(60 * time.Second).Navigate("https://app.gfxtoolz.ai/signin"); err != nil {
		log.Printf("[gfx_%s] Navigation warning: %v", slot.Account.WebsiteID, err)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	time.Sleep(3 * time.Second)

	alreadyLoggedIn := false
	if u := safeURL(); u != "" && !strings.Contains(u, "signin") {
		log.Printf("[gfx_%s] Already logged in", slot.Account.WebsiteID)
		return nil
	}

	log.Printf("[gfx_%s] Waiting for login form...", slot.Account.WebsiteID)
	loaded := false
	pageReloaded := false
	for i := 0; i < 60; i++ {
		if ctx.Err() != nil {
			return fmt.Errorf("context expired waiting for login form: %w", ctx.Err())
		}
		hasEmail, _, _ := page.Has("input[type='email']")
		if hasEmail {
			loaded = true
			break
		}
		urlNow := safeURL()
		if urlNow != "" && !strings.Contains(urlNow, "signin") {
			alreadyLoggedIn = true
			loaded = true
			break
		}
		title := safeTitle()
		if strings.Contains(title, "Just a moment") || strings.Contains(title, "Checking your browser") {
			solveCloudflare(page)
			time.Sleep(5 * time.Second)
			continue
		}
		if i == 10 && !pageReloaded {
			_ = page.Timeout(30 * time.Second).Reload()
			pageReloaded = true
			time.Sleep(3 * time.Second)
			continue
		}
		time.Sleep(1 * time.Second)
	}
	if !loaded {
		saveErrorScreenshot(page, accountID, "login_form_missing")
		return fmt.Errorf("timed out waiting for GFX login form")
	}
	if alreadyLoggedIn {
		return nil
	}

	log.Printf("[gfx_%s] Filling credentials...", slot.Account.WebsiteID)
	stopWatch, loginAPI := watchGFXLoginAPI(page)
	defer stopWatch()

	_, err := page.Eval(`(u, p) => {
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
		return fmt.Errorf("failed to fill credentials: %w", err)
	}

	_, _ = page.Eval(`() => {
		const btn = document.querySelector('button[type="submit"]');
		if (btn) { btn.click(); return true; }
		return false;
	}`)

	isRedirected := false
	showDeviceLimit := false
	for i := 0; i < 40; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		urlNow := safeURL()
		if urlNow != "" && !strings.Contains(urlNow, "signin") {
			isRedirected = true
			break
		}
		hasLimitText, errLimit := page.Eval(`() => {
			const btns = Array.from(document.querySelectorAll('button'));
			return btns.some(b => b.textContent && b.textContent.includes('Sign In Again'));
		}`)
		if errLimit == nil && hasLimitText.Value.Bool() {
			showDeviceLimit = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if showDeviceLimit {
		log.Printf("[gfx_%s] Device limit modal — clicking Sign In Again", slot.Account.WebsiteID)
		_, _ = page.Eval(`() => {
			const btn = Array.from(document.querySelectorAll('button')).find(b => b.textContent && b.textContent.includes('Sign In Again'));
			if (btn) { btn.click(); return true; }
			return false;
		}`)
		for k := 0; k < 20; k++ {
			if u := safeURL(); u != "" && !strings.Contains(u, "signin") {
				isRedirected = true
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if !isRedirected {
			saveErrorScreenshot(page, accountID, "device_limit")
			return fmt.Errorf("%s", formatGFXLoginFailure(accountID, loginAPI, "device limit — Sign In Again did not work"))
		}
	}

	if !isRedirected {
		time.Sleep(2 * time.Second) // allow auth/login response body
		pageErr := readPageLoginError(page)
		saveErrorScreenshot(page, accountID, "signin_failed")
		return fmt.Errorf("%s", formatGFXLoginFailure(accountID, loginAPI, pageErr))
	}
	log.Printf("[gfx_%s] Login successful", slot.Account.WebsiteID)
	return nil
}
