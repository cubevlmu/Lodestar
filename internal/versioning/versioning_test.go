package versioning

import "testing"

func TestParseVersionCode(t *testing.T) {
	cases := []struct {
		version string
		want    int64
	}{
		{"1.0.0", 1000000},
		{"1.4.2", 1004002},
		{"v1.4.2", 1004002},
		{"V2.0.1", 2000001},
		{"1.4.2-beta.1", 1004002},
		{"1.4.2+build.9", 1004002},
		{"0.0.1", 1},
		{"3", 3000000},
		{"3.2", 3002000},
	}
	for _, tc := range cases {
		got, err := ParseVersionCode(tc.version)
		if err != nil {
			t.Fatalf("ParseVersionCode(%q) error: %v", tc.version, err)
		}
		if got != tc.want {
			t.Fatalf("ParseVersionCode(%q) = %d, want %d", tc.version, got, tc.want)
		}
	}
}

func TestParseVersionCodeRejectsInvalid(t *testing.T) {
	for _, version := range []string{"", "1.2.3.4", "a.b.c", "1..2", "-1.0.0"} {
		if _, err := ParseVersionCode(version); err == nil {
			t.Fatalf("expected %q to be rejected", version)
		}
	}
}

func TestParseVersionCodeIsMonotonic(t *testing.T) {
	ordered := []string{"0.9.9", "1.0.0", "1.0.1", "1.1.0", "1.9.9", "2.0.0"}
	var previous int64 = -1
	for _, version := range ordered {
		code, err := ParseVersionCode(version)
		if err != nil {
			t.Fatalf("ParseVersionCode(%q): %v", version, err)
		}
		if code <= previous {
			t.Fatalf("%s (%d) is not greater than the previous version code (%d)", version, code, previous)
		}
		previous = code
	}
}

func TestNormalizeArch(t *testing.T) {
	cases := map[string]string{
		"amd64":     "amd64",
		"x64":       "amd64",
		"x86_64":    "amd64",
		"X86-64":    "amd64",
		"aarch64":   "arm64",
		"arm64-v8a": "arm64",
		"i686":      "386",
		"x86":       "386",
		"armv7l":    "arm",
		"all":       "universal",
		"any":       "universal",
		"":          "universal",
		"  amd64  ": "amd64",
		"riscv64":   "riscv64",
		"ARM64":     "arm64",
	}
	for input, want := range cases {
		if got := NormalizeArch(input); got != want {
			t.Fatalf("NormalizeArch(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizePlatform(t *testing.T) {
	cases := map[string]string{
		"windows": "windows",
		"win32":   "windows",
		"Win64":   "windows",
		"macos":   "darwin",
		"osx":     "darwin",
		"mac":     "darwin",
		"Linux":   "linux",
		"android": "android",
		"":        "all",
		"freebsd": "freebsd",
	}
	for input, want := range cases {
		if got := NormalizePlatform(input); got != want {
			t.Fatalf("NormalizePlatform(%q) = %q, want %q", input, got, want)
		}
	}
}
