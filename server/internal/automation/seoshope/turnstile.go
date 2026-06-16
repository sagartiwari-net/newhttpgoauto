package seoshope

import (
	"log"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// attachTurnstileHijack is kept as alias for setupPageNetwork.
func attachTurnstileHijack(page *rod.Page) {
	setupPageNetwork(page)
}

const turnstileMockJS = `
(function() {
	console.log("Mock Turnstile Loader Active");
	window.turnstile = {
		render: function(el, options) {
			const callback = (options && options.callback) || (el && el.getAttribute && el.getAttribute('data-callback'));
			setTimeout(() => {
				const token = "mock_turnstile_success_token_1234567890";
				const input1 = document.querySelector('input[name="cf-turnstile-response"]');
				const input2 = document.getElementById("cf-turnstile-response");
				if (input1) input1.value = token;
				if (input2) input2.value = token;
				if (typeof callback === "function") {
					callback(token);
				} else if (typeof callback === "string" && typeof window[callback] === "function") {
					window[callback](token);
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
		if (btn) { btn.disabled = false; btn.removeAttribute('disabled'); btn.click(); return true; }
		const loginInput = document.querySelector('input[name="amember_login"]');
		if (loginInput && loginInput.form) { loginInput.form.submit(); return true; }
		return false;
	}`)
}

func primeTurnstile(page *rod.Page) {
	_, _ = page.Eval(`() => {
		if (!window.turnstile || !window.turnstile.render) return false;
		document.querySelectorAll('.cf-turnstile').forEach(el => {
			el.setAttribute('data-rendered', 'true');
			window.turnstile.render(el);
		});
		return true;
	}`)
}

func turnstileToken(page *rod.Page) string {
	res, err := page.Eval(`() => {
		const inp = document.querySelector('input[name="cf-turnstile-response"]');
		return inp ? inp.value : "";
	}`)
	if err != nil {
		return ""
	}
	return res.Value.Str()
}

func waitTurnstile(page *rod.Page) bool {
	for i := 0; i < 60; i++ {
		if token := turnstileToken(page); token != "" {
			log.Printf("[SEOShope] Turnstile token ready (len=%d)", len(token))
			return true
		}
		if i == 0 || i == 10 || i == 20 || i == 30 {
			primeTurnstile(page)
		}
		if i == 20 {
			log.Println("[SEOShope] Turnstile not auto-solved — trying checkbox click")
			tryClickTurnstile(page)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return turnstileToken(page) != ""
}

func tryClickTurnstile(page *rod.Page) {
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
			time.Sleep(300 * time.Millisecond)
			_ = page.Mouse.Click(proto.InputMouseButtonLeft, 1)
		}
		return
	}
	cfFrame, err := iframeEl.Frame()
	if err != nil {
		return
	}
	for _, sel := range []string{".ctp-checkbox-container", "input[type='checkbox']", ".mark", "[role='checkbox']"} {
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
