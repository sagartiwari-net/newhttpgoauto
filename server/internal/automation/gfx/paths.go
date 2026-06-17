package gfx

import (
	"os"
	"path/filepath"
	"runtime"
)

// extensionDir returns the bundled GFX Chrome extension (manifest v3).
func extensionDir() string {
	if d := os.Getenv("GFX_EXT_PATH"); d != "" {
		return d
	}
	_, file, _, ok := runtime.Caller(0)
	if ok {
		return filepath.Join(filepath.Dir(file), "ext")
	}
	return filepath.Join(dataRoot(), "gfx-ext")
}

func dataRoot() string {
	if d := os.Getenv("GOHTTPAUTO_DATA"); d != "" {
		return d
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "gohttpauto")
	}
	return "/www/wwwroot/gohttpauto"
}

func profileDirForAccount(websiteID string) string {
	return filepath.Join(dataRoot(), "profiles", "gfx", websiteID)
}

// profileDirForPortal is a separate Chrome profile (does not touch automation profiles).
func profileDirForPortal(websiteID string) string {
	return filepath.Join(dataRoot(), "profiles", "gfx-portal", websiteID)
}

func portalHomepageCookieFile() string {
	return filepath.Join(cookiesBackupDir(), "gfx_portal_homepage.json")
}

func cookiesBackupDir() string {
	return filepath.Join(dataRoot(), "cookies")
}

func profileMetaPath(profileDir string) string {
	return filepath.Join(profileDir, ".meta.json")
}
