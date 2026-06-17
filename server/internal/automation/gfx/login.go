package gfx

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

// ensureGFXLogin opens GFX portal (or tool page) and logs in if needed.
// Returns the page left on the tool portal for reuse by runExtension.
func ensureGFXLogin(ctx context.Context, session *Session, startURL string) (*rod.Page, error) {
	page := session.newPage()
	slot := session.Slot()
	username := slot.Account.Username
	password := slot.Account.Password
	accountID := slot.Account.WebsiteID

	if startURL == "" {
		startURL = "https://app.gfxtoolz.ai/signin"
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

	log.Printf("[gfx_%s] Opening GFX portal: %s", slot.Account.WebsiteID, startURL)
	if err := page.Timeout(45 * time.Second).Navigate(startURL); err != nil {
		log.Printf("[gfx_%s] Navigation warning: %v", slot.Account.WebsiteID, err)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	for i := 0; i < 20; i++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		u := safeURL()
		if u != "" && !strings.Contains(u, "signin") {
			log.Printf("[gfx_%s] Already logged in (%s)", slot.Account.WebsiteID, u)
			return page, nil
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
		time.Sleep(200 * time.Millisecond)
	}

	u := safeURL()
	if u != "" && !strings.Contains(u, "signin") {
		log.Printf("[gfx_%s] Already logged in", slot.Account.WebsiteID)
		return page, nil
	}

	log.Printf("[gfx_%s] Logging in...", slot.Account.WebsiteID)
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
		return nil, fmt.Errorf("failed to fill credentials: %w", err)
	}

	_, _ = page.Eval(`() => {
		const btn = document.querySelector('button[type="submit"]');
		if (btn) { btn.click(); return true; }
		return false;
	}`)

	isRedirected := false
	showDeviceLimit := false
	for i := 0; i < 30; i++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
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
		time.Sleep(300 * time.Millisecond)
	}

	if showDeviceLimit {
		log.Printf("[gfx_%s] Device limit modal — clicking Sign In Again", slot.Account.WebsiteID)
		_, _ = page.Eval(`() => {
			const btn = Array.from(document.querySelectorAll('button')).find(b => b.textContent && b.textContent.includes('Sign In Again'));
			if (btn) { btn.click(); return true; }
			return false;
		}`)
		for k := 0; k < 15; k++ {
			if u := safeURL(); u != "" && !strings.Contains(u, "signin") {
				isRedirected = true
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
		if !isRedirected {
			saveErrorScreenshot(page, accountID, "device_limit")
			return nil, fmt.Errorf("%s", formatGFXLoginFailure(accountID, loginAPI, "device limit — Sign In Again did not work"))
		}
	}

	if !isRedirected {
		time.Sleep(1 * time.Second)
		pageErr := readPageLoginError(page)
		saveErrorScreenshot(page, accountID, "signin_failed")
		return nil, fmt.Errorf("%s", formatGFXLoginFailure(accountID, loginAPI, pageErr))
	}

	if startURL != "" && !strings.Contains(safeURL(), "/tools/") {
		_ = page.Timeout(30 * time.Second).Navigate(startURL)
		for i := 0; i < 15; i++ {
			if strings.Contains(safeURL(), "/tools/") || !strings.Contains(safeURL(), "signin") {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
	}

	log.Printf("[gfx_%s] Login successful", slot.Account.WebsiteID)
	return page, nil
}
