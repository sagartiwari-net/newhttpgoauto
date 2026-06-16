package markhor

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
	refsUploadURL      = "https://refs.1clkaccess.store/markhor.php"
	portalCookieDomain = ".markhorseotool.com"
	portalSessionID    = "markhorseotool_login"
	sessionWebsiteID   = "markhorseotool"
)

var (
	semrushLinkRE = regexp.MustCompile(`(?i)href=["']([^"']*sm01\.markhorseotool\.com[^"']*)["']`)
	csrfTokenRE   = regexp.MustCompile(`(?i)name=["']csrf_token["'][^>]*value=["']([^"']+)["']`)
)

// refsPortalCookie matches JSON format expected by markhor.php.
type refsPortalCookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	Size     int     `json:"size"`
	HttpOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
	SameSite string  `json:"sameSite"`
}

func RunSemrush() (string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var username, password string
	err := db.DB.QueryRowContext(ctx,
		`SELECT username, password_enc FROM credentials WHERE website_id='markhorseotool' AND is_enabled=1`).
		Scan(&username, &password)
	if err != nil {
		return "failed", "markhorseotool credentials not found in database"
	}

	client := httpclient.New(60 * time.Second)
	memberURL := "https://markhorseotool.com/member/"
	loginURL := "https://markhorseotool.com/login"
	portalBase := "https://markhorseotool.com/"

	if saved, err := cookiesession.Load(ctx, portalSessionID); err == nil && len(saved) > 0 {
		_ = client.SetCookies(portalBase, portalSessionToHTTP(saved))
		log.Printf("[Markhor] restored %d portal cookies", len(saved))
	}

	body, status, pageURL, err := client.GET(memberURL, nil)
	if err != nil || status != 200 {
		return "failed", fmt.Sprintf("member page error: %v (status %d)", err, status)
	}

	if !isLoggedIn(body, pageURL) {
		loginBody, _, _, err := client.GET(loginURL, map[string]string{"Referer": memberURL})
		if err != nil {
			return "failed", "markhor login page error: " + err.Error()
		}
		csrfToken, err := parseCSRFToken(loginBody)
		if err != nil {
			return "failed", "markhor csrf_token missing: " + err.Error()
		}

		form := url.Values{}
		form.Set("csrf_token", csrfToken)
		form.Set("username", username)
		form.Set("password", password)

		finalURL, postBody, postStatus, err := client.POST(loginURL, form.Encode(), map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
			"Origin":       "https://markhorseotool.com",
			"Referer":      loginURL,
		})
		if err != nil {
			return "failed", "markhor login POST error: " + err.Error()
		}
		if !isLoggedIn(postBody, finalURL) {
			reason := loginFailureReason(finalURL, postBody, postStatus)
			log.Printf("[Markhor] login failed: %s (url=%s status=%d)", reason, finalURL, postStatus)
			return "failed", "markhor login failed: " + reason
		}
		log.Printf("[Markhor] login OK → %s", finalURL)
	}

	dashBody, _, dashURL, err := client.GET(memberURL, map[string]string{
		"Referer": "https://markhorseotool.com/",
	})
	if err != nil {
		return "failed", "dashboard load error: " + err.Error()
	}
	if !isLoggedIn(dashBody, dashURL) {
		reason := loginFailureReason(dashURL, dashBody, 200)
		if reason == "" {
			reason = "session invalid after login"
		}
		return "failed", "markhor session invalid: " + reason
	}

	portalCookies, err := collectPortalCookies(client)
	if err != nil {
		return "failed", "portal cookie capture error: " + err.Error()
	}
	if !hasRequiredPortalCookies(portalCookies) {
		return "failed", "missing mht_session or PHPSESSID after login"
	}

	if err := savePortalSession(ctx, portalCookies, portalSessionID, portalBase); err != nil {
		log.Printf("⚠️ [Markhor] portal cookie save failed: %v", err)
	}

	sessionCookies := portalCookiesToSession(portalCookies)
	err = cookiesession.Save(ctx, cookiesession.SaveOptions{
		WebsiteID: sessionWebsiteID,
		Referer:   portalBase,
		Cookies:   sessionCookies,
	})
	if err != nil {
		return "failed", "db save error: " + err.Error()
	}

	go uploadPortalCookies(portalCookies)

	btn := findSemrushLink(dashBody)
	if btn == "" {
		btn = "https://sm01.markhorseotool.com/analytics/overview/"
	}
	if strings.HasPrefix(btn, "/") {
		btn = "https://markhorseotool.com" + btn
	}
	log.Printf("[Markhor] semrush link: %s", btn)

	finalSemURL, _, _, err := client.GET(btn, map[string]string{"Referer": memberURL})
	if err != nil {
		return "success", fmt.Sprintf("portal cookies updated (%d) — semrush verify failed: %v", len(portalCookies), err)
	}
	if strings.Contains(strings.ToLower(finalSemURL), "access-denied") {
		log.Printf("⚠️ [Markhor] semrush access-denied at %s", finalSemURL)
	}

	return "success", fmt.Sprintf("portal cookies updated (%d) + semrush access verified", len(portalCookies))
}

