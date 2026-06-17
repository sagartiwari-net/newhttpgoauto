package gfx

// Kind identifies how a GFX task is executed.
type Kind string

const (
	KindExtension Kind = "extension"  // Chrome + GFX extension cookie capture
	KindCredFetch Kind = "cred_fetch" // Scrape username/password from GFX portal
	KindOneClick  Kind = "oneclick"   // Future: no extension (e.g. Vecteezy)
)

// ToolDef is static per-task configuration (selectors, cookie formats, upload ids).
type ToolDef struct {
	TaskUID             string
	Kind                Kind
	Name                string
	ToolURL             string
	Selector            string
	FallbackIndex       int
	WebsiteID           string
	BackupFile          string
	Referer             string
	ToolID              string
	ScrapeName          string
	SessionCookieNames  []string
	CaptureLocalStorage bool
	CaptureIndexedDB    bool
	UseLSPayloadFormat  bool
	SkipPageReload      bool // fast path: capture LS/cookies without reload (e.g. Airbrush)
}
