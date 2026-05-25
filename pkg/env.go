// all environment variables including default values put here

package pkg

const (
	Logo = "🦞"
	// AppName is the name of the app
	AppName = "ZhosClaw"
)

// DefaultPicoClawHome is the default home directory name.
// Overridable at compile time via -ldflags.
var DefaultPicoClawHome = ".zhosclaw"

// CommandName is the CLI command name (lowercase).
// Overridable at compile time via -ldflags.
var CommandName = "zhosclaw"

const (
	WorkspaceName = "workspace"
)
