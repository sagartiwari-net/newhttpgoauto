package httpclient

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36"

var (
	LoginAttemptRE = regexp.MustCompile(`(?i)name=["']login_attempt_id["'][^>]*value=["']([^"']+)["']`)
	AmErrorsRE     = regexp.MustCompile(`(?is)class=["']am-errors["'][^>]*>(.*?)</`)
)

type capturedCookies struct{ list []*http.Cookie }

func (cc *capturedCookies) record(resp *http.Response) {
	for _, c := range resp.Cookies() {
		cp := *c
		if cp.Domain == "" {
			cp.Domain = resp.Request.URL.Host
		}
		cc.list = append(cc.list, &cp)
	}
}

type loggingTransport struct{ captured *capturedCookies }

func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err == nil {
		t.captured.record(resp)
	}
	return resp, err
}

// Client wraps an HTTP client with cookie jar and response cookie capture.
type Client struct {
	HTTP     *http.Client
	Captured []*http.Cookie
}

func New(timeout time.Duration) *Client {
	jar, _ := cookiejar.New(nil)
	cc := &capturedCookies{}
	return &Client{
		HTTP: &http.Client{
			Timeout: timeout,
			Jar:     jar,
			Transport: &loggingTransport{captured: cc},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 12 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		Captured: cc.list,
	}
}

func (c *Client) refreshCaptured() {
	// loggingTransport appends to internal slice; expose via type assertion hack
	if tr, ok := c.HTTP.Transport.(*loggingTransport); ok {
		c.Captured = tr.captured.list
	}
}

func (c *Client) GET(rawURL string, extra map[string]string) (body string, status int, finalURL string, err error) {
	req, _ := http.NewRequest(http.MethodGet, rawURL, nil)
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", 0, "", err
	}
	defer resp.Body.Close()
	c.refreshCaptured()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return string(data), resp.StatusCode, resp.Request.URL.String(), nil
}

func (c *Client) POST(rawURL, postBody string, extra map[string]string) (finalURL, body string, status int, err error) {
	req, _ := http.NewRequest(http.MethodPost, rawURL, strings.NewReader(postBody))
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", "", 0, err
	}
	defer resp.Body.Close()
	c.refreshCaptured()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return resp.Request.URL.String(), string(data), resp.StatusCode, nil
}

func (c *Client) SetCookies(rawURL string, cookies []*http.Cookie) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	c.HTTP.Jar.SetCookies(u, cookies)
	return nil
}

func (c *Client) CookiesFor(rawURL string) ([]*http.Cookie, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	src := c.HTTP.Jar.Cookies(u)
	out := make([]*http.Cookie, 0, len(src))
	for _, c := range src {
		cp := *c
		if cp.Domain == "" {
			cp.Domain = u.Hostname()
		}
		out = append(out, &cp)
	}
	return out, nil
}

func ParseAttemptID(html string) (string, error) {
	if m := LoginAttemptRE.FindStringSubmatch(html); len(m) > 1 {
		return strings.TrimSpace(m[1]), nil
	}
	return "", fmt.Errorf("login_attempt_id not found")
}

func LoginOK(finalURL, body string) bool {
	if AmErrorsRE.FindStringSubmatch(body) != nil {
		return false
	}
	lower := strings.ToLower(body)
	if strings.Contains(lower, `name="amember_login"`) || strings.Contains(lower, `id="amember-login"`) {
		if strings.Contains(strings.ToLower(finalURL), "/login") {
			return false
		}
	}
	return true
}
