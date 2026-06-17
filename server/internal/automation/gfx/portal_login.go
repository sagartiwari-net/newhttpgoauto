package gfx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

const gfxSigninURL = "https://app.gfxtoolz.ai/signin"

// ensurePortalLogin always performs a fresh sign-in for homepage capture (no stale-session shortcut).
// Returns the page and the login API response body (when captured).
func ensurePortalLogin(ctx context.Context, session *Session, homeURL string) (*rod.Page, string, error) {
	page := session.newPage()
	slot := session.Slot()
	accountID := slot.Account.WebsiteID
	username := slot.Account.Username
	password := slot.Account.Password

	if homeURL == "" {
		homeURL = gfxPortalHomeURL
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

	log.Printf("[gfx_portal] Fresh login flow → %s", gfxSigninURL)
	if err := page.Timeout(45 * time.Second).Navigate(gfxSigninURL); err != nil {
		log.Printf("[gfx_portal] Navigate warning: %v", err)
	}
	if ctx.Err() != nil {
		return nil, "", ctx.Err()
	}

	_, _ = page.Eval(`() => {
		try { localStorage.clear(); sessionStorage.clear(); } catch(e) {}
	}`)
	_, _ = page.Eval(`() => {
		if (!localStorage.getItem('device_fingerprint')) {
			const arr = new Uint8Array(16);
			crypto.getRandomValues(arr);
			localStorage.setItem('device_fingerprint', Array.from(arr).map(b => b.toString(16).padStart(2,'0')).join(''));
		}
	}`)

	for i := 0; i < 25; i++ {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		hasEmail, _, _ := page.Has("input[type='email']")
		if hasEmail {
			break
		}
		title := ""
		if info, err := page.Info(); err == nil {
			title = info.Title
		}
		if strings.Contains(title, "Just a moment") || strings.Contains(title, "Checking your browser") {
			solveCloudflare(page)
			time.Sleep(2 * time.Second)
			continue
		}
		time.Sleep(200 * time.Millisecond)
	}

	stopWatch, loginAPI := watchGFXLoginAPI(page)
	defer stopWatch()

	fp := portalDeviceFingerprint(page)
	log.Printf("[gfx_portal] Logging in as %s (fp=%s...)", username, truncStr(fp, 8))

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
		return nil, "", fmt.Errorf("fill credentials: %w", err)
	}

	_, _ = page.Eval(`() => {
		const btn = document.querySelector('button[type="submit"]');
		if (btn) { btn.click(); return true; }
		return false;
	}`)

	loggedIn := false
	for i := 0; i < 40; i++ {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		urlNow := safeURL()
		if urlNow != "" && !strings.Contains(urlNow, "signin") {
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
			_, _ = page.Eval(`() => {
				const btn = Array.from(document.querySelectorAll('button')).find(b => b.textContent && b.textContent.includes('Sign In Again'));
				if (btn) btn.click();
			}`)
		}
		time.Sleep(300 * time.Millisecond)
	}

	if !loggedIn {
		pageErr := readPageLoginError(page)
		saveErrorScreenshot(page, accountID, "signin_failed")
		return nil, "", fmt.Errorf("%s", formatGFXLoginFailure(accountID, loginAPI, pageErr))
	}

	status, body, seen := loginAPI.snapshot()
	if seen && status >= 400 {
		saveErrorScreenshot(page, accountID, "signin_api_fail")
		return nil, "", fmt.Errorf("%s", formatGFXLoginFailure(accountID, loginAPI, ""))
	}
	if seen && !loginBodyHasToken(body) {
		log.Printf("[gfx_portal] Login API HTTP %d but no accessToken in body — continuing to check page", status)
	} else if seen {
		log.Printf("[gfx_portal] Login API OK (HTTP %d, has token)", status)
	}

	if !strings.Contains(safeURL(), "signin") {
		_ = page.Timeout(30 * time.Second).Navigate(homeURL)
		for i := 0; i < 20; i++ {
			if u := safeURL(); u != "" && !strings.Contains(u, "signin") {
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
	}

	log.Printf("[gfx_portal] Login complete → %s", safeURL())
	return page, body, nil
}

func portalDeviceFingerprint(page *rod.Page) string {
	res, err := page.Eval(`() => localStorage.getItem('device_fingerprint') || ''`)
	if err == nil && !res.Value.Nil() {
		if fp := strings.TrimSpace(res.Value.Str()); fp != "" {
			return fp
		}
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	fp := hex.EncodeToString(b)
	_, _ = page.Eval(`(fp) => localStorage.setItem('device_fingerprint', fp)`, fp)
	return fp
}

func loginBodyHasToken(body string) bool {
	body = strings.TrimSpace(body)
	if body == "" {
		return false
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return strings.Contains(body, "accessToken") || strings.Contains(body, "eyJ")
	}
	for _, path := range []string{"accessToken", "refreshToken"} {
		if v, ok := parsed[path]; ok {
			if s, ok := v.(string); ok && len(s) > 20 {
				return true
			}
		}
	}
	if data, ok := parsed["data"].(map[string]interface{}); ok {
		if v, ok := data["accessToken"].(string); ok && len(v) > 20 {
			return true
		}
	}
	return false
}

func localStorageHasAuthSession(data map[string]interface{}) bool {
	for k, v := range data {
		kl := strings.ToLower(k)
		s, ok := v.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s == "" || s == "null" {
			continue
		}
		if kl == "accesstoken" || kl == "refreshtoken" {
			return len(s) > 20
		}
		if strings.Contains(kl, "auth") && len(s) > 40 {
			return true
		}
		if strings.HasPrefix(s, "eyJ") && len(s) > 80 {
			return true
		}
		if strings.Contains(kl, "persist") && (strings.Contains(s, "accessToken") || strings.Contains(s, `"email"`)) {
			return true
		}
	}
	return false
}

func portalPageLooksLoggedIn(page *rod.Page) bool {
	res, err := page.Eval(`() => {
		const url = location.href || '';
		if (url.includes('signin')) return false;
		if (document.querySelector('a[href*="/tools/"]')) return true;
		if (document.querySelector('[data-tool-cookie]')) return true;
		if (document.querySelector('[data-tool-id]')) return true;
		const text = (document.body && document.body.innerText) ? document.body.innerText.toLowerCase() : '';
		if (text.includes('sign in') && text.includes('password')) return false;
		return text.length > 500;
	}`)
	return err == nil && res.Value.Bool()
}

func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
