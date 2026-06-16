package toolbaazar

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"gohttpauto/internal/automation/httpclient"
	"gohttpauto/internal/cookiesession"
	"gohttpauto/internal/db"
)

const (
	refsUploadURL    = "https://refs.1clkaccess.store/tbsm1.php"
	portalCookieDomain = ".toolbaazar.com"
)

var semrushLinkRE = regexp.MustCompile(`(?i)href=["']([^"']*(?:semrush4\.toolbaazar\.com|toolbaazar\.com)[^"']*)["']`)

// refsPortalCookie matches the JSON format expected by tbsm1.php.
type refsPortalCookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	Size     int     `json:"size"`
	HttpOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
}

func RunSemrush() (string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var username, password string
	err := db.DB.QueryRowContext(ctx,
		`SELECT username, password_enc FROM credentials WHERE website_id='toolbaazar' AND is_enabled=1`).
		Scan(&username, &password)
	if err != nil {
		return "failed", "toolbaazar credentials not found in database"
	}

	client := httpclient.New(60 * time.Second)
	memberURL := "https://app.toolbaazar.com/member"
	loginURL := "https://app.toolbaazar.com/login"
	portalSessionID := "toolbaazar_login"
	portalBase := "https://app.toolbaazar.com/"

	if saved, err := cookiesession.Load(ctx, portalSessionID); err == nil && len(saved) > 0 {
		_ = client.SetCookies(portalBase, portalSessionToHTTP(saved))
		log.Printf("[Toolbaazar] restored %d portal cookies", len(saved))
	}

	body, status, _, err := client.GET(memberURL, nil)
	if err != nil || status != 200 {
		return "failed", fmt.Sprintf("member page error: %v (status %d)", err, status)
	}
	needsLogin := strings.Contains(strings.ToLower(body), `name="amember_login"`) ||
		strings.Contains(strings.ToLower(body), `id="amember-login"`) ||
		strings.Contains(strings.ToLower(body), `type="password"`)

	if needsLogin {
		form := url.Values{}
		form.Set("amember_login", username)
		form.Set("amember_pass", password)
		form.Set("remember_login", "1")

		finalURL, postBody, _, err := client.POST(loginURL, form.Encode(), map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
			"Origin":       "https://app.toolbaazar.com",
			"Referer":      memberURL,
		})
		if err != nil || !httpclient.LoginOK(finalURL, postBody) {
			return "failed", "toolbaazar login failed"
		}
		log.Printf("[Toolbaazar] login OK → %s", finalURL)
	}

	dashBody, _, _, err := client.GET(memberURL, map[string]string{
		"Referer": "https://app.toolbaazar.com/",
	})
	if err != nil {
		return "failed", "dashboard load error: " + err.Error()
	}
	if strings.Contains(strings.ToLower(dashBody), `name="amember_login"`) {
		return "failed", "toolbaazar session invalid after login"
	}

	portalCookies, err := collectPortalCookies(client)
	if err != nil {
		return "failed", "portal cookie capture error: " + err.Error()
	}
	if len(portalCookies) == 0 {
		return "failed", "no aMember portal cookies captured (PHPSESSID/amember_nr)"
	}
	if !hasRequiredPortalCookies(portalCookies) {
		return "failed", "missing PHPSESSID or amember_nr after login"
	}

	if err := savePortalSession(ctx, portalCookies, portalSessionID, portalBase); err != nil {
		log.Printf("⚠️ [Toolbaazar] portal cookie save failed: %v", err)
	}

	sessionCookies := portalCookiesToSession(portalCookies)
	err = cookiesession.Save(ctx, cookiesession.SaveOptions{
		WebsiteID: "toolbaazar",
		Referer:   portalBase,
		Cookies:   sessionCookies,
	})
	if err != nil {
		return "failed", "db save error: " + err.Error()
	}

	go uploadPortalCookies(portalCookies)

	btn := findSemrushLink(dashBody)
	if btn == "" {
		return "success", fmt.Sprintf("portal cookies updated (%d) — semrush link not found on dashboard", len(portalCookies))
	}
	if strings.HasPrefix(btn, "/") {
		btn = "https://app.toolbaazar.com" + btn
	}
	log.Printf("[Toolbaazar] semrush link: %s", btn)

	finalSemURL, _, _, err := client.GET(btn, map[string]string{"Referer": memberURL})
	if err != nil {
		return "success", fmt.Sprintf("portal cookies updated (%d) — semrush verify failed: %v", len(portalCookies), err)
	}
	if !strings.Contains(strings.ToLower(finalSemURL), "semrush4.toolbaazar.com") {
		log.Printf("⚠️ [Toolbaazar] final URL: %s", finalSemURL)
	}

	return "success", fmt.Sprintf("portal cookies updated (%d) + semrush access verified", len(portalCookies))
}

