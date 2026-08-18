package dashboardui_test

import (
	"io/fs"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/nginxinc/gateway-lens/internal/app/dashboardui"
)

func TestFileSystem(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	assets, err := dashboardui.FileSystem()
	g.Expect(err).ToNot(HaveOccurred())

	indexHTML, err := fs.ReadFile(assets, "index.html")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(indexHTML)).To(ContainSubstring("Gateway Lens"))
	g.Expect(string(indexHTML)).To(ContainSubstring("/assets/"))
}
