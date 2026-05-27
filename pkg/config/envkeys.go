// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package config

import (
	"os"
	"path/filepath"

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
