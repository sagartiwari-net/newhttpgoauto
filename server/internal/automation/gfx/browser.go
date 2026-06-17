package gfx

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

// Session is one Chrome + GFX extension run for a pool account profile.
type Session struct {
	slot    Slot
	browser *rod.Browser
	cancel  context.CancelFunc
}

func newSession(ctx context.Context, slot Slot) (*Session, error) {
	if err := CheckProfileMeta(slot); err != nil {
		log.Printf("[GFX] profile meta warning (%s): %v", slot.Account.WebsiteID, err)
	}
	_ = os.MkdirAll(slot.ProfileDir, 0755)
	for _, lf := range []string{"SingletonLock", "SingletonCookie", "SingletonSocket"} {
		_ = os.Remove(filepath.Join(slot.ProfileDir, lf))
	}

	extPath := extensionDir()
	// Headless by default; GFX_VISIBLE=1 shows Chrome for manual debugging on worker Mac.
	headless := os.Getenv("GFX_VISIBLE") != "1"
	log.Printf("[GFX] Launching Chrome account=%s headless=%v profile=%s ext=%s",
		slot.Account.WebsiteID, headless, slot.ProfileDir, extPath)

	l := launcher.New().
		Headless(headless).
		Set("no-sandbox").
		Set("disable-setuid-sandbox").
		Set("disable-dev-shm-usage").
		Set("disable-gpu").
		Set("disable-popup-blocking").
		Set("disable-features", "IsolateOrigins,site-per-process").
		Set("disable-blink-features", "AutomationControlled").
		Set("disable-extensions-except", extPath).
		Set("load-extension", extPath).
		UserDataDir(slot.ProfileDir)
	if headless {
		l = l.Set("blink-settings", "imagesEnabled=false")
	}

	u, err := l.Launch()
	if err != nil {
		return nil, err
	}

	sessCtx, cancel := context.WithCancel(ctx)
	browser := rod.New().ControlURL(u).MustConnect().Context(sessCtx)
	logGFXNetworkFilterOnce()
	return &Session{slot: slot, browser: browser, cancel: cancel}, nil
}

func (s *Session) Close() {
	if s == nil {
		return
	}
	if os.Getenv("GFX_KEEP_OPEN") == "1" {
		browser := s.browser
		s.browser = nil
		log.Printf("[GFX] Keeping browser open 12s for inspection (account=%s) — task can finish now", s.slot.Account.WebsiteID)
		go func() {
			time.Sleep(12 * time.Second)
			if browser != nil {
				_ = browser.Close()
			}
		}()
	} else {
		log.Printf("[GFX] Closing Chrome (account=%s)", s.slot.Account.WebsiteID)
		if s.browser != nil {
			_ = s.browser.Close()
		}
		s.browser = nil
	}
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *Session) Browser() *rod.Browser { return s.browser }
func (s *Session) Slot() Slot            { return s.slot }

func (s *Session) newPage() *rod.Page {
	page := stealth.MustPage(s.browser)
	attachGFXNetworkFilter(page)
	page.MustSetViewport(1920, 1080, 1, false)
	page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	})
	attachPageDiagnostics(page, "gfx_"+s.slot.Account.WebsiteID)
	if pages, err := s.browser.Pages(); err == nil {
		for _, p := range pages {
			if p.TargetID != page.TargetID {
				_ = p.Close()
			}
		}
	}
	return page
}
