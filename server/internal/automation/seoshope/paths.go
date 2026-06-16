package seoshope

import (
	"os"
	"path/filepath"
	"runtime"
)

const profileName = "seoshope"

func dataRoot() string {
	if d := os.Getenv("GOHTTPAUTO_DATA"); d != "" {
		return d
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "gohttpauto")
	}
	return "/www/wwwroot/gohttpauto"
}

func profileDir() string {
	return filepath.Join(dataRoot(), "profiles", profileName)
}

func screenshotDir() string {
	return errorScreenshotDir()
}

func loginCookieFile() string {
	return filepath.Join(dataRoot(), "cookies", "seoshope_login_cookies.json")
}
