// Package codeberg provides the Codeberg backend for appimg.
// Importing this package registers the "codeberg" source prefix.
package codeberg

import (
	"github.com/sivel/appimg/internal/backend"
	"github.com/sivel/appimg/internal/backend/github"
)

func init() {
	backend.Register("codeberg", func() backend.Backend {
		return github.NewWithConfig(github.Config{
			APIBase:     "https://codeberg.org/api/v1",
			HostURL:     "https://codeberg.org",
			TokenEnvVar: "CODEBERG_TOKEN",
		})
	})
}
