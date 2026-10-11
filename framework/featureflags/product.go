package featureflags

// Product behavior flags. These are separate from the section.* menu flags:
// turning one off changes what the gateway does on the next request.
const (
	FlagBrowserPromptGuard = "browser.prompt_guard"
	FlagBrowserFileScan    = "browser.file_scan"
	FlagCacheResponses     = "cache.responses"
)

func init() {
	defs := []FlagDef{
		{
			ID:          FlagBrowserPromptGuard,
			DisplayName: "Browser prompt guard",
			Description: "Evaluate prompts from Guard agents. Off: prompts are forwarded and not stored in Prompt Logs.",
			Default:     true,
		},
		{
			ID:          FlagBrowserFileScan,
			DisplayName: "Browser file scan",
			Description: "Scan file and voice uploads from Guard agents. Off: uploads are allowed and not stored.",
			Default:     true,
		},
		{
			ID:          FlagCacheResponses,
			DisplayName: "Response cache",
			Description: "Use the Caching settings (direct and semantic) on inference requests. Off: every request goes to the provider.",
			Default:     true,
		},
	}
	for _, def := range defs {
		MustRegister(def)
	}
}
