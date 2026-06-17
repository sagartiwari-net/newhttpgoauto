package gfx

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

const gfxPortalHomeURL = "https://app.gfxtoolz.ai/"

func runPortalHomepage(ctx context.Context, session *Session, tool ToolDef) error {
	accountID := session.Slot().Account.WebsiteID
	log.Printf("[gfx_portal] Capturing GFX homepage session (account=%s, profile=%s)", accountID, session.Slot().ProfileDir)

	var page *rod.Page
	defer func() {
		if page != nil && ctx.Err() != nil {
			saveErrorScreenshot(page, accountID, "task_timeout")
			saveCaptureScreenshot(page, accountID, "task_timeout_debug")
			log.Printf("[gfx_portal] Task cancelled/timed out — screenshot saved (url check logs above)")
		}
	}()

	var loginBody string
	var err error
	page, loginBody, err = ensurePortalLogin(ctx, session, gfxPortalHomeURL)
	if err != nil {
		if page != nil {
			saveErrorScreenshot(page, accountID, "signin_error")
		}
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

	log.Printf("[gfx_portal] Settling 5s then polling localStorage...")
	time.Sleep(5 * time.Second)

	localStorageData := waitForPortalLocalStorage(ctx, page, 10*time.Second)
	if len(localStorageData) == 0 {
		saveErrorScreenshot(page, accountID, "portal_empty")
		return fmt.Errorf("no localStorage captured from GFX homepage")
	}

	mergeLoginTokensIntoStorage(localStorageData, loginBody)

	if !portalLocalStorageReady(localStorageData) || !portalPageLooksLoggedIn(page) {
		keys := storageKeyNames(localStorageData)
		saveErrorScreenshot(page, accountID, "portal_not_ready")
		saveCaptureScreenshot(page, accountID, "portal_not_ready_debug")
		return fmt.Errorf("portal session not ready (keys=%v, loggedIn=%v)", keys, portalPageLooksLoggedIn(page))
	}

	referer := gfxPortalHomeURL
	pageURL := ""
	if info, err := page.Info(); err == nil && info.URL != "" && !strings.Contains(info.URL, "signin") {
		referer = info.URL
		pageURL = info.URL
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

	shotPath := saveCaptureScreenshot(page, accountID, "portal_home_saved")
	log.Printf("[gfx_portal] ✅ Saved homepage localStorage to %s (%d keys, url=%s, screenshot=%s)",
		outPath, len(localStorageData), pageURL, shotPath)
	return nil
}

func mergeLoginTokensIntoStorage(data map[string]interface{}, loginBody string) {
	loginBody = strings.TrimSpace(loginBody)
	if loginBody == "" {
		return
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(loginBody), &parsed); err != nil {
		return
	}
	if at := tokenFromLoginJSON(parsed, "accessToken"); at != "" {
		data["accessToken"] = at
	}
	if rt := tokenFromLoginJSON(parsed, "refreshToken"); rt != "" {
		data["refreshToken"] = rt
	}
	if user, ok := parsed["user"]; ok {
		if b, err := json.Marshal(user); err == nil {
			data["user"] = string(b)
		}
	}
}

func tokenFromLoginJSON(parsed map[string]interface{}, key string) string {
	if v, ok := parsed[key].(string); ok && len(v) > 10 {
		return v
	}
	if data, ok := parsed["data"].(map[string]interface{}); ok {
		if v, ok := data[key].(string); ok && len(v) > 10 {
			return v
		}
	}
	return ""
}

func waitForPortalDashboard(ctx context.Context, page *rod.Page) error {
	for i := 0; i < 30; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if portalPageLooksLoggedIn(page) {
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
	return localStorageHasAuthSession(data)
}

func storageKeyNames(data map[string]interface{}) []string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	return keys
}
