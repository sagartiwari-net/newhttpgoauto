package gfx

import (
	"context"
	"fmt"
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
	// Headless by default for speed; set GFX_VISIBLE=1 on the worker to show Chrome for debugging.
	headless := os.Getenv("GFX_VISIBLE") != "1"
	log.Printf("[GFX] Launching Chrome account=%s headless=%v profile=%s ext=%s",
		slot.Account.WebsiteID, headless, slot.ProfileDir, extPath)

	l := launcher.New().
		Headless(headless).
		Set("no-sandbox").
		Set("disable-setuid-sandbox").
		Set("disable-dev-shm-usage").
		Set("disable-gpu").
		Set("blink-settings", "imagesEnabled=false").
		Set("disable-popup-blocking").
		Set("disable-features", "IsolateOrigins,site-per-process").
		Set("disable-blink-features", "AutomationControlled").
		Set("disable-extensions-except", extPath).
		Set("load-extension", extPath).
		UserDataDir(slot.ProfileDir)

	u, err := l.Launch()
	if err != nil {
		return nil, err
	}

	sessCtx, cancel := context.WithCancel(ctx)
	browser := rod.New().ControlURL(u).MustConnect().Context(sessCtx)
	logGFXNetworkFilterOnce()
	if err := requireExtensionLoaded(browser, extPath, slot.Account.WebsiteID); err != nil {
		cancel()
		_ = browser.Close()
		return nil, err
	}
	return &Session{slot: slot, browser: browser, cancel: cancel}, nil
}

func requireExtensionLoaded(browser *rod.Browser, extPath, accountID string) error {
	page := stealth.MustPage(browser)
	defer func() { _ = page.Close() }()
	// Content scripts do not run on about:blank — probe on GFX sign-in instead.
	log.Printf("[GFX] Verifying extension on %s (account=%s)", gfxSigninURL, accountID)
	if err := page.Timeout(30 * time.Second).Navigate(gfxSigninURL); err != nil {
		log.Printf("[GFX] Extension probe navigation warning: %v", err)
	}
	time.Sleep(2 * time.Second)
	if waitExtensionInject(page, 20) {
		log.Printf("[GFX] Extension loaded OK (account=%s)", accountID)
		return nil
	}
	return fmt.Errorf("GFX extension failed to load from %s (no inject on %s)", extPath, gfxSigninURL)
}

func (s *Session) Close() {
	if s == nil {
		return
	}
	if os.Getenv("GFX_KEEP_OPEN") == "1" {
		log.Printf("[GFX] Keeping browser open 45s for inspection (account=%s)", s.slot.Account.WebsiteID)
		time.Sleep(45 * time.Second)
	}
	s.closeBrowser()
}

func (s *Session) closeBrowser() {
	log.Printf("[GFX] Closing Chrome (account=%s)", s.slot.Account.WebsiteID)
	if s.browser != nil {
		_ = s.browser.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.browser = nil
}

// Relaunch closes Chrome and starts a fresh instance with the same profile dir.
// Call after credential login so the GFX extension loads with persisted session cookies.
func (s *Session) Relaunch(ctx context.Context) error {
	accountID := s.slot.Account.WebsiteID
	log.Printf("[GFX] Relaunching Chrome after login (account=%s)", accountID)
	s.closeBrowser()
	time.Sleep(1500 * time.Millisecond)

	for _, lf := range []string{"SingletonLock", "SingletonCookie", "SingletonSocket"} {
		_ = os.Remove(filepath.Join(s.slot.ProfileDir, lf))
	}

	extPath := extensionDir()
	headless := os.Getenv("GFX_VISIBLE") != "1"
	l := launcher.New().
		Headless(headless).
		Set("no-sandbox").
		Set("disable-setuid-sandbox").
		Set("disable-dev-shm-usage").
		Set("disable-gpu").
		Set("blink-settings", "imagesEnabled=false").
		Set("disable-popup-blocking").
		Set("disable-features", "IsolateOrigins,site-per-process").
		Set("disable-blink-features", "AutomationControlled").
		Set("disable-extensions-except", extPath).
		Set("load-extension", extPath).
		UserDataDir(s.slot.ProfileDir)

	u, err := l.Launch()
	if err != nil {
		return err
	}
	sessCtx, cancel := context.WithCancel(ctx)
	s.browser = rod.New().ControlURL(u).MustConnect().Context(sessCtx)
	s.cancel = cancel
	if err := requireExtensionLoaded(s.browser, extPath, accountID); err != nil {
		cancel()
		s.closeBrowser()
		return err
	}
	log.Printf("[GFX] Chrome relaunched (account=%s)", accountID)
	return nil
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
