package gfx

import (
	"fmt"
	"log"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// AttachPageDiagnostics attaches event listeners to a page to capture console errors and network failures.
func attachPageDiagnostics(page *rod.Page, prefix string) {
	_ = proto.RuntimeEnable{}.Call(page)
	_ = proto.NetworkEnable{}.Call(page)

	// 1. Console API calls (log, error, info, etc.)
	go page.EachEvent(func(e *proto.RuntimeConsoleAPICalled) {
		var args []string
		for _, arg := range e.Args {
			val := fmt.Sprintf("%v", arg.Value)
			if len(val) > 200 {
				val = val[:200] + "..."
			}
			args = append(args, val)
		}
		log.Printf("[%s-Console-%s] %s", prefix, e.Type, strings.Join(args, " "))
	})

	// 2. Uncaught JS Exceptions
	go page.EachEvent(func(e *proto.RuntimeExceptionThrown) {
		desc := ""
		if e.ExceptionDetails.Exception != nil {
			desc = e.ExceptionDetails.Exception.Description
		}
		log.Printf("[%s-JS-Err] %s | Details: %s", prefix, e.ExceptionDetails.Text, desc)
	})

	// 3. Network HTTP responses with status >= 400
	go page.EachEvent(func(e *proto.NetworkResponseReceived) {
		if e.Response.Status >= 400 {
			log.Printf("[%s-Net-HTTP-%d] URL: %s", prefix, e.Response.Status, e.Response.URL)
		}
	})

	// 4. Network Loading Failures (like net::ERR_CONNECTION_TIMED_OUT or proxy issues)
	go page.EachEvent(func(e *proto.NetworkLoadingFailed) {
		log.Printf("[%s-Net-Failed] Request ID: %s | Error: %s | BlockedReason: %s", prefix, e.RequestID, e.ErrorText, e.BlockedReason)
	})
}
