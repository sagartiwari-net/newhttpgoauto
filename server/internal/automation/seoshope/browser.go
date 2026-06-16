package seoshope

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"runtime"
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
		UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	})
	attachTurnstileHijack(page)

	return &Session{browser: browser, page: page, cancel: cancel}, nil
}

func (s *Session) Close() {
	if s == nil {
		return
	}
	log.Println("[SEOShope] Closing Chrome (no more queued tasks for this profile)")
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
func (s *Session) Page() *rod.Page       { return s.page }
func (s *Session) MarkLoggedIn()         { s.logged = true }
func (s *Session) LoggedIn() bool        { return s.logged }

func (s *Session) CloseExtraTabs() {
	if s.browser == nil || s.page == nil {
		return
	}
	mainID := s.page.TargetID
	pages, err := s.browser.Pages()
	if err != nil {
		return
	}
	for _, p := range pages {
		if p.TargetID != mainID {
			_ = p.Close()
		}
	}
}

func (s *Session) WaitSettle() {
	time.Sleep(1 * time.Second)
}
