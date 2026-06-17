package gfx

import (
	"context"
	"log"
	"time"
)

const taskTimeout = 75 * time.Second

// Run executes a GFX task with pool routing and parallel Chrome limits.
func Run(taskUID string) (status, msg string) {
	tool, ok := ToolFor(taskUID)
	if !ok {
		return "failed", "unknown gfx task: " + taskUID
	}

	AcquireParallel()
	defer ReleaseParallel()

	timeout := taskTimeout
	if tool.Kind == KindPortalHome {
		timeout = 95 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var slot Slot
	var err error
	lockKey := ""
	if tool.Kind == KindPortalHome {
		slot, err = ResolvePortalSlot(ctx, taskUID)
		if err != nil {
			return "failed", err.Error()
		}
		lockKey = "portal-" + slot.Account.WebsiteID
	} else {
		slot, err = ResolveSlot(ctx, taskUID)
		if err != nil {
			return "failed", err.Error()
		}
		lockKey = slot.Account.WebsiteID
	}

	mu := ProfileLock(lockKey)
	mu.Lock()
	defer mu.Unlock()

	log.Printf("[GFX] Task %s → account %s profile %s", taskUID, slot.Account.WebsiteID, slot.ProfileDir)

	var session *Session
	if tool.Kind == KindPortalHome {
		session, err = newPortalSession(ctx, slot)
	} else {
		session, err = newSession(ctx, slot)
	}
	if err != nil {
		return "failed", "chrome launch failed: " + err.Error()
	}
	defer session.Close()

	switch tool.Kind {
	case KindExtension:
		gfxPage, err := ensureGFXLogin(ctx, session, tool.ToolURL)
		if err != nil {
			return "failed", "gfx login failed: " + err.Error()
		}
		if err := runExtension(ctx, session, tool, gfxPage); err != nil {
			return "failed", err.Error()
		}
		return "success", tool.Name + " session captured (" + slot.Account.WebsiteID + ")"
	case KindPortalHome:
		if err := runPortalHomepage(ctx, session, tool); err != nil {
			return "failed", err.Error()
		}
		out := portalHomepageCookieFile()
		return "success", "GFX homepage cookies saved locally → " + out
	case KindCredFetch:
		if _, err := ensureGFXLogin(ctx, session, ""); err != nil {
			return "failed", "gfx login failed: " + err.Error()
		}
		if err := runCredScraper(ctx, session, tool); err != nil {
			return "failed", err.Error()
		}
		return "success", "scraped credentials for " + tool.ScrapeName
	case KindOneClick:
		return "failed", "one-click GFX tools not implemented yet (planned: " + tool.Name + ")"
	default:
		return "failed", "unsupported gfx kind"
	}
}
