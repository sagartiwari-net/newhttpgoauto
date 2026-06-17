package gfx

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

func waitExtensionInject(page *rod.Page, polls int) bool {
	for i := 0; i < polls; i++ {
		res, err := page.Eval(`() => document.documentElement.hasAttribute('data-my-extension-installed')`)
		if err == nil && res.Value.Bool() {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// stabilizeToolPageAfterLogin navigates to the tool page, reloads, and waits for the
// GFX extension to inject access controls. Required after a fresh credential login —
// cookie sessions usually already have buttons on the page.
func stabilizeToolPageAfterLogin(ctx context.Context, page *rod.Page, toolURL, accountID string) error {
	log.Printf("[gfx_%s] Preparing tool page after fresh login: %s", accountID, toolURL)

	if err := page.Timeout(30 * time.Second).Navigate(toolURL); err != nil {
		log.Printf("[gfx_%s] Tool page navigation warning: %v", accountID, err)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	time.Sleep(2 * time.Second)

	if waitExtensionInject(page, 20) {
		log.Printf("[gfx_%s] Extension detected on tool page", accountID)
	} else {
		log.Printf("[gfx_%s] Extension not yet on tool page — will reload", accountID)
	}

	log.Printf("[gfx_%s] Reloading tool page so extension injects access button...", accountID)
	_ = page.Timeout(30 * time.Second).Reload()
	time.Sleep(2 * time.Second)
	if waitExtensionInject(page, 16) {
		log.Printf("[gfx_%s] Extension active after reload", accountID)
	}
	time.Sleep(2 * time.Second)

	dismissNonAuthDialogs(page)
	scrollPageForButtons(page)

	for i := 0; i < 50; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if gfxAccessButtonReady(page) {
			log.Printf("[gfx_%s] Access button ready after login (iter %d)", accountID, i)
			return nil
		}
		if i == 8 || i == 24 {
			scrollPageForButtons(page)
		}
		if i == 16 {
			log.Printf("[gfx_%s] Still no access button — reloading tool page again", accountID)
			_ = page.Timeout(30 * time.Second).Reload()
			time.Sleep(2 * time.Second)
			waitExtensionInject(page, 12)
			time.Sleep(1500 * time.Millisecond)
			dismissNonAuthDialogs(page)
			scrollPageForButtons(page)
		}
		time.Sleep(400 * time.Millisecond)
	}
	return nil
}

func dismissNonAuthDialogs(page *rod.Page) {
	_, _ = page.Eval(`() => {
		const ev = new KeyboardEvent('keydown', { key: 'Escape', keyCode: 27, bubbles: true });
		document.dispatchEvent(ev);
		document.querySelectorAll('button[aria-label="Close"], button.close, [class*="close"], [class*="dismiss"]').forEach(b => {
			try { b.click(); } catch(e) {}
		});
	}`)
}

// ensureAccessButtonOnToolPage reloads once when buttons are missing on an already-authed page.
func ensureAccessButtonOnToolPage(ctx context.Context, page *rod.Page, tool ToolDef, accountID string) {
	if gfxAccessButtonReady(page) {
		return
	}
	log.Printf("[gfx_%s] Access button missing — reloading %s for extension inject", tool.WebsiteID, tool.ToolURL)
	if info, err := page.Info(); err != nil || !strings.Contains(info.URL, "/tools/") {
		_ = page.Timeout(30 * time.Second).Navigate(tool.ToolURL)
	} else {
		_ = page.Timeout(30 * time.Second).Reload()
	}
	time.Sleep(2 * time.Second)
	waitExtensionInject(page, 12)
	time.Sleep(1500 * time.Millisecond)
	dismissNonAuthDialogs(page)
	scrollPageForButtons(page)
	_ = ctx.Err()
}
