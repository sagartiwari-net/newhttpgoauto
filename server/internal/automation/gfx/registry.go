package gfx

var registry map[string]ToolDef

func init() {
	initRegistry()
}

// IsGFXTask reports whether taskUID is a registered GFX automation.
func IsGFXTask(taskUID string) bool {
	_, ok := registry[taskUID]
	return ok
}

// ToolFor returns the tool definition for a task UID.
func ToolFor(taskUID string) (ToolDef, bool) {
	t, ok := registry[taskUID]
	return t, ok
}

// AllTaskUIDs returns every registered GFX task id.
func AllTaskUIDs() []string {
	out := make([]string, 0, len(registry))
	for uid := range registry {
		out = append(out, uid)
	}
	return out
}
