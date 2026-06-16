package seoshope

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gohttpauto/internal/cookiesession"
	"gohttpauto/internal/db"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

type cookieJSON struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	Size     int     `json:"size"`
	HttpOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
	SameSite string  `json:"sameSite,omitempty"`
}

func runSemrushSlot(ctx context.Context, s *Session, slot Slot) (string, string) {
	var username, password string
	err := db.DB.QueryRowContext(ctx,
		`SELECT username, password_enc FROM credentials WHERE website_id='seoshope' AND is_enabled=1`).
		Scan(&username, &password)
	if err != nil {
		return "failed", "seoshope credentials not found"
	}
	if err := ensureLoggedIn(ctx, s, username, password); err != nil {
		return "failed", err.Error()
	}

	s.CloseExtraTabs()
	page := s.EnsurePortalPage()

	log.Printf("[SEOShope] Navigating to /page/sem for access %s", slot.ButtonNum)
	if err := page.Timeout(30*time.Second).Navigate(semPageURL); err != nil {
		saveErrorScreenshot(page, "sem_page_nav_failed")
		return "failed", "sem page navigation failed: " + err.Error()
	}
	time.Sleep(2 * time.Second)

	if isLoginPage(page) {
		log.Println("[SEOShope] /page/sem shows login — clearing stale session and re-logging in")
		saveErrorScreenshot(page, "sem_page_not_logged_in")
		s.logged = false
		clearPortalSession(page)
		if err := ensureLoggedIn(ctx, s, username, password); err != nil {
			return "failed", "login retry after /page/sem redirect: " + err.Error()
		}
		page = s.EnsurePortalPage()
		if err := page.Timeout(30 * time.Second).Navigate(semPageURL); err != nil {
			saveErrorScreenshot(page, "sem_page_nav_retry_failed")
			return "failed", "sem page navigation failed after login: " + err.Error()
		}
		time.Sleep(2 * time.Second)
		if isLoginPage(page) {
			saveErrorScreenshot(page, "sem_page_still_login")
			return "failed", "still not logged in on /page/sem after fresh login"
		}
	}

	btn, err := findAccessButton(page, slot)
	if err != nil {
		saveErrorScreenshot(page, "no_semrush_btn")
		if isLoginPage(page) {
			return "failed", "semrush buttons missing: still on login page (session invalid)"
		}
		return "failed", err.Error()
	}

	var targetID proto.TargetTargetID
	wait := s.Browser().EachEvent(func(e *proto.TargetTargetCreated) bool {
		if e.TargetInfo.Type == proto.TargetTargetInfoTypePage {
			targetID = e.TargetInfo.TargetID
			return true
		}
		return false
	})

	btn.MustWaitVisible()
	if err := btn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		if !clickAccessButtonJS(page, slot) {
			saveErrorScreenshot(page, "access_btn_click_failed")
			return "failed", "access button click failed: " + err.Error()
		}
	}
	wait()

	newPage := s.Browser().MustPageFromTargetID(targetID)
	setupPageNetwork(newPage)
	defer func() {
		_ = newPage.Close()
		s.CloseExtraTabs()
	}()

	var targetDomain string
	for w := 0; w < 20; w++ {
		time.Sleep(2 * time.Second)
		currentURL := newPage.MustInfo().URL
		log.Printf("[SEOShope] redirect %d/20: %s", w+1, currentURL)
		if strings.Contains(currentURL, "ban.php") {
			saveErrorScreenshot(newPage, "access_blocked")
			return "failed", "access blocked by provider"
		}
		parsed, _ := url.Parse(currentURL)
		if parsed != nil && parsed.Host != "" {
			host := strings.ToLower(parsed.Host)
			if !strings.Contains(host, "app.seoshope.com") && !strings.Contains(host, "seoshope.com") {
				targetDomain = host
				break
			}
		}
	}
	log.Printf("[SEOShope] target domain: %s", targetDomain)
	time.Sleep(2 * time.Second)

	raw, err := newPage.Cookies([]string{})
	if err != nil {
		return "failed", "cookie read failed: " + err.Error()
	}

	filtered := filterSemrushCookies(raw)
	if len(filtered) == 0 {
		saveErrorScreenshot(newPage, "empty_cookies")
		return "failed", "no semrush cookies after redirect"
	}

	jsonStr := cookiesToJSON(filtered)
	netscapeStr := cookiesToNetscape(filtered)
	headerStr := cookiesToHeader(filtered)

	_, err = db.DB.ExecContext(ctx, `
		INSERT INTO shared_sessions (website_id, cookies_json, cookies_netscape, cookies_header, updated_at)
		VALUES (?, ?, ?, ?, NOW())
		ON DUPLICATE KEY UPDATE
			cookies_json = VALUES(cookies_json),
			cookies_netscape = VALUES(cookies_netscape),
			cookies_header = VALUES(cookies_header),
			updated_at = NOW()`,
		slot.WebsiteID, jsonStr, netscapeStr, headerStr)
	if err != nil {
		return "failed", "db save failed: " + err.Error()
	}

	backup := filepath.Join(dataRoot(), "cookies", slot.WebsiteID+"_cookies.json")
	osWriteFile(backup, jsonStr)
	go uploadCookies(slot.UploadURL, jsonStr)

	return "success", fmt.Sprintf("semrush access %s updated (%d cookies)", slot.ButtonNum, len(filtered))
}

