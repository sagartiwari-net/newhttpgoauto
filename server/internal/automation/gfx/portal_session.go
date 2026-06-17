package gfx

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/go-rod/rod"
)

// ensurePortalSession opens the isolated portal profile.
// Uses cached portal session if already logged in; otherwise injects cookies exported from the main GFX profile.
// Does NOT perform credential login on the portal profile.
func ensurePortalSession(ctx context.Context, session *Session, homeURL string) (*rod.Page, error) {
	page := session.newPage()
	accountID := session.Slot().Account.WebsiteID

	if homeURL == "" {
		homeURL = gfxPortalHomeURL
	}

	log.Printf("[gfx_portal] Opening homepage (portal profile cache check)...")
	if err := page.Timeout(45 * time.Second).Navigate(homeURL); err != nil {
		log.Printf("[gfx_portal] Navigate warning: %v", err)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	for i := 0; i < 15; i++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if portalPageLooksLoggedIn(page) {
			log.Printf("[gfx_portal] ✅ Portal profile already logged in — skipped cookie inject")
			return page, nil
		}
		time.Sleep(400 * time.Millisecond)
	}

	cookies, err := loadPortalSeedCookies(accountID)
	if err != nil {
		saveErrorScreenshot(page, accountID, "no_seed_cookies")
		return nil, err
	}

	log.Printf("[gfx_portal] Injecting %d cookies from main GFX profile seed...", len(cookies))
	_ = injectCookiesIntoPage(page, cookies)

	if err := page.Timeout(30 * time.Second).Navigate(homeURL); err != nil {
		log.Printf("[gfx_portal] Re-navigate after inject warning: %v", err)
	}

	for i := 0; i < 20; i++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if portalPageLooksLoggedIn(page) {
			log.Printf("[gfx_portal] ✅ Logged in via main profile cookies")
			return page, nil
		}
		time.Sleep(400 * time.Millisecond)
	}

	saveErrorScreenshot(page, accountID, "cookie_inject_failed")
	return nil, fmt.Errorf("portal cookie inject failed — run any GFX tool task first to refresh main profile session")
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