func isLoggedIn(body, pageURL string) bool {
	if httpclient.IsCloudflareChallenge(body) {
		return false
	}
	lowerURL := strings.ToLower(pageURL)
	if strings.Contains(lowerURL, "/login") {
		return false
	}
	lower := strings.ToLower(body)
	if strings.Contains(lower, `name="username"`) && strings.Contains(lower, `name="password"`) {
		return false
	}
	return strings.Contains(lower, "logout") || strings.Contains(lowerURL, "/member")
}

func loginFailureReason(finalURL, body string, httpStatus int) string {
	if httpclient.IsCloudflareChallenge(body) {
		return "Cloudflare challenge blocked HTTP login from worker IP"
	}
	if msg := httpclient.ParseLoginError(body); msg != "" {
		return msg
	}
	lower := strings.ToLower(body)
	for _, phrase := range []string{
		"invalid request",
		"invalid credentials",
		"incorrect username or password",
		"authentication failed",
		"account has been locked",
		"too many login attempts",
	} {
		if strings.Contains(lower, phrase) {
			return phrase
		}
	}
	if httpStatus == 403 {
		return "HTTP 403 forbidden — possible IP or account block"
	}
	if strings.Contains(strings.ToLower(finalURL), "/login") {
		return "still on login page after POST"
	}
	return "login verification failed"
}

func findSemrushLink(html string) string {
	if m := semrushLinkRE.FindStringSubmatch(html); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func parseCSRFToken(html string) (string, error) {
	if m := csrfTokenRE.FindStringSubmatch(html); len(m) > 1 {
		if token := strings.TrimSpace(m[1]); token != "" {
			return token, nil
		}
	}
	return "", fmt.Errorf("csrf_token not found in login page")
}

func collectPortalCookies(client *httpclient.Client) ([]refsPortalCookie, error) {
	byName := map[string]*http.Cookie{}
	for _, c := range client.Captured {
		if c != nil && isPortalCookie(c.Name) {
			byName[c.Name] = c
		}
	}
	raw, err := client.CookiesFor("https://markhorseotool.com/")
	if err != nil {
		return nil, err
	}
	for _, c := range raw {
		if c != nil && isPortalCookie(c.Name) {
			byName[c.Name] = c
		}
	}

	var out []refsPortalCookie
	for _, c := range byName {
		path := c.Path
		if path == "" {
			path = "/"
		}
		expires := float64(time.Now().Add(365 * 24 * time.Hour).Unix())
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
			SameSite: "Lax",
		})
	}
	return out, nil
}

func isPortalCookie(name string) bool {
	switch strings.ToLower(name) {
	case "phpsessid", "mht_session":
		return true
	default:
		return false
	}
}

func hasRequiredPortalCookies(cookies []refsPortalCookie) bool {
	hasSession := false
	hasPHP := false
	for _, c := range cookies {
		switch strings.ToLower(c.Name) {
		case "mht_session":
			hasSession = c.Value != ""
		case "phpsessid":
			hasPHP = c.Value != ""
		}
	}
	return hasSession && hasPHP
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

func portalCookiesToSession(cookies []refsPortalCookie) []cookiesession.Cookie {
	out := make([]cookiesession.Cookie, 0, len(cookies))
	for _, c := range cookies {
		exp := time.Now().Add(365 * 24 * time.Hour)
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
		log.Printf("⚠️ [Markhor] markhor.php JSON marshal failed: %v", err)
		return
	}
	data := url.Values{}
	data.Set("cookies", string(payload))
	data.Set("update", "Update Cookies")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.PostForm(refsUploadURL, data)
	if err != nil {
		log.Printf("⚠️ [Markhor] markhor.php upload failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		log.Printf("✅ [Markhor] portal cookies synced to %s", refsUploadURL)
	} else {
		log.Printf("⚠️ [Markhor] %s returned HTTP %d", refsUploadURL, resp.StatusCode)
	}
}
