package gfx

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

type loginAPIResult struct {
	mu     sync.Mutex
	Status int
	Body   string
	Seen   bool
}

func (r *loginAPIResult) set(status int, body string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Status = status
	r.Body = body
	r.Seen = true
}

func (r *loginAPIResult) snapshot() (status int, body string, seen bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.Status, r.Body, r.Seen
}

// watchGFXLoginAPI records the GFX auth/login API response (status + body).
// stop() cancels the listener; it must not block (EachEvent wait() blocks forever unless the callback returns true).
func watchGFXLoginAPI(page *rod.Page) (stop func(), result *loginAPIResult) {
	result = &loginAPIResult{}
	watchPage, cancel := page.WithCancel()
	_ = proto.NetworkEnable{}.Call(watchPage)

	go func() {
		watchPage.EachEvent(func(e *proto.NetworkResponseReceived) {
			if !strings.Contains(e.Response.URL, "/api/v1/auth/login") {
				return
			}
			status := e.Response.Status
			reqID := e.RequestID
			go func() {
				body := ""
				res, err := proto.NetworkGetResponseBody{RequestID: reqID}.Call(watchPage)
				if err == nil {
					body = res.Body
				}
				result.set(status, body)
			}()
		})()
	}()

	return cancel, result
}

func readPageLoginError(page *rod.Page) string {
	res, err := page.Eval(`() => {
		const pick = (sel) => {
			const el = document.querySelector(sel);
			return el && el.textContent ? el.textContent.trim() : '';
		};
		for (const sel of ['[role="alert"]', '[data-testid="error"]', '.text-destructive', '.text-red-500', '.error-message']) {
			const t = pick(sel);
			if (t) return t;
		}
		const body = (document.body && document.body.innerText) ? document.body.innerText.toLowerCase() : '';
		const keys = [
			'account blocked', 'account suspended', 'blocked', 'suspended',
			'invalid email', 'invalid password', 'incorrect password', 'wrong password',
			'device limit', 'too many devices', 'sign in again',
			'unauthorized', 'access denied', 'banned'
		];
		for (const k of keys) {
			if (body.includes(k)) return k;
		}
		return '';
	}`)
	if err != nil || res.Value.Nil() {
		return ""
	}
	return strings.TrimSpace(res.Value.Str())
}

func formatGFXLoginFailure(accountID string, api *loginAPIResult, pageErr string) string {
	status, body, seen := api.snapshot()
	bodyLower := strings.ToLower(body + " " + pageErr)

	switch {
	case strings.Contains(bodyLower, "block") || strings.Contains(bodyLower, "banned") || strings.Contains(bodyLower, "suspend"):
		return "account blocked or suspended (check gfxtoolz portal for " + accountID + ")"
	case strings.Contains(bodyLower, "device") && (strings.Contains(bodyLower, "limit") || strings.Contains(bodyLower, "many")):
		return "device limit reached — too many active GFX sessions on " + accountID
	case status == 401 || strings.Contains(bodyLower, "invalid") || strings.Contains(bodyLower, "incorrect") || strings.Contains(bodyLower, "wrong password"):
		return "invalid credentials — wrong email or password for " + accountID
	case status == 403:
		return "login forbidden (403) — account may be blocked: " + accountID
	case status == 429:
		return "rate limited (429) — too many login attempts on " + accountID
	case seen && status >= 400:
		msg := extractAPIMessage(body)
		if msg != "" {
			return "login API HTTP " + strconv.Itoa(status) + ": " + msg
		}
		return "login API HTTP " + strconv.Itoa(status) + " for " + accountID
	case pageErr != "":
		return "login failed: " + pageErr
	default:
		return "sign-in failed — still on login page (account " + accountID + ")"
	}
}

func extractAPIMessage(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		if len(body) > 200 {
			return body[:200]
		}
		return body
	}
	for _, key := range []string{"message", "error", "detail", "msg"} {
		if v, ok := parsed[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}
