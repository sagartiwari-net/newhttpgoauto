package seoshope

import (
	"github.com/go-rod/rod"

	"gohttpauto/internal/automation/screenshots"
)

func errorScreenshotDir() string {
	return screenshots.Dir("seoshope")
}

func saveErrorScreenshot(page *rod.Page, step string) {
	screenshots.SaveError(page, "SEOShope", "seoshope", "", step)
}
