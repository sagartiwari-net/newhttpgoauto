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

	localStorageData := readLocalStorage(page)
	if len(localStorageData) == 0 {
		saveErrorScreenshot(page, accountID, "portal_empty")
		return fmt.Errorf("no localStorage captured from GFX homepage")
	}

	referer := info.URL
	if referer == "" {
		referer = gfxPortalHomeURL
	}

	// Same format as GFX extension tools with UseLSPayloadFormat (tools.1clkaccess.store inject).
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

	log.Printf("[gfx_portal] ✅ Saved homepage localStorage session to %s (%d LS keys)", outPath, len(localStorageData))
	return nil
}
