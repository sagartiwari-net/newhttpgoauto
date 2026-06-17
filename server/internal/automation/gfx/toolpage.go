package gfx

import (
	"context"
	"fmt"
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

func dismissNonAuthDialogs(page *rod.Page) {
	_, _ = page.Eval(`() => {
		const ev = new KeyboardEvent('keydown', { key: 'Escape', keyCode: 27, bubbles: true });
		document.dispatchEvent(ev);
		document.querySelectorAll('button[aria-label="Close"], button.close, [class*="close"], [class*="dismiss"]').forEach(b => {
			try { b.click(); } catch(e) {}
		});
	}`)
}

// openToolPageAfterRelaunch opens the tool on a freshly relaunched browser (post-login cookies in profile).
func openToolPageAfterRelaunch(ctx context.Context, page *rod.Page, toolURL, accountID string) error {
	log.Printf("[gfx_%s] Opening tool page on relaunched browser: %s", accountID, toolURL)

	if err := page.Timeout(45 * time.Second).Navigate(toolURL); err != nil {
		log.Printf("[gfx_%s] Tool navigation warning: %v", accountID, err)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	time.Sleep(2 * time.Second)

	if waitExtensionInject(page, 24) {
		log.Printf("[gfx_%s] Extension active on relaunched browser", accountID)
	} else {
		log.Printf("[gfx_%s] Extension not detected yet — reloading tool page", accountID)
		_ = page.Timeout(30 * time.Second).Reload()
		time.Sleep(2 * time.Second)
		waitExtensionInject(page, 16)
	}
	time.Sleep(2 * time.Second)
	dismissNonAuthDialogs(page)
	scrollPageForButtons(page)

	for i := 0; i < 45; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if gfxAccessButtonReady(page) {
			log.Printf("[gfx_%s] Access button ready on relaunched browser (iter %d)", accountID, i)
			return nil
		}
		if i == 6 || i == 20 {
			scrollPageForButtons(page)
		}
		if i == 12 {
			log.Printf("[gfx_%s] Reloading tool page on relaunched browser", accountID)
			_ = page.Timeout(30 * time.Second).Reload()
			time.Sleep(2 * time.Second)
			waitExtensionInject(page, 12)
			dismissNonAuthDialogs(page)
			scrollPageForButtons(page)
		}
		time.Sleep(400 * time.Millisecond)
	}
	return fmt.Errorf("access button not ready after browser relaunch for %s", accountID)
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
