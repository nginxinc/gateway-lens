/*
Copyright 2026 F5, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
