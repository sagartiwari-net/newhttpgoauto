package gfx

import (
	"fmt"
	"log"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// accessButtonMatch finds the GFX tool access control on the page and tags it for clicking.
const accessButtonMatchJS = `() => {
	const norm = (s) => (s||'').replace(/\s+/g,' ').trim().toLowerCase();
	const texts = ['access now','get access','launch tool','open tool','access tool','start tool','launch app','open app','launch','get started'];
	const pick = (el) => {
		if (!el || el.disabled) return false;
		const t = norm(el.textContent);
		if (!t) return false;
		return texts.some(x => t.includes(x)) || t === 'access';
	};
	const cookie = document.querySelector('[data-tool-cookie="true"]');
	if (cookie) {
		cookie.setAttribute('data-gfx-access-btn','1');
		return { found: true, mode: 'cookie' };
	}
	const candidates = [...document.querySelectorAll('button,a,[role="button"],div[role="button"]')];
	for (const el of candidates) {
		if (pick(el)) {
			el.setAttribute('data-gfx-access-btn','1');
			return { found: true, mode: 'text' };
		}
	}
	return { found: false, mode: '' };
}`

func gfxAccessButtonReady(page *rod.Page) bool {
	res, err := page.Eval(accessButtonMatchJS)
	return err == nil && res.Value.Get("found").Bool()
}

func scrollPageForButtons(page *rod.Page) {
	_, _ = page.Eval(`async () => {
		const step = 400;
		const maxY = Math.max(document.body.scrollHeight, document.documentElement.scrollHeight);
		for (let y = 0; y <= maxY; y += step) {
			window.scrollTo(0, y);
			await new Promise(r => setTimeout(r, 120));
		}
		window.scrollTo(0, 0);
	}`)
}

type accessButtonHit struct {
	found       bool
	selector    string
	useFallback bool
	index       int
}

func findAccessButton(page *rod.Page, tool ToolDef) accessButtonHit {
	hasBtn, _, _ := page.Has(tool.Selector)
	if hasBtn {
		return accessButtonHit{found: true, selector: tool.Selector}
	}

	resCheck, errCheck := page.Eval(`(idx) => {
		const list = document.querySelectorAll('button[data-tool-cookie="true"]');
		if (list.length > idx) return { found: true, actualIndex: idx };
		if (list.length > 0) return { found: true, actualIndex: 0 };
		return { found: false, actualIndex: -1 };
	}`, tool.FallbackIndex)
	if errCheck == nil && resCheck.Value.Get("found").Bool() {
		return accessButtonHit{
			found:       true,
			useFallback: true,
			index:       resCheck.Value.Get("actualIndex").Int(),
		}
	}

	resAccess, errAccess := page.Eval(accessButtonMatchJS)
	if errAccess == nil && resAccess.Value.Get("found").Bool() {
		return accessButtonHit{found: true, selector: `[data-gfx-access-btn="1"]`}
	}

	return accessButtonHit{}
}

func waitForAccessButton(page *rod.Page, tool ToolDef, websiteID string) (accessButtonHit, error) {
	scrollPageForButtons(page)

	for idxPoll := 0; idxPoll < 40; idxPoll++ {
		if hit := findAccessButton(page, tool); hit.found {
			if hit.useFallback && hit.index != tool.FallbackIndex {
				log.Printf("[gfx_%s] Fallback index %d unavailable, using index %d", websiteID, tool.FallbackIndex, hit.index)
			}
			if hit.selector == `[data-gfx-access-btn="1"]` {
				log.Printf("[gfx_%s] Found access button via text/cookie scan", websiteID)
			}
			return hit, nil
		}
		if idxPoll == 3 || idxPoll == 12 {
			log.Printf("[gfx_%s] Scrolling again to reveal lazy access button...", websiteID)
			scrollPageForButtons(page)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return accessButtonHit{}, fmt.Errorf("access button not found on page (selector: %s, fallbackIndex: %d)", tool.Selector, tool.FallbackIndex)
}

func clickAccessButton(page *rod.Page, tool ToolDef, hit accessButtonHit, websiteID string) error {
	resClick, err := page.Eval(`(sel, idx, useFB) => {
		let btn;
		if (useFB) {
			btn = document.querySelectorAll('button[data-tool-cookie="true"]')[idx];
		} else {
			btn = document.querySelector(sel);
		}
		if (btn) {
			btn.scrollIntoView({ block: 'center', behavior: 'instant' });
			btn.click();
			return true;
		}
		return false;
	}`, hit.selector, hit.index, hit.useFallback)
	if err != nil {
		return fmt.Errorf("failed to click access button: %w", err)
	}
	if resClick != nil && resClick.Value.Bool() {
		return nil
	}

	// Rod native click fallback when React ignores synthetic click.
	if hit.useFallback {
		resIdx, errIdx := page.Eval(`(idx) => {
			const btn = document.querySelectorAll('button[data-tool-cookie="true"]')[idx];
			if (!btn) return false;
			btn.scrollIntoView({ block: 'center' });
			btn.click();
			return true;
		}`, hit.index)
		if errIdx == nil && resIdx != nil && resIdx.Value.Bool() {
			return nil
		}
	} else if hit.selector != "" {
		el, err := page.Element(hit.selector)
		if err == nil {
			_ = el.ScrollIntoView()
			if err := el.Click(proto.InputMouseButtonLeft, 1); err == nil {
				return nil
			}
		}
	}
	saveErrorScreenshot(page, websiteID, "click_failed")
	return fmt.Errorf("SSO access button click failed (selector: %s, fallback: %v)", hit.selector, hit.useFallback)
}
