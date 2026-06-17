package gfx

import (
	"context"
	"log"
	"os"
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

	ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
	defer cancel()

	slot, err := ResolveSlot(ctx, taskUID)
	if err != nil {
		return "failed", err.Error()
	}

	mu := ProfileLock(slot.Account.WebsiteID)
	mu.Lock()
	defer mu.Unlock()

	log.Printf("[GFX] Task %s → account %s profile %s", taskUID, slot.Account.WebsiteID, slot.ProfileDir)
	log.Printf("[GFX] Error screenshots → %s", screenshotRoot()+"/gfx/")
	if os.Getenv("GFX_VISIBLE") == "1" {
		log.Printf("[GFX] Chrome visible (GFX_VISIBLE=1) — watch Mac screen during task")
	}

	session, err := newSession(ctx, slot)
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
	case KindCredFetch:
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
