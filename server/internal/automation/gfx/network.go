package gfx

import (
	"log"
	"os"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// attachGFXNetworkFilter blocks images, fonts, media, and analytics to speed page loads.
// Set GFX_BLOCK_IMAGES=0 to disable.
func attachGFXNetworkFilter(page *rod.Page) {
	if os.Getenv("GFX_BLOCK_IMAGES") == "0" {
		return
	}

	router := page.HijackRequests()

	for _, pattern := range gfxBlockedURLPatterns {
		p := pattern
		router.MustAdd(p, func(ctx *rod.Hijack) {
			ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
		})
	}

	router.MustAdd("*", func(ctx *rod.Hijack) {
		url := ctx.Request.URL().String()
		if gfxShouldBlockRequest(ctx.Request.Type(), url) {
			ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
			return
		}
		ctx.ContinueRequest(&proto.FetchContinueRequest{})
	})

	go router.Run()
}

var gfxBlockedURLPatterns = []string{
	"*/cdn-cgi/rum*",
	"*google-analytics.com*",
	"*googletagmanager.com*",
	"*doubleclick.net*",
	"*facebook.net*",
	"*facebook.com/tr*",
	"*hotjar.com*",
	"*sentry.io*",
	"*segment.io*",
	"*mixpanel.com*",
	"*fonts.googleapis.com*",
	"*fonts.gstatic.com*",
}

func gfxShouldBlockRequest(rtype proto.NetworkResourceType, url string) bool {
	u := strings.ToLower(url)

	if strings.HasPrefix(u, "chrome-extension://") {
		return false
	}
	if gfxIsEssentialHost(u) {
		return false
	}

	switch rtype {
	case proto.NetworkResourceTypeImage,
		proto.NetworkResourceTypeFont,
		proto.NetworkResourceTypeMedia,
		proto.NetworkResourceTypePing,
		proto.NetworkResourceTypeEventSource,
		proto.NetworkResourceTypeWebSocket,
		proto.NetworkResourceTypeManifest:
		return true
	}

	for _, frag := range []string{
		".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".avif",
		".woff", ".woff2", ".ttf", ".otf", ".eot",
		".mp4", ".webm", ".mp3",
	} {
		if strings.Contains(u, frag) {
			return true
		}
	}
	return false
}

func gfxIsEssentialHost(url string) bool {
	return strings.Contains(url, "gfxtoolz.ai") ||
		strings.Contains(url, "airbrush.ai") ||
		strings.Contains(url, "challenges.cloudflare.com") ||
		strings.Contains(url, "cloudflare.com")
}

func logGFXNetworkFilterOnce() {
	if os.Getenv("GFX_BLOCK_IMAGES") == "0" {
		return
	}
	log.Println("[GFX] Network filter on (blocked: images, fonts, media, analytics — CSS allowed)")
}
