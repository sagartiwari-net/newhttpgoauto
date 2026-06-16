package seoshope

import (
	"log"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// setupPageNetwork installs Turnstile hijack + blocks heavy resources (images, fonts, analytics).
// Call before any navigation on a page.
func setupPageNetwork(page *rod.Page) {
	router := page.HijackRequests()

	router.MustAdd("*challenges.cloudflare.com/turnstile/v0/api.js*", func(ctx *rod.Hijack) {
		ctx.Response.SetBody(turnstileMockJS)
		ctx.Response.Headers().Set("Content-Type", "application/javascript")
	})
	router.MustAdd("*challenges.cloudflare.com*", func(ctx *rod.Hijack) {
		ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
	})
	router.MustAdd("*cdn-cgi/challenge-platform/*", func(ctx *rod.Hijack) {
		ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
	})

	for _, pattern := range blockedURLPatterns {
		p := pattern
		router.MustAdd(p, func(ctx *rod.Hijack) {
			ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
		})
	}

	router.MustAdd("*", func(ctx *rod.Hijack) {
		url := ctx.Request.URL().String()
		if shouldBlockRequest(ctx.Request.Type(), url) {
			ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
			return
		}
		ctx.ContinueRequest(&proto.FetchContinueRequest{})
	})

	go router.Run()
	log.Println("[SEOShope] Network filter on (blocked: images, fonts, media, analytics, rum)")
}

var blockedURLPatterns = []string{
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
	"*seoshope.com/data/public/content/*",
	"*gravatar.com*",
}

func shouldBlockRequest(rtype proto.NetworkResourceType, url string) bool {
	u := strings.ToLower(url)
	if strings.Contains(u, "turnstile/v0/api.js") {
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

	if rtype == proto.NetworkResourceTypeStylesheet && !isEssentialHost(u) {
		return true
	}

	for _, frag := range []string{
		".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico",
		".woff", ".woff2", ".ttf", ".otf", ".eot",
		".mp4", ".webm", ".mp3",
	} {
		if strings.Contains(u, frag) {
			return true
		}
	}
	return false
}

func isEssentialHost(url string) bool {
	return strings.Contains(url, "seoshope.com") ||
		strings.Contains(url, "challenges.cloudflare.com")
}
