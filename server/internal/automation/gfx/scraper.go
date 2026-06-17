package gfx

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"gohttpauto/internal/db"

	"github.com/go-rod/rod"
)

func runCredScraper(ctx context.Context, session *Session, tool ToolDef) error {
	log.Printf("[GFX] Cred scrape %s (%s)", tool.ScrapeName, tool.ToolURL)
	page := session.newPage()

	_ = page.Timeout(25 * time.Second).Navigate(tool.ToolURL)

	buttonsLoaded := false
	for i := 0; i < 20; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		res, err := page.Eval(`() => {
			const btns = Array.from(document.querySelectorAll('button'));
			const hasUser = btns.some(b => b.textContent && b.textContent.toLowerCase().includes('user'));
			const hasPass = btns.some(b => b.textContent && b.textContent.toLowerCase().includes('pass'));
			return hasUser && hasPass;
		}`)
		if err == nil && res.Value.Bool() {
			buttonsLoaded = true
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !buttonsLoaded {
		log.Println("[GFX] Credentials buttons did not appear within 20s — continuing...")
	}

	_, _ = page.Eval(`() => {
		window.lastCopiedText = "";
		window.lastOpenedUrl = "";
		navigator.clipboard.writeText = async (text) => { window.lastCopiedText = text; return true; };
		window.open = (url) => { window.lastOpenedUrl = url; return { close: () => {} }; };
		return true;
	}`)
	time.Sleep(1500 * time.Millisecond)

	toolName := tool.ScrapeName
	resName, err := page.Eval(`() => {
		const h4 = document.querySelector('h4');
		if (h4 && h4.textContent) return h4.textContent.trim();
		return "";
	}`)
	if err == nil && strings.TrimSpace(resName.Value.Str()) != "" {
		toolName = strings.TrimSpace(resName.Value.Str())
	}

	usernameVal := scrapeClipboard(page, "user")
	passwordVal := scrapeClipboard(page, "pass")
	targetURLVal := scrapeOpenURL(page)

	if usernameVal == "" || passwordVal == "" {
		group := tool.WebsiteID
		if group == "" {
			group = tool.TaskUID
		}
		saveErrorScreenshot(page, group, "scrape_failed")
		return fmt.Errorf("failed to extract credentials for %s", toolName)
	}

	_, err = db.DB.ExecContext(ctx, `
		INSERT INTO scraped_credentials (source_platform, website_name, login_url, username, password)
		VALUES ('gfx', ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE login_url = VALUES(login_url), password = VALUES(password), updated_at = NOW()
	`, toolName, targetURLVal, usernameVal, passwordVal)
	if err != nil {
		return fmt.Errorf("db write for %s: %w", toolName, err)
	}
	log.Printf("[GFX] Stored scraped credentials for %s", toolName)
	return nil
}

func scrapeClipboard(page *rod.Page, kind string) string {
	for attempt := 1; attempt <= 5; attempt++ {
		_, _ = page.Eval(`() => { window.lastCopiedText = ""; return true; }`)
		_, _ = page.Eval(`(label) => {
			const btns = Array.from(document.querySelectorAll('button'));
			const btn = btns.find(b => b.textContent && b.textContent.toLowerCase().includes(label));
			if (btn) { btn.click(); return true; }
			return false;
		}`, kind)
		for p := 0; p < 4; p++ {
			time.Sleep(500 * time.Millisecond)
			res, err := page.Eval(`() => window.lastCopiedText`)
			if err == nil {
				v := strings.TrimSpace(res.Value.Str())
				if v != "" {
					return v
				}
			}
		}
		time.Sleep(1 * time.Second)
	}
	return ""
}

func scrapeOpenURL(page *rod.Page) string {
	for attempt := 1; attempt <= 5; attempt++ {
		_, _ = page.Eval(`() => { window.lastOpenedUrl = ""; return true; }`)
		_, _ = page.Eval(`() => {
			const btns = Array.from(document.querySelectorAll('button'));
			const openBtn = btns.find(b => b.textContent && b.textContent.toLowerCase().includes('open website'));
			if (openBtn) { openBtn.click(); return true; }
			return false;
		}`)
		for p := 0; p < 4; p++ {
			time.Sleep(500 * time.Millisecond)
			res, err := page.Eval(`() => window.lastOpenedUrl`)
			if err == nil {
				v := strings.TrimSpace(res.Value.Str())
				if v != "" {
					return v
				}
			}
		}
		time.Sleep(1 * time.Second)
	}
	return ""
}
