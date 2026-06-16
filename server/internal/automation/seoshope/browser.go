package seoshope

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

// Session is one Chrome process tied to the shared SEOShope profile.
type Session struct {
	browser *rod.Browser
	page    *rod.Page
	logged  bool
	cancel  context.CancelFunc
}

func newSession(ctx context.Context) (*Session, error) {
	pDir := profileDir()
	sDir := screenshotDir()
	_ = os.MkdirAll(pDir, 0755)
	_ = os.MkdirAll(sDir, 0755)
	for _, lf := range []string{"SingletonLock", "SingletonCookie", "SingletonSocket"} {
		_ = os.Remove(filepath.Join(pDir, lf))
	}

	headless := runtime.GOOS != "darwin" && os.Getenv("DISPLAY") == ""
	log.Printf("[SEOShope] Launching Chrome (headless=%v, profile=%s)", headless, pDir)

	l := launcher.New().
		Headless(headless).
		Set("no-sandbox").
		Set("disable-setuid-sandbox").
		Set("disable-dev-shm-usage").
		Set("disable-blink-features", "AutomationControlled").
		Set("mute-audio").
		Set("disable-popup-blocking").
		Set("window-size", "1920,1080").
		UserDataDir(pDir)

	u, err := l.Launch()
	if err != nil {
		return nil, err
	}

	sessCtx, cancel := context.WithCancel(ctx)
	browser := rod.New().ControlURL(u).MustConnect().Context(sessCtx)

	page := stealth.MustPage(browser)
	page.MustSetViewport(1920, 1080, 1, false)
	page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	})
	attachTurnstileHijack(page)

	return &Session{browser: browser, page: page, cancel: cancel}, nil
}

func (s *Session) Close() {
	if s == nil {
		return
	}
	log.Println("[SEOShope] Closing Chrome (task finished)")
	if s.browser != nil {
		_ = s.browser.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.browser = nil
	s.page = nil
	s.logged = false
}

func (s *Session) Browser() *rod.Browser { return s.browser }
func (s *Session) Page() *rod.Page       { return s.EnsurePortalPage() }
func (s *Session) MarkLoggedIn()         { s.logged = true }
func (s *Session) LoggedIn() bool        { return s.logged }

// EnsurePortalPage returns a live tab for portal work. After Semrush capture the
// stored page target may be closed — recover from browser or open a new tab.
func (s *Session) EnsurePortalPage() *rod.Page {
	if s.browser == nil {
		return s.page
	}
	if s.page != nil {
		if _, err := s.page.Info(); err == nil {
			return s.page
		}
		log.Println("[SEOShope] Portal page target stale — recovering tab")
	}

	pages, err := s.browser.Pages()
	if err == nil {
		var fallback *rod.Page
		for _, p := range pages {
			info, infoErr := p.Info()
			if infoErr != nil {
				continue
			}
			u := strings.ToLower(info.URL)
			if strings.Contains(u, "seoshope.com") {
				s.page = p
				log.Printf("[SEOShope] Recovered portal tab: %s", info.URL)
				return p
			}
			if fallback == nil {
				fallback = p
			}
		}
		if fallback != nil {
			s.page = fallback
			return fallback
		}
	}

	log.Println("[SEOShope] Opening new portal tab")
	s.page = stealth.MustPage(s.browser)
	s.page.MustSetViewport(1920, 1080, 1, false)
	s.page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	})
	attachTurnstileHijack(s.page)
	return s.page
}

// HasMemberCookies checks member session cookies on any open browser tab.
func (s *Session) HasMemberCookies() bool {
	if s.browser == nil {
		return false
	}
	pages, err := s.browser.Pages()
	if err != nil {
		return false
	}
	for _, p := range pages {
		if hasMemberSessionCookie(p) {
			return true
		}
	}
	return false
}

// CloseExtraTabs closes Semrush proxy tabs and keeps one live seoshope.com portal tab.
func (s *Session) CloseExtraTabs() {
	if s.browser == nil {
		return
	}
	pages, err := s.browser.Pages()
	if err != nil {
		s.EnsurePortalPage()
		return
	}

	var portal *rod.Page
	for _, p := range pages {
		info, infoErr := p.Info()
		if infoErr != nil {
			_ = p.Close()
			continue
		}
		u := strings.ToLower(info.URL)
		if strings.Contains(u, "seoshope.com") {
			if portal == nil {
				portal = p
			} else {
				_ = p.Close()
			}
			continue
		}
		_ = p.Close()
	}

	if portal != nil {
		s.page = portal
		return
	}
	s.EnsurePortalPage()
}

func (s *Session) WaitSettle() {
	time.Sleep(1 * time.Second)
}
