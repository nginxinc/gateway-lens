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

package main

import (
	"io"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

type failingWriter struct{}

func (failingWriter) Write(_ []byte) (int, error) {
	return 0, io.ErrClosedPipe
}

type rootCommandTestCase struct {
	name               string
	args               []string
	expectedOutMatcher func() OmegaMatcher
}

func TestRootCommand(t *testing.T) {
	t.Parallel()

	for _, testCase := range rootCommandTestCases() {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runRootCommandTest(t, testCase)
		})
	}
}

func rootCommandTestCases() []rootCommandTestCase {
	return []rootCommandTestCase{
		{
			name: "uses version subcommand",
			args: []string{versionCommandName},
			expectedOutMatcher: func() OmegaMatcher {
				return ContainSubstring("version=")
			},
		},
		{
			name: "prints help usage",
			args: []string{"--help"},
			expectedOutMatcher: func() OmegaMatcher {
				return And(
					ContainSubstring("Usage:"),
					ContainSubstring(versionCommandName),
					ContainSubstring("--namespaces"),
				)
			},
		},
	}
}

func runRootCommandTest(t *testing.T, testCase rootCommandTestCase) {
	t.Helper()

	g := NewWithT(t)

	var out strings.Builder

	var errOut strings.Builder

	rootCmd := newRootCommand(&out, &errOut)
	rootCmd.SetArgs(testCase.args)

	err := rootCmd.ExecuteContext(t.Context())

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(out.String()).To(testCase.expectedOutMatcher())
	g.Expect(errOut.String()).To(BeEmpty())
}

func TestRunRejectsInvalidLogLevel(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	err := run(t.Context(), defaultPort, "bogus", "", nil)
	g.Expect(err).To(MatchError(ContainSubstring(`invalid log level: "bogus"`)))
}

func TestRunRejectsInvalidPort(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name string
		port int
	}{
		{name: "zero", port: 0},
		{name: "negative", port: -1},
		{name: "too large", port: 65536},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			g := NewWithT(t)

			err := run(t.Context(), testCase.port, defaultLogLevel, "", nil)
			g.Expect(err).To(MatchError(ContainSubstring("invalid port")))
		})
	}
}

func TestRunRejectsInvalidNamespace(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name       string
		namespaces []string
	}{
		{name: "uppercase", namespaces: []string{"Invalid"}},
		{name: "underscore", namespaces: []string{"invalid_namespace"}},
		{name: "leading hyphen", namespaces: []string{"-invalid"}},
		{name: "empty string", namespaces: []string{""}},
		{name: "valid then invalid", namespaces: []string{"valid-ns", "Invalid"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			g := NewWithT(t)

			err := run(t.Context(), defaultPort, defaultLogLevel, "", testCase.namespaces)
			g.Expect(err).To(MatchError(ContainSubstring("invalid namespace")))
		})
	}
}

func TestRunRejectsInvalidBasePath(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name     string
		basePath string
	}{
		{name: "just a slash", basePath: "/"},
		{name: "only slashes", basePath: "///"},
		{name: "contains a space", basePath: "/gateway lens"},
		{name: "full URL", basePath: "http://example.com/gateway-lens"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			g := NewWithT(t)

			err := run(t.Context(), defaultPort, defaultLogLevel, testCase.basePath, nil)
			g.Expect(err).To(MatchError(ContainSubstring("invalid base path")))
		})
	}
}

func TestNormalizeBasePath(t *testing.T) {
	t.Parallel()

	path := "/gateway-lens"

	for _, testCase := range []struct {
		name     string
		raw      string
		expected string
	}{
		{name: "empty", raw: "", expected: ""},
		{name: "already normalized", raw: path, expected: path},
		{name: "missing leading slash", raw: "gateway-lens", expected: path},
		{name: "trailing slash", raw: "/gateway-lens/", expected: path},
		{name: "nested path", raw: "/foo/gateway-lens/", expected: "/foo/gateway-lens"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			g := NewWithT(t)

			result, err := normalizeBasePath(testCase.raw)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(result).To(Equal(testCase.expected))
		})
	}
}

func TestValidateNamespaces(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name        string
		namespaces  []string
		expectError bool
	}{
		{name: "nil namespaces", namespaces: nil, expectError: false},
		{name: "empty slice", namespaces: []string{}, expectError: false},
		{name: "single valid namespace", namespaces: []string{"default"}, expectError: false},
		{name: "multiple valid namespaces", namespaces: []string{"default", "kube-system"}, expectError: false},
		{name: "invalid uppercase", namespaces: []string{"Default"}, expectError: true},
		{name: "invalid underscore", namespaces: []string{"kube_system"}, expectError: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			g := NewWithT(t)

			err := validateNamespaces(testCase.namespaces)

			if testCase.expectError {
				g.Expect(err).To(MatchError(ContainSubstring("invalid namespace")))

				return
			}

			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func TestVersionCommand(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	var out strings.Builder

	cmd := newVersionCommand(&out, "v1.2.3", "abcdef123456")

	err := cmd.ExecuteContext(t.Context())

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(out.String()).To(Equal("version=v1.2.3 commit=abcdef123456\n"))
}

func TestPrintVersion(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name          string
		newWriter     func() io.Writer
		expectedOut   string
		expectedError string
	}{
		{
			name: "writes version string",
			newWriter: func() io.Writer {
				return &strings.Builder{}
			},
			expectedOut: "version=v0.1.0 commit=abc123\n",
		},
		{
			name: "returns wrapped writer error",
			newWriter: func() io.Writer {
				return failingWriter{}
			},
			expectedError: "writing version output: io: read/write on closed pipe",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			g := NewWithT(t)
			writer := testCase.newWriter()
			err := printVersion(writer, "v0.1.0", "abc123")

			if testCase.expectedError != "" {
				g.Expect(err).To(MatchError(testCase.expectedError))

				return
			}

			g.Expect(err).ToNot(HaveOccurred())

			builder, ok := writer.(*strings.Builder)
			g.Expect(ok).To(BeTrue())
			g.Expect(builder.String()).To(Equal(testCase.expectedOut))
		})
	}
}
