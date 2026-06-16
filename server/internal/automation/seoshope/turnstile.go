package seoshope

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func attachTurnstileHijack(page *rod.Page) {
	router := page.HijackRequests()
	router.MustAdd("*challenges.cloudflare.com/turnstile/v0/api.js*", func(ctx *rod.Hijack) {
		log.Println("[SEOShope] Intercepted Turnstile API — serving mock")
		ctx.Response.SetBody(turnstileMockJS)
		ctx.Response.Headers().Set("Content-Type", "application/javascript")
	})
	router.MustAdd("*challenges.cloudflare.com*", func(ctx *rod.Hijack) {
		ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
	})
	go router.Run()
}

const turnstileMockJS = `
(function() {
	window.turnstile = {
		render: function(el, options) {
			const callback = (options && options.callback) || (el && el.getAttribute && el.getAttribute('data-callback'));
			setTimeout(() => {
				const input1 = document.querySelector('input[name="cf-turnstile-response"]');
				const input2 = document.getElementById("cf-turnstile-response");
				if (input1) input1.value = "mock_turnstile_success_token_1234567890";
				if (input2) input2.value = "mock_turnstile_success_token_1234567890";
				if (typeof callback === "function") {
					callback("mock_turnstile_success_token_1234567890");
				} else if (typeof callback === "string" && typeof window[callback] === "function") {
					window[callback]("mock_turnstile_success_token_1234567890");
				}
			}, 500);
			return "mock-widget-id";
		},
		reset: function() {},
		getResponse: function() { return "mock_turnstile_success_token_1234567890"; },
		execute: function() {}
	};
	const scanAndRender = () => {
		document.querySelectorAll('.cf-turnstile').forEach(el => {
			if (!el.getAttribute('data-rendered')) {
				el.setAttribute('data-rendered', 'true');
				window.turnstile.render(el);
			}
		});
	};
	if (document.readyState === "loading") {
		document.addEventListener("DOMContentLoaded", scanAndRender);
	} else {
		scanAndRender();
	}
})();`

func fillLoginForm(page *rod.Page, username, password string) {
	_, _ = page.Eval(`(u, p) => {
		const loginEl = document.querySelector('input[name="amember_login"]')
			|| document.querySelector('#amember-login')
			|| document.querySelector('input[type="text"]')
			|| document.querySelector('input[type="email"]');
		const passEl = document.querySelector('input[name="amember_pass"]')
			|| document.querySelector('#amember-pass')
			|| document.querySelector('input[type="password"]');
		if (!loginEl || !passEl) return false;
		loginEl.value = u;
		passEl.value = p;
		['input', 'change'].forEach(ev => {
			loginEl.dispatchEvent(new Event(ev, { bubbles: true }));
			passEl.dispatchEvent(new Event(ev, { bubbles: true }));
		});
		return true;
	}`, username, password)
}

func submitLoginForm(page *rod.Page) {
	_, _ = page.Eval(`() => {
		const btn = document.querySelector('#login-submit-button')
			|| document.querySelector('.frm-submit')
			|| document.querySelector('button[type="submit"]')
			|| document.querySelector('input[type="submit"]');
		if (btn) { btn.disabled = false; btn.click(); return true; }
		const loginInput = document.querySelector('input[name="amember_login"]');
		if (loginInput && loginInput.form) { loginInput.form.submit(); return true; }
		return false;
	}`)
}

func waitTurnstile(page *rod.Page, shots string) bool {
	for i := 0; i < 60; i++ {
		res, err := page.Eval(`() => {
			const inp = document.querySelector('input[name="cf-turnstile-response"]');
			return inp ? inp.value : "";
		}`)
		if err == nil && res.Value.Str() != "" {
			return true
		}
		if i == 20 {
			tryClickTurnstile(page, shots)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

func tryClickTurnstile(page *rod.Page, shots string) {
	container, err := page.Element(".cf-turnstile")
	if err != nil {
		return
	}
	iframeEl, err := container.Element("iframe")
	if err != nil {
		res, evalErr := page.Eval(`() => {
			const el = document.querySelector('.cf-turnstile');
			if (!el) return null;
			const r = el.getBoundingClientRect();
			return { x: r.left + 20, y: r.top + r.height/2 };
		}`)
		if evalErr == nil && !res.Value.Nil() {
			var pt struct{ X, Y float64 }
			_ = res.Value.Unmarshal(&pt)
			_ = page.Mouse.MoveTo(proto.NewPoint(pt.X, pt.Y))
			time.Sleep(200 * time.Millisecond)
			_ = page.Mouse.Click(proto.InputMouseButtonLeft, 1)
		}
		return
	}
	cfFrame, err := iframeEl.Frame()
	if err != nil {
		return
	}
	for _, sel := range []string{".ctp-checkbox-container", "input[type='checkbox']", "[role='checkbox']"} {
		el, elErr := cfFrame.Element(sel)
		if elErr != nil {
			continue
		}
		if visible, _ := el.Visible(); visible {
			_ = el.Click(proto.InputMouseButtonLeft, 1)
			return
		}
	}
}

func solveCloudflare(page *rod.Page) {
	iframes, _ := page.Elements("iframe")
	for _, iframe := range iframes {
		src, _ := iframe.Attribute("src")
		if src != nil && strings.Contains(*src, "cloudflare") {
			cfFrame := iframe.MustFrame()
			res, err := cfFrame.Eval(`() => {
				const el = document.querySelector('.ctp-checkbox-container') || document.querySelector('input[type="checkbox"]');
				if (!el) return null;
				const r = el.getBoundingClientRect();
				return { x: r.x + r.width/2, y: r.y + r.height/2 };
			}`)
			if err == nil && !res.Value.Nil() {
				var pt struct{ X, Y float64 }
				_ = res.Value.Unmarshal(&pt)
				mouse := cfFrame.Mouse
				mouse.MustMoveTo(pt.X, pt.Y)
				mouse.MustClick(proto.InputMouseButtonLeft)
				time.Sleep(3 * time.Second)
			}
			return
		}
	}
}

func takeScreenshot(page *rod.Page, label, dir string) {
	_ = os.MkdirAll(dir, 0755)
	fp := filepath.Join(dir, fmt.Sprintf("%s_latest.png", label))
	if img, err := page.Screenshot(true, nil); err == nil {
		_ = os.WriteFile(fp, img, 0644)
	}
}
