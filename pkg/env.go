// all environment variables including default values put here

package pkg

import "os"

var (
	// Logo is the emoji icon displayed in the terminal.
	// Overridable at compile time via -ldflags.
	Logo = "🦞"

	// AppName is the user-visible application display name.
	// Overridable at compile time via -ldflags.
	AppName = "PicoClaw"

	// EnvPrefix is the prefix for all environment variables.
	// Overridable at compile time via -ldflags.
	EnvPrefix = "PICOCLAW_"

	// DefaultHome is the default config directory name (relative to user home).
	// Overridable at compile time via -ldflags.
	DefaultHome = ".picoclaw"

	// CommandName is the CLI command name (lowercase).
	// Overridable at compile time via -ldflags.
	CommandName = "picoclaw"
)

const (
	WorkspaceName = "workspace"
)

// GetEnv reads an environment variable with the configured prefix,
// falling back to the PICOCLAW_ prefix for backward compatibility.
func GetEnv(suffix string) string {
	if v := os.Getenv(EnvPrefix + suffix); v != "" {
		return v
	}
	return os.Getenv("PICOCLAW_" + suffix)
}

// LookupEnv is like GetEnv but reports whether the key was present.
// Matches os.LookupEnv semantics: an empty-but-set variable returns ("", true).
func LookupEnv(suffix string) (string, bool) {
	if v, ok := os.LookupEnv(EnvPrefix + suffix); ok {
		return v, true
	}
	return os.LookupEnv("PICOCLAW_" + suffix)
}
