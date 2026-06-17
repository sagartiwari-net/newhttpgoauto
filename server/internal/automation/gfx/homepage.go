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
		}
	}()

	var err error
	page, err = ensurePortalSession(ctx, session, gfxPortalHomeURL)
	if err != nil {
		return err
	}

	log.Printf("[gfx_portal] Settling 4s for localStorage...")
	time.Sleep(4 * time.Second)

	localStorageData := readPortalStorage(page)
	if len(localStorageData) == 0 {
		localStorageData = waitForPortalLocalStorage(ctx, page, 8*time.Second)
	}
	if len(localStorageData) == 0 {
		saveErrorScreenshot(page, accountID, "portal_empty")
		return fmt.Errorf("no localStorage captured from GFX homepage")
	}

	if !portalPageLooksLoggedIn(page) {
		saveErrorScreenshot(page, accountID, "portal_not_logged_in")
		return fmt.Errorf("portal page not logged in after cookie session")
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

func waitForPortalLocalStorage(ctx context.Context, page *rod.Page, maxWait time.Duration) map[string]interface{} {
	deadline := time.Now().Add(maxWait)
	var best map[string]interface{}
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			break
		}
		data := readPortalStorage(page)
		if len(data) > len(best) {
			best = data
		}
		if len(data) >= 4 {
			return data
		}
		time.Sleep(400 * time.Millisecond)
	}
	return best
}

func storageKeyNames(data map[string]interface{}) []string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	return keys
}
