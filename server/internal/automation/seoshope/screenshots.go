package seoshope

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

const maxErrorScreenshots = 3

var safeStepName = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// errorScreenshotDir is screenshots/<tool>/errors/
func errorScreenshotDir() string {
	return filepath.Join(dataRoot(), "screenshots", profileName, "errors")
}

// saveErrorScreenshot captures the page on failure and keeps only the latest N per tool.
func saveErrorScreenshot(page *rod.Page, step string) {
	if page == nil {
		return
	}
	img, err := page.Screenshot(true, nil)
	if err != nil {
		log.Printf("[SEOShope] Screenshot failed (%s): %v", step, err)
		return
	}

	dir := errorScreenshotDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("[SEOShope] Screenshot dir error: %v", err)
		return
	}

	step = strings.Trim(safeStepName.ReplaceAllString(step, "_"), "_")
	if step == "" {
		step = "error"
	}
	name := fmt.Sprintf("%d_%s.png", time.Now().Unix(), step)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, img, 0644); err != nil {
		log.Printf("[SEOShope] Screenshot write error: %v", err)
		return
	}
	pruneErrorScreenshots(dir)
	log.Printf("[SEOShope] Error screenshot: %s", path)
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
		log.Printf("[SEOShope] Pruned old screenshot: %s", files[i].path)
	}
}