func findAccessButton(page *rod.Page, slot Slot) (*rod.Element, error) {
	for i := 0; i < 10; i++ {
		elements, err := page.Elements("button.semmy-btn")
		if err == nil {
			for _, el := range elements {
				numEl, errNum := el.Element("span.semmy-btn-num")
				if errNum == nil && numEl != nil {
					text, _ := numEl.Text()
					if strings.TrimSpace(text) == slot.ButtonNum {
						return el, nil
					}
				}
				text, _ := el.Text()
				if strings.Contains(strings.ToLower(text), slot.AccessKey) {
					return el, nil
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil, fmt.Errorf("semrush access %s button not found", slot.ButtonNum)
}

func clickAccessButtonJS(page *rod.Page, slot Slot) bool {
	res, err := page.Eval(`(num, key) => {
		const btns = Array.from(document.querySelectorAll('button.semmy-btn'));
		const btn = btns.find(b => {
			const numEl = b.querySelector('.semmy-btn-num');
			return (numEl && numEl.textContent.trim() === num) || b.textContent.toLowerCase().includes(key);
		});
		if (btn) { btn.click(); return true; }
		return false;
	}`, slot.ButtonNum, slot.AccessKey)
	return err == nil && res != nil && res.Value.Bool()
}

func filterSemrushCookies(raw []*proto.NetworkCookie) []cookieJSON {
	var out []cookieJSON
	for _, c := range raw {
		if strings.Contains(strings.ToLower(c.Domain), "seoshope.com") {
			continue
		}
		out = append(out, cookieJSON{
			Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path,
			Expires: float64(c.Expires), Size: c.Size,
			HttpOnly: c.HTTPOnly, Secure: c.Secure, SameSite: string(c.SameSite),
		})
	}
	return out
}

func cookiesToJSON(cookies []cookieJSON) string {
	b, _ := json.MarshalIndent(cookies, "", "  ")
	return string(b)
}

func cookiesToNetscape(cookies []cookieJSON) string {
	return cookiesession.BuildNetscape(toSessionCookies(cookies), "Generated by GoHttpAuto SEOShope")
}

func cookiesToHeader(cookies []cookieJSON) string {
	return cookiesession.BuildHeader(toSessionCookies(cookies))
}

func toSessionCookies(cookies []cookieJSON) []cookiesession.Cookie {
	out := make([]cookiesession.Cookie, 0, len(cookies))
	for _, c := range cookies {
		ss := c.SameSite
		out = append(out, cookiesession.Cookie{
			Domain: c.Domain, Name: c.Name, Value: c.Value, Path: c.Path,
			ExpirationDate: c.Expires, HTTPOnly: c.HttpOnly, Secure: c.Secure, SameSite: &ss,
		})
	}
	return out
}

func uploadCookies(uploadURL, jsonStr string) {
	data := url.Values{}
	data.Set("cookies", jsonStr)
	data.Set("update", "Update Cookies")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.PostForm(uploadURL, data)
	if err != nil {
		log.Printf("[SEOShope] upload failed %s: %v", uploadURL, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		log.Printf("[SEOShope] upload ok: %s", uploadURL)
	} else {
		log.Printf("[SEOShope] upload HTTP %d: %s", resp.StatusCode, uploadURL)
	}
}

func osWriteFile(path, content string) {
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	_ = os.WriteFile(path, []byte(content), 0644)
}
