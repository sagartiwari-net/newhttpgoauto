package gfx

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

var safeStepName = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func screenshotRoot() string {
	if p := os.Getenv("GOAUTO_SCREENSHOT_PATH"); p != "" {
		return p
	}
	if runtime.GOOS == "darwin" {
		if home := os.Getenv("HOME"); home != "" {
			return filepath.Join(home, "Desktop", "screenshot")
		}
	}
	return filepath.Join(dataRoot(), "screenshots")
}

func errorScreenshotDir(group string) string {
	group = strings.Trim(safeStepName.ReplaceAllString(group, "_"), "_")
	if group == "" {
		group = "gfx"
	}
	return filepath.Join(screenshotRoot(), "gfx", group)
}

func saveErrorScreenshot(page *rod.Page, group, step string) {
	if page == nil {
		return
	}
	img, err := page.Timeout(15 * time.Second).Screenshot(true, nil)
	if err != nil {
		log.Printf("[GFX] Screenshot failed (%s/%s): %v", group, step, err)
		return
	}

	dir := errorScreenshotDir(group)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("[GFX] Screenshot dir error: %v", err)
		return
	}

	step = strings.Trim(safeStepName.ReplaceAllString(step, "_"), "_")
	if step == "" {
		step = "error"
	}
	name := fmt.Sprintf("%d_%s.png", time.Now().Unix(), step)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, img, 0644); err != nil {
		log.Printf("[GFX] Screenshot write error: %v", err)
		return
	}
	pruneErrorScreenshots(dir)
	log.Printf("[GFX] Error screenshot → %s", path)
}

type shotFile struct {
	path    string
	modTime time.Time
}

func pruneErrorScreenshots(dir string) {
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
		files = append(files, shotFile{
			path:    filepath.Join(dir, e.Name()),
			modTime: info.ModTime(),
		})
	}
	if len(files) <= maxErrorScreenshots {
		return
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})
	for i := 0; i < len(files)-maxErrorScreenshots; i++ {
		_ = os.Remove(files[i].path)
	}
}
