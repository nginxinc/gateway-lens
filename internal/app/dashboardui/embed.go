// Package dashboardui exposes the compiled dashboard frontend as an embedded file system.
package dashboardui

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed dist
var embeddedAssets embed.FS

// FileSystem returns the embedded dashboard asset tree rooted at the compiled dist directory.
func FileSystem() (fs.FS, error) {
	assets, err := fs.Sub(embeddedAssets, "dist")
	if err != nil {
		return nil, fmt.Errorf("preparing embedded dashboard assets: %w", err)
	}

	return assets, nil
}
