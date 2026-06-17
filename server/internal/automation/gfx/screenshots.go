package gfx

import (
	"github.com/go-rod/rod"

	"gohttpauto/internal/automation/screenshots"
)

func saveErrorScreenshot(page *rod.Page, group, step string) {
	screenshots.SaveError(page, "GFX", "gfx", group, step)
}
