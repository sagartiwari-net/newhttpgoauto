package screenshots

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

const maxErrorScreenshots = 5

var lastSavedPath string

var safeName = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// Root returns the Desktop screenshot folder on Mac worker (override with GOAUTO_SCREENSHOT_PATH).
func Root() string {
	if p := os.Getenv("GOAUTO_SCREENSHOT_PATH"); p != "" {
		return p
	}
	if runtime.GOOS == "darwin" {
		if home := os.Getenv("HOME"); home != "" {
			return filepath.Join(home, "Desktop", "screenshot")
		}
	}
	if d := os.Getenv("GOHTTPAUTO_DATA"); d != "" {
		return filepath.Join(d, "screenshots")
	}
	return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "gohttpauto", "screenshots")
}

// EnsureDirs creates Desktop/screenshot and common subfolders on worker startup.
func EnsureDirs() string {
	root := Root()
	for _, sub := range []string{"gfx", "seoshope"} {
		_ = os.MkdirAll(filepath.Join(root, sub), 0755)
	}
	return root
}

// Dir builds ~/Desktop/screenshot/<tool>/[<group>/].
func Dir(tool string, group ...string) string {
	tool = sanitize(tool, "misc")
	parts := []string{Root(), tool}
	for _, g := range group {
		g = sanitize(g, "")
		if g != "" {
			parts = append(parts, g)
		}
	}
	return filepath.Join(parts...)
}

// SaveError captures the page on failure; keeps latest N per folder.
func SaveError(page *rod.Page, logTag, tool, group, step string) {
	savePage(page, logTag, tool, group, step, maxErrorScreenshots)
}

// SaveCapture saves a screenshot whenever session/cookies are saved (success path).
func SaveCapture(page *rod.Page, logTag, tool, group, step string) {
	savePage(page, logTag, tool, group, "capture_"+step, maxCaptureScreenshots)
}

const maxCaptureScreenshots = 10

// LastSavedPath returns the path from the most recent SaveError/SaveCapture call.
func LastSavedPath() string { return lastSavedPath }

func savePage(page *rod.Page, logTag, tool, group, step string, keep int) {
	if page == nil {
		return
	}
	img, err := page.Timeout(15 * time.Second).Screenshot(true, nil)
	if err != nil {
		log.Printf("[%s] Screenshot failed (%s/%s): %v", logTag, tool, step, err)
		return
	}

	dir := Dir(tool, group)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("[%s] Screenshot dir error: %v", logTag, err)
		return
	}

	step = sanitize(step, "error")
	name := fmt.Sprintf("%d_%s.png", time.Now().Unix(), step)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, img, 0644); err != nil {
		log.Printf("[%s] Screenshot write error: %v", logTag, err)
		return
	}
	lastSavedPath = path
	prune(dir, keep)
	log.Printf("[%s] Screenshot → %s", logTag, path)
}

func sanitize(s, fallback string) string {
	s = strings.Trim(safeName.ReplaceAllString(s, "_"), "_")
	if s == "" {
		return fallback
	}
	return s
}

type shotFile struct {
	path    string
	modTime time.Time
}

func prune(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var files []shotFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".png") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, shotFile{path: filepath.Join(dir, e.Name()), modTime: info.ModTime()})
	}
	if len(files) <= keep {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modTime.Before(files[j].modTime) })
	for i := 0; i < len(files)-keep; i++ {
		_ = os.Remove(files[i].path)
	}
}
