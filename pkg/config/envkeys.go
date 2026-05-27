// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/sipeed/picoclaw/pkg"
)

// Runtime environment variable keys for the process.
// These control the location of files and binaries at runtime and are read
// directly via os.Getenv / os.LookupEnv. Values are built from pkg.EnvPrefix
// so they follow the configured brand prefix.
var (
	EnvHome          = pkg.EnvPrefix + "HOME"
	EnvConfig        = pkg.EnvPrefix + "CONFIG"
	EnvBuiltinSkills = pkg.EnvPrefix + "BUILTIN_SKILLS"
	EnvBinary        = pkg.EnvPrefix + "BINARY"
	EnvGatewayHost   = pkg.EnvPrefix + "GATEWAY_HOST"
)

func GetHome() string {
	homePath, _ := os.UserHomeDir()
	if picoclawHome := os.Getenv(EnvHome); picoclawHome != "" {
		homePath = picoclawHome
	} else if homePath != "" {
		homePath = filepath.Join(homePath, pkg.DefaultHome)
	}
	if homePath == "" {
		homePath = "."
	}
	return homePath
}

// envOptions returns env.Options that maps both the configured prefix
// and the legacy PICOCLAW_ prefix into the environment, so struct tags
// work regardless of which env vars the user has set.
func envOptions() env.Options {
	raw := os.Environ()
	envMap := make(map[string]string, len(raw))
	for _, e := range raw {
		k, v, _ := strings.Cut(e, "=")
		envMap[k] = v
	}
	// Fallback: PICOCLAW_* → configured prefix_*
	for _, e := range raw {
		k, v, _ := strings.Cut(e, "=")
		if strings.HasPrefix(k, "PICOCLAW_") {
			newKey := pkg.EnvPrefix + strings.TrimPrefix(k, "PICOCLAW_")
			if _, ok := envMap[newKey]; !ok {
				envMap[newKey] = v
			}
		}
	}
	return env.Options{Environment: envMap}
}
