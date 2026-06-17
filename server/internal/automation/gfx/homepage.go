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
)

const gfxPortalHomeURL = "https://app.gfxtoolz.ai/"

func runPortalHomepage(ctx context.Context, session *Session, tool ToolDef) error {
	accountID := session.Slot().Account.WebsiteID
	log.Printf("[gfx_portal] Capturing GFX homepage session (account=%s, profile=%s)", accountID, session.Slot().ProfileDir)

	page, err := ensureGFXLogin(ctx, session, gfxPortalHomeURL)
	if err != nil {
		return err
	}

	for i := 0; i < 20; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		info, err := page.Info()
		if err == nil && info.URL != "" && !strings.Contains(info.URL, "signin") {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	info, err := page.Info()
	if err != nil {
		return fmt.Errorf("portal page info: %w", err)
	}
	if strings.Contains(info.URL, "signin") {
		saveErrorScreenshot(page, accountID, "portal_signin")
		return fmt.Errorf("still on sign-in page after login")
	}

	log.Printf("[gfx_portal] Homepage ready: %s — settling 4s...", info.URL)
	time.Sleep(4 * time.Second)

	rawCookies, err := cookiesForDomain(page, "gfxtoolz.ai")
	if err != nil {
		return err
	}
	localStorageData := readLocalStorage(page)

	if len(rawCookies) == 0 && len(localStorageData) == 0 {
		saveErrorScreenshot(page, accountID, "portal_empty")
		return fmt.Errorf("no cookies or localStorage captured from GFX homepage")
	}

	var cookieHeaderString string
	if len(rawCookies) > 0 {
		var pairs []string
		for _, c := range rawCookies {
			pairs = append(pairs, fmt.Sprintf("%s=%s", c.Name, c.Value))
		}
		cookieHeaderString = strings.Join(pairs, "; ")
	}

	payload := map[string]interface{}{
		"task_uid":        tool.TaskUID,
		"account_id":      accountID,
		"portal_url":      info.URL,
		"captured_at":     time.Now().UTC().Format(time.RFC3339),
		"domain":          "gfxtoolz.ai",
		"cookies":         rawCookies,
		"localStorage":    localStorageData,
		"cookies_header":  cookieHeaderString,
		"cookies_netscape": cookiesToNetscape(rawCookies),
		"note":            "local file only — not saved to shared_sessions DB",
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

	// Easy access copy on Desktop (worker Mac).
	if home := os.Getenv("HOME"); home != "" {
		desktopPath := filepath.Join(home, "Desktop", "gfx_portal_homepage.json")
		if err := os.WriteFile(desktopPath, jsonBytes, 0644); err != nil {
			log.Printf("[gfx_portal] Desktop copy failed: %v", err)
		} else {
			log.Printf("[gfx_portal] Desktop copy: %s", desktopPath)
		}
	}

	log.Printf("[gfx_portal] ✅ Saved homepage session to %s (%d cookies, %d LS keys)",
		outPath, len(rawCookies), len(localStorageData))
	return nil
}
