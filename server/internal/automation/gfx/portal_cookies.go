package gfx

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func portalSeedCookieFile(accountID string) string {
	return filepath.Join(cookiesBackupDir(), fmt.Sprintf("gfx_%s_portal_seed.json", accountID))
}

// exportPortalSeedCookies saves gfxtoolz.ai cookies from the main automation profile for portal reuse.
func exportPortalSeedCookies(page *rod.Page, accountID string) {
	raw, err := cookiesForDomain(page, "gfxtoolz.ai")
	if err != nil || len(raw) == 0 {
		return
	}
	b, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return
	}
	path := portalSeedCookieFile(accountID)
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, b, 0644); err != nil {
		log.Printf("[gfx] portal seed write failed: %v", err)
		return
	}
	log.Printf("[gfx] Portal seed cookies saved (%d) → %s", len(raw), path)
}

func loadPortalSeedCookies(accountID string) ([]CookieJSON, error) {
	path := portalSeedCookieFile(accountID)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("portal seed file missing (%s) — run any GFX tool task first", path)
	}
	var cookies []CookieJSON
	if err := json.Unmarshal(b, &cookies); err != nil {
		return nil, fmt.Errorf("portal seed file invalid: %w", err)
	}
	if len(cookies) == 0 {
		return nil, fmt.Errorf("portal seed file empty")
	}
	return cookies, nil
}

func injectCookiesIntoPage(page *rod.Page, cookies []CookieJSON) error {
	for _, c := range cookies {
		if strings.TrimSpace(c.Name) == "" {
			continue
		}
		domain := c.Domain
		if domain == "" {
			domain = ".gfxtoolz.ai"
		}
		path := c.Path
		if path == "" {
			path = "/"
		}
		sameSite := proto.NetworkCookieSameSiteLax
		switch strings.ToLower(c.SameSite) {
		case "strict":
			sameSite = proto.NetworkCookieSameSiteStrict
		case "none", "no_restriction":
			sameSite = proto.NetworkCookieSameSiteNone
		}
		expires := proto.TimeSinceEpoch(c.ExpirationDate)
		if c.Session || c.ExpirationDate <= 0 {
			expires = -1
		}
		err := page.SetCookies([]*proto.NetworkCookieParam{{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   domain,
			Path:     path,
			Secure:   c.Secure,
			HTTPOnly: c.HttpOnly,
			SameSite: sameSite,
			Expires:  expires,
		}})
		if err != nil {
			log.Printf("[gfx_portal] cookie inject warning (%s): %v", c.Name, err)
		}
	}
	return nil
}
