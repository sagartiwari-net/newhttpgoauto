package gfx

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

// waitForToolPage waits for GFX to open the target tool (new tab or same-tab navigation).
func waitForToolPage(ctx context.Context, browser *rod.Browser, portalPage *rod.Page, tool ToolDef) (*rod.Page, error) {
	refererHost := ""
	if tool.Referer != "" {
		if u, err := url.Parse(tool.Referer); err == nil {
			refererHost = strings.TrimPrefix(u.Hostname(), "www.")
		}
	}
	if refererHost == "" {
		refererHost = tool.WebsiteID
	}

	portalID := portalPage.TargetID
	log.Printf("[gfx_%s] Waiting for tool page (%s)...", tool.WebsiteID, refererHost)

	for i := 0; i < 24; i++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if info, err := portalPage.Info(); err == nil {
			host := hostFromURL(info.URL)
			if host != "" && host != "app.gfxtoolz.ai" && strings.Contains(host, refererHost) {
				log.Printf("[gfx_%s] Tool opened in same tab: %s", tool.WebsiteID, info.URL)
				return portalPage, nil
			}
		}

		pages, err := browser.Pages()
		if err == nil {
			for _, p := range pages {
				if p.TargetID == portalID {
					continue
				}
				info, err := p.Info()
				if err != nil {
					continue
				}
				host := hostFromURL(info.URL)
				if host == "" || host == "about:blank" {
					continue
				}
				if strings.Contains(host, refererHost) || strings.Contains(info.URL, refererHost) {
					log.Printf("[gfx_%s] Tool opened in new tab: %s", tool.WebsiteID, info.URL)
					return p, nil
				}
			}
		}

		time.Sleep(500 * time.Millisecond)
	}

	shot := saveErrorScreenshot(portalPage, tool.WebsiteID, "tool_tab_timeout")
	msg := fmt.Sprintf("tool page did not open within 20s (expected %s)", refererHost)
	if shot != "" {
		msg += " | screenshot: " + shot
	}
	return nil, fmt.Errorf("%s", msg)
}

func hostFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Hostname(), "www.")
}
