package gfx

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

const gfxPortalHomeURL = "https://app.gfxtoolz.ai/"

func runPortalHomepage(ctx context.Context, session *Session, tool ToolDef) error {
	accountID := session.Slot().Account.WebsiteID
	log.Printf("[gfx_portal] Capturing GFX homepage session (account=%s, profile=%s)", accountID, session.Slot().ProfileDir)

	page, err := ensureGFXLogin(ctx, session, gfxPortalHomeURL)
	if err != nil {
		return err
	}

	if err := waitForPortalDashboard(ctx, page); err != nil {
		saveErrorScreenshot(page, accountID, "portal_dashboard")
		return err
	}

	log.Printf("[gfx_portal] Dashboard visible — hard reload for fresh client state...")
	if err := page.Timeout(30 * time.Second).Reload(); err != nil {
		log.Printf("[gfx_portal] Reload warning: %v", err)
	}
	if err := waitForPortalDashboard(ctx, page); err != nil {
		saveErrorScreenshot(page, accountID, "portal_reload")
		return err
	}

	log.Printf("[gfx_portal] Settling 6s then polling localStorage...")
	time.Sleep(6 * time.Second)

	localStorageData := waitForPortalLocalStorage(ctx, page, 12*time.Second)
	if len(localStorageData) == 0 {
		saveErrorScreenshot(page, accountID, "portal_empty")
		return fmt.Errorf("no localStorage captured from GFX homepage")
	}
	if !portalLocalStorageReady(localStorageData) {
		keys := storageKeyNames(localStorageData)
		saveErrorScreenshot(page, accountID, "portal_not_ready")
		return fmt.Errorf("portal localStorage session not ready (keys=%v)", keys)
	}

	referer := gfxPortalHomeURL
	if info, err := page.Info(); err == nil && info.URL != "" && !strings.Contains(info.URL, "signin") {
		referer = info.URL
	}

	payload := map[string]interface{}{
		"referer":         referer,
		"includedFormats": []string{"localStorage"},
		"storage": map[string]interface{}{
			"localStorage": localStorageData,
		},
	}

	outPath := portalHomepageCookieFile()
	_ = os.MkdirAll(filepath.Dir(outPath), 0755)
	jsonBytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(outPath, jsonBytes, 0644); err != nil {
		return fmt.Errorf("write portal cookie file: %w", err)
	}

	log.Printf("[gfx_portal] ✅ Saved homepage localStorage session to %s (%d keys, fp=%s)",
		outPath, len(localStorageData), localStorageData["device_fingerprint"])
	return nil
}

func waitForPortalDashboard(ctx context.Context, page *rod.Page) error {
	for i := 0; i < 40; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		res, err := page.Eval(`() => {
			const url = location.href || '';
			if (url.includes('signin')) return false;
			if (document.querySelector('a[href*="/tools/"]')) return true;
			if (document.querySelector('[data-tool-cookie]')) return true;
			if (document.querySelector('[data-tool-id]')) return true;
			const text = (document.body && document.body.innerText) ? document.body.innerText : '';
			return text.length > 400;
		}`)
		if err == nil && res.Value.Bool() {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	info, _ := page.Info()
	url := ""
	if info != nil {
		url = info.URL
	}
	return fmt.Errorf("portal dashboard did not finish loading (url=%s)", url)
}

func waitForPortalLocalStorage(ctx context.Context, page *rod.Page, maxWait time.Duration) map[string]interface{} {
	deadline := time.Now().Add(maxWait)
	var best map[string]interface{}
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			break
		}
		data := readPortalStorage(page)
		if portalLocalStorageReady(data) {
			return data
		}
		if len(data) > len(best) {
			best = data
		}
		time.Sleep(500 * time.Millisecond)
	}
	return best
}

func portalLocalStorageReady(data map[string]interface{}) bool {
	if len(data) < 4 {
		return false
	}
	fp, _ := data["device_fingerprint"].(string)
	if strings.TrimSpace(fp) == "" {
		return false
	}

	hasAuthKey := false
	for k := range data {
		kl := strings.ToLower(k)
		if strings.Contains(kl, "auth") || strings.Contains(kl, "token") || strings.Contains(kl, "firebase") {
			hasAuthKey = true
			break
		}
	}

	posthogReady := false
	for k, v := range data {
		if !strings.Contains(k, "posthog") {
			continue
		}
		s, ok := v.(string)
		if !ok {
			continue
		}
		if strings.Contains(s, "$sess_rec_flush_size") || strings.Contains(s, `"$user_state":"identified"`) {
			posthogReady = true
			break
		}
	}

	guestFresh := guestIDIsFresh(data["popup_guest_id"])
	if hasAuthKey {
		return true
	}
	if posthogReady && guestFresh {
		return true
	}
	if guestFresh && len(data) >= 7 {
		return true
	}
	return false
}

func guestIDIsFresh(v interface{}) bool {
	guest, ok := v.(string)
	if !ok || !strings.HasPrefix(guest, "guest_") {
		return false
	}
	parts := strings.Split(guest, "_")
	if len(parts) < 2 {
		return false
	}
	ts, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || ts <= 0 {
		return false
	}
	age := time.Now().UnixMilli() - ts
	return age >= 0 && age <= 3*60*1000
}

func storageKeyNames(data map[string]interface{}) []string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	return keys
}
