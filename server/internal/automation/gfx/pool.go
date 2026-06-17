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

// Account is one enabled GFX portal credential from the shared pool.
type Account struct {
	WebsiteID string
	Username  string
	Password  string
}

// Slot binds a task to one pool account and its Chrome profile directory.
type Slot struct {
	Account    Account
	ProfileDir string
}

const poolGroupDefault = "gfxtoolz"

var profileLocks sync.Map // websiteID -> *sync.Mutex

// ProfileLock returns the per-account mutex (one Chrome per profile).
func ProfileLock(websiteID string) *sync.Mutex {
	v, _ := profileLocks.LoadOrStore(websiteID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// LoadPoolAccounts returns all enabled GFX portal accounts (extension + cred-fetch share this pool).
func LoadPoolAccounts(ctx context.Context) ([]Account, error) {
	rows, err := db.DB.QueryContext(ctx, `
		SELECT website_id, username, password_enc
		FROM credentials
		WHERE is_enabled = 1
		  AND (
		    pool_group = ?
		    OR (pool_group IS NULL AND website_id LIKE 'gfxtoolz%')
		  )
		ORDER BY website_id`, poolGroupDefault)
	if err != nil {
		return loadAccountsLegacy(ctx)
	}
	defer rows.Close()

	var list []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.WebsiteID, &a.Username, &a.Password); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func loadAccountsLegacy(ctx context.Context) ([]Account, error) {
	rows, err := db.DB.QueryContext(ctx, `
		SELECT website_id, username, password_enc
		FROM credentials
		WHERE is_enabled = 1 AND website_id LIKE 'gfxtoolz%'
		ORDER BY website_id`)
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
		list = append(list, a)
	}
	return list, rows.Err()
}

// ResolveSlot picks an account for any GFX task (extension, cred-fetch, future one-click)
// using stable hash partitioning across the shared pool.
func ResolveSlot(ctx context.Context, taskUID string) (Slot, error) {
	accounts, err := LoadPoolAccounts(ctx)
	if err != nil {
		return Slot{}, err
	}
	if len(accounts) == 0 {
		return Slot{}, fmt.Errorf("no enabled GFX accounts (add gfxtoolz_1, gfxtoolz_2, … in credentials)")
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

// ResolvePortalSlot picks pool credentials but uses an isolated Chrome profile for portal capture.
func ResolvePortalSlot(ctx context.Context, taskUID string) (Slot, error) {
	accounts, err := LoadPoolAccounts(ctx)
	if err != nil {
		return Slot{}, err
	}
	if len(accounts) == 0 {
		return Slot{}, fmt.Errorf("no enabled GFX accounts (add gfxtoolz_1, gfxtoolz_2, … in credentials)")
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(taskUID))
	idx := int(h.Sum32()) % len(accounts)
	acc := accounts[idx]
	return Slot{
		Account:    acc,
		ProfileDir: profileDirForPortal(acc.WebsiteID),
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
