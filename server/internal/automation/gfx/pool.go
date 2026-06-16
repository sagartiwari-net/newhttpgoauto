package gfx

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"sync"

	"gohttpauto/internal/db"
)

// Account is one enabled GFX portal credential from the pool.
type Account struct {
	WebsiteID string
	Username  string
	Password  string
	Role      string // default | scraper
}

// Slot binds a task to one pool account and its Chrome profile directory.
type Slot struct {
	Account    Account
	ProfileDir string
}

const (
	poolGroupDefault = "gfxtoolz"
	roleDefault      = "default"
	roleScraper      = "scraper"
)

var profileLocks sync.Map // websiteID -> *sync.Mutex

// ProfileLock returns the per-account mutex (one Chrome per profile).
func ProfileLock(websiteID string) *sync.Mutex {
	v, _ := profileLocks.LoadOrStore(websiteID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// LoadAccounts returns enabled GFX credentials for the given role.
func LoadAccounts(ctx context.Context, role string) ([]Account, error) {
	rows, err := db.DB.QueryContext(ctx, `
		SELECT website_id, username, password_enc,
		       COALESCE(pool_role, CASE WHEN website_id = 'gfxtoolz_scraper' THEN 'scraper' ELSE 'default' END)
		FROM credentials
		WHERE is_enabled = 1
		  AND (
		    pool_group = ?
		    OR (pool_group IS NULL AND website_id LIKE 'gfxtoolz%')
		  )
		  AND COALESCE(pool_role, CASE WHEN website_id = 'gfxtoolz_scraper' THEN 'scraper' ELSE 'default' END) = ?
		ORDER BY website_id`, poolGroupDefault, role)
	if err != nil {
		// Fallback when migration not applied yet.
		return loadAccountsLegacy(ctx, role)
	}
	defer rows.Close()

	var list []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.WebsiteID, &a.Username, &a.Password, &a.Role); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func loadAccountsLegacy(ctx context.Context, role string) ([]Account, error) {
	q := `
		SELECT website_id, username, password_enc
		FROM credentials
		WHERE is_enabled = 1 AND website_id LIKE 'gfxtoolz%'
		ORDER BY website_id`
	rows, err := db.DB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.WebsiteID, &a.Username, &a.Password); err != nil {
			return nil, err
		}
		if role == roleScraper {
			if a.WebsiteID == "gfxtoolz_scraper" || a.WebsiteID == "gfxtoolz_2" {
				a.Role = roleScraper
				list = append(list, a)
			}
			continue
		}
		if a.WebsiteID == "gfxtoolz_scraper" {
			continue
		}
		a.Role = roleDefault
		list = append(list, a)
	}
	return list, rows.Err()
}

// ResolveSlot picks an account for a task using stable hash partitioning.
func ResolveSlot(ctx context.Context, taskUID string, kind Kind) (Slot, error) {
	role := roleDefault
	if kind == KindCredFetch {
		role = roleScraper
	}
	accounts, err := LoadAccounts(ctx, role)
	if err != nil {
		return Slot{}, err
	}
	if len(accounts) == 0 {
		if kind == KindCredFetch {
			return Slot{}, fmt.Errorf("no enabled GFX scraper account (set pool_role=scraper or website_id=gfxtoolz_scraper)")
		}
		return Slot{}, fmt.Errorf("no enabled GFX accounts in pool")
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(taskUID))
	idx := int(h.Sum32()) % len(accounts)
	acc := accounts[idx]
	return Slot{
		Account:    acc,
		ProfileDir: profileDirForAccount(acc.WebsiteID),
	}, nil
}

// CheckProfileMeta wipes profile if bound username changed in DB.
func CheckProfileMeta(slot Slot) error {
	metaPath := profileMetaPath(slot.ProfileDir)
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return writeProfileMeta(slot)
	}
	var stored struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		return writeProfileMeta(slot)
	}
	if stored.Username != "" && stored.Username != slot.Account.Username {
		_ = os.RemoveAll(slot.ProfileDir)
		_ = os.MkdirAll(slot.ProfileDir, 0755)
	}
	return writeProfileMeta(slot)
}

func writeProfileMeta(slot Slot) error {
	_ = os.MkdirAll(slot.ProfileDir, 0755)
	b, _ := json.Marshal(map[string]string{"username": slot.Account.Username})
	return os.WriteFile(profileMetaPath(slot.ProfileDir), b, 0644)
}