func findSemrushLink(html string) string {
	if m := semrushLinkRE.FindStringSubmatch(html); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func collectPortalCookies(client *httpclient.Client) ([]refsPortalCookie, error) {
	// Prefer response Set-Cookie values (latest login), then jar state.
	byName := map[string]*http.Cookie{}
	for _, c := range client.Captured {
		if c == nil || !isPortalCookie(strings.ToLower(c.Name)) {
			continue
		}
		byName[c.Name] = c
	}
	raw, err := client.CookiesFor("https://app.toolbaazar.com/")
	if err != nil {
		return nil, err
	}
	for _, c := range raw {
		if c == nil || !isPortalCookie(strings.ToLower(c.Name)) {
			continue
		}
		byName[c.Name] = c // jar state wins (latest session after login)
	}

	var out []refsPortalCookie
	for _, c := range byName {
		path := c.Path
		if path == "" {
			path = "/"
		}
		expires := float64(-1)
		if !c.Expires.IsZero() && c.Expires.Unix() > 0 {
			expires = float64(c.Expires.Unix())
		}
		out = append(out, refsPortalCookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   portalCookieDomain,
			Path:     path,
			Expires:  expires,
			Size:     len(c.Name) + len(c.Value),
			HttpOnly: true,
			Secure:   true,
		})
	}
	return out, nil
}

func hasRequiredPortalCookies(cookies []refsPortalCookie) bool {
	hasSession := false
	hasMember := false
	for _, c := range cookies {
		switch strings.ToLower(c.Name) {
		case "phpsessid":
			hasSession = c.Value != ""
		default:
			if strings.HasPrefix(strings.ToLower(c.Name), "amember") && c.Value != "" {
				hasMember = true
			}
		}
	}
	return hasSession && hasMember
}

func portalSessionToHTTP(saved []cookiesession.Cookie) []*http.Cookie {
	out := make([]*http.Cookie, 0, len(saved))
	for _, c := range saved {
		if strings.TrimSpace(c.Name) == "" {
			continue
		}
		path := c.Path
		if path == "" {
			path = "/"
		}
		h := &http.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   portalCookieDomain,
			Path:     path,
			Secure:   true,
			HttpOnly: true,
		}
		if c.ExpirationDate > 0 {
			h.Expires = time.Unix(int64(c.ExpirationDate), 0)
		}
		out = append(out, h)
	}
	return out
}

func isPortalCookie(name string) bool {
	return name == "phpsessid" ||
		strings.HasPrefix(name, "amember")
}

func portalCookiesToSession(cookies []refsPortalCookie) []cookiesession.Cookie {
	out := make([]cookiesession.Cookie, 0, len(cookies))
	for _, c := range cookies {
		exp := time.Now().Add(24 * time.Hour)
		if c.Expires > 0 {
			exp = time.Unix(int64(c.Expires), 0)
		}
		out = append(out, cookiesession.SimpleCookie(c.Domain, c.Name, c.Value, c.Secure, c.HttpOnly, exp))
	}
	return out
}

func savePortalSession(ctx context.Context, cookies []refsPortalCookie, websiteID, referer string) error {
	return cookiesession.Save(ctx, cookiesession.SaveOptions{
		WebsiteID: websiteID,
		Referer:   referer,
		Cookies:   portalCookiesToSession(cookies),
	})
}

func uploadPortalCookies(cookies []refsPortalCookie) {
	payload, err := json.Marshal(cookies)
	if err != nil {
		log.Printf("⚠️ [Toolbaazar] tbsm1 JSON marshal failed: %v", err)
		return
	}
	data := url.Values{}
	data.Set("cookies", string(payload))
	data.Set("update", "Update Cookies")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.PostForm(refsUploadURL, data)
	if err != nil {
		log.Printf("⚠️ [Toolbaazar] tbsm1 upload failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		log.Printf("✅ [Toolbaazar] portal cookies synced to %s", refsUploadURL)
	} else {
		log.Printf("⚠️ [Toolbaazar] %s returned HTTP %d", refsUploadURL, resp.StatusCode)
	}
}
