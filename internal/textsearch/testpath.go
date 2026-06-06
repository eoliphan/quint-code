package textsearch

import "strings"

// IsTestPath reports whether a file path looks like test / non-production code.
// Ported and trimmed from codegraph's isTestFile — used to de-prioritize test
// files in ranking (the proximity-recall analog of codegraph subtracting from
// scorePathRelevance for test files). Pure, language-general over the common
// Go / JS / TS / Python conventions.
func IsTestPath(path string) bool {
	if path == "" {
		return false
	}
	lower := strings.ToLower(path)

	base := lower
	if i := strings.LastIndexByte(lower, '/'); i >= 0 {
		base = lower[i+1:]
	}

	// Filename patterns: foo_test.go, foo.test.ts, bar_spec.rb, test_baz.py …
	switch {
	case strings.HasPrefix(base, "test_"):
		return true
	case strings.Contains(base, "_test."),
		strings.Contains(base, ".test."),
		strings.Contains(base, "-test."),
		strings.Contains(base, "_spec."),
		strings.Contains(base, ".spec."),
		strings.Contains(base, "-spec."):
		return true
	}

	// Directory patterns — guard with leading "/" so a path without a leading
	// slash still matches a first-segment test dir.
	guarded := "/" + lower
	for _, d := range []string{"/test/", "/tests/", "/testdata/", "/__tests__/", "/spec/", "/specs/", "/testing/"} {
		if strings.Contains(guarded, d) {
			return true
		}
	}
	return false
}

// generatedSuffixes are high-confidence machine-generated file suffixes across the
// common ecosystems (protobuf/gRPC stubs, ORM/codegen output, mocks).
var generatedSuffixes = []string{
	".pb.go", ".pb.gw.go", "_grpc.pb.go", "_pb2.py", "_pb2_grpc.py",
	".gen.go", "_generated.go", "_mock.go",
	".g.dart", ".freezed.dart",
	".designer.cs", ".generated.cs",
	".generated.ts", ".gen.ts",
}

// IsGeneratedPath reports whether a file path looks like machine-generated code
// (protobuf/gRPC stubs, ORM/codegen output, mocks). Such files routinely share
// symbol names with the hand-written code they wrap, so search ranks them last so
// the real implementation surfaces first on a name collision. Pure, path-based
// over high-confidence cross-language generator conventions.
func IsGeneratedPath(path string) bool {
	if path == "" {
		return false
	}
	lower := strings.ToLower(path)
	base := lower
	if i := strings.LastIndexByte(lower, '/'); i >= 0 {
		base = lower[i+1:]
	}
	if strings.HasPrefix(base, "mock_") || strings.HasPrefix(base, "zz_generated") {
		return true
	}
	for _, suf := range generatedSuffixes {
		if strings.HasSuffix(base, suf) {
			return true
		}
	}
	return false
}
