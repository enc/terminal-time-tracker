package cmd

// Subtle ANSI color variables for consistent, shared styling across commands.
// These are intentionally variables (not constants) so callers can disable or
// re-enable coloring at runtime (e.g. when output is redirected or for tests).
// The palette is chosen to be subtle and readable on dark terminal backgrounds.
//
// If you need to disable colors (for non-TTY or CI), call DisableColors().
//
// Note: values use standard ANSI SGR sequences supported by most terminal emulators.

var (
	// Primary controls
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiDim   = "\x1b[2m"

	// Semantic colors (subtle / dim variants for dark shells)
	ansiHeading = "\x1b[1;37m" // bright white bold for headings
	ansiLabel   = "\x1b[2;36m" // dim cyan for labels and left-hand items
	ansiHours   = "\x1b[2;32m" // dim green for hour numbers / totals
	ansiNotes   = "\x1b[90m"   // gray for notes and secondary text
	ansiWarn    = "\x1b[33m"   // yellow for warnings
	ansiOverlap = "\x1b[2;31m" // dim red for overlap/alert markers
)

// Default values are stored so colors can be re-enabled after being disabled.
var (
	defaultAnsiReset   = ansiReset
	defaultAnsiBold    = ansiBold
	defaultAnsiDim     = ansiDim
	defaultAnsiHeading = ansiHeading
	defaultAnsiLabel   = ansiLabel
	defaultAnsiHours   = ansiHours
	defaultAnsiNotes   = ansiNotes
	defaultAnsiWarn    = ansiWarn
	defaultAnsiOverlap = ansiOverlap
)

// DisableColors turns off ANSI sequences by setting all color vars to empty strings.
// Useful for non-TTY output or deterministic test output.
func DisableColors() {
	ansiReset = ""
	ansiBold = ""
	ansiDim = ""
	ansiHeading = ""
	ansiLabel = ""
	ansiHours = ""
	ansiNotes = ""
	ansiWarn = ""
	ansiOverlap = ""
}

// EnableColors restores the palette to the package defaults.
func EnableColors() {
	ansiReset = defaultAnsiReset
	ansiBold = defaultAnsiBold
	ansiDim = defaultAnsiDim
	ansiHeading = defaultAnsiHeading
	ansiLabel = defaultAnsiLabel
	ansiHours = defaultAnsiHours
	ansiNotes = defaultAnsiNotes
	ansiWarn = defaultAnsiWarn
	ansiOverlap = defaultAnsiOverlap
}
