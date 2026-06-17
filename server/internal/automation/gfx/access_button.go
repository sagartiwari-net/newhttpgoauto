package gfx

import (
	"log"
	"time"

	"github.com/go-rod/rod"
)

type accessButtonMatch struct {
	found       bool
	selector    string
	useFallback bool
	fallbackIdx int
}

func scrollForLazyButtons(page *rod.Page) {
	log.Printf("[GFX] Scrolling page to trigger lazy buttons...")
	_, _ = page.Eval(`async () => {
		const step = 400;
		const maxY = document.body.scrollHeight;
		for (let y = 0; y < maxY; y += step) {
			window.scrollTo(0, y);
			await new Promise(r => setTimeout(r, 120));
		}
		window.scrollTo(0, 0);
	}`)
}

// locateAccessButton finds the GFX tool launch button (data-tool-cookie or "Access Now" text).
func locateAccessButton(page *rod.Page, tool ToolDef) accessButtonMatch {
	if has, _, _ := page.Has(tool.Selector); has {
		return accessButtonMatch{found: true, selector: tool.Selector}
	}

	res, err := page.Eval(`(idx) => {
		const list = document.querySelectorAll('button[data-tool-cookie="true"]');
		if (list.length > idx) return { found: true, actualIndex: idx };
		if (list.length > 0) return { found: true, actualIndex: 0 };
		return { found: false, actualIndex: -1 };
	}`, tool.FallbackIndex)
	if err == nil && res.Value.Get("found").Bool() {
		return accessButtonMatch{
			found:       true,
			useFallback: true,
			fallbackIdx: res.Value.Get("actualIndex").Int(),
		}
	}

	res, err = page.Eval(`() => {
		const candidates = [...document.querySelectorAll('button, a[role="button"], a.btn, a[class*="button"]')];
		const isAccess = (el) => {
			const t = (el.textContent || '').replace(/\\s+/g, ' ').trim().toLowerCase();
			return t.includes('access now') || t === 'access' || t.startsWith('access ');
		};
		let match = candidates.find(isAccess);
		if (!match) {
			match = candidates.find(el => {
				const t = (el.textContent || '').replace(/\\s+/g, ' ').trim().toLowerCase();
				return t.includes('launch') || t.includes('open tool') || t.includes('get access');
			});
		}
		if (match) {
			match.setAttribute('data-gfx-access-btn', '1');
			return true;
		}
		return false;
	}`)
	if err == nil && res.Value.Bool() {
		return accessButtonMatch{found: true, selector: `[data-gfx-access-btn="1"]`}
	}

	for _, sel := range []string{
		"button.cookie-btn",
		"button.bg-gradient-to-r",
		`button[class*="from-cyan"]`,
		`button[class*="from-blue"]`,
	} {
		if has, _, _ := page.Has(sel); has {
			return accessButtonMatch{found: true, selector: sel}
		}
	}

	return accessButtonMatch{}
}

func clickAccessButton(page *rod.Page, match accessButtonMatch, tool ToolDef) (bool, error) {
	res, err := page.Eval(`(sel, idx, useFB) => {
		let btn;
		if (useFB) {
			btn = document.querySelectorAll('button[data-tool-cookie="true"]')[idx];
		} else {
			btn = document.querySelector(sel);
		}
		if (!btn) return false;
		btn.scrollIntoView({ block: 'center', inline: 'center' });
		btn.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, cancelable: true, view: window }));
		btn.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, cancelable: true, view: window }));
		btn.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, view: window }));
		if (typeof btn.click === 'function') btn.click();
		return true;
	}`, match.selector, match.fallbackIdx, match.useFallback)
	if err != nil {
		return false, err
	}
	return res.Value.Bool(), nil
}

func waitForAccessButton(page *rod.Page, tool ToolDef) accessButtonMatch {
	scrollForLazyButtons(page)
	for i := 0; i < 32; i++ {
		if m := locateAccessButton(page, tool); m.found {
			if m.useFallback {
				log.Printf("[gfx_%s] Found access button via data-tool-cookie index %d", tool.WebsiteID, m.fallbackIdx)
			} else {
				log.Printf("[gfx_%s] Found access button via selector: %s", tool.WebsiteID, m.selector)
			}
			return m
		}
		if i == 6 || i == 14 {
			scrollForLazyButtons(page)
		}
		time.Sleep(300 * time.Millisecond)
	}
	return accessButtonMatch{}
}
