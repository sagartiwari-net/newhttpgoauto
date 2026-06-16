package portalchrome

import (
	"os"
	"path/filepath"
	"runtime"
)

func dataRoot() string {
	if d := os.Getenv("GOHTTPAUTO_DATA"); d != "" {
		return d
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "gohttpauto")
	}
	return "/www/wwwroot/gohttpauto"
}

func profileDir(name string) string {
	return filepath.Join(dataRoot(), "profiles", name)
}

func loginCookieFile(name string) string {
	return filepath.Join(dataRoot(), "cookies", name+"_login_cookies.json")
}
