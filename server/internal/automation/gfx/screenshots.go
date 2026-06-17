package gfx

import (
	"github.com/go-rod/rod"

	"gohttpauto/internal/automation/screenshots"
)

func saveErrorScreenshot(page *rod.Page, group, step string) {
	screenshots.SaveError(page, "GFX", "gfx", group, step)
}

func saveCaptureScreenshot(page *rod.Page, group, step string) string {
	screenshots.SaveCapture(page, "GFX", "gfx", group, step)
	return screenshots.LastSavedPath()
}
