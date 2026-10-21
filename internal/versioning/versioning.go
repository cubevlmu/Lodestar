package versioning

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseVersionCode converts "1.2.3" (optionally "v1.2.3" or "1.2.3-beta.1") into a
// monotonic integer: major*1_000_000 + minor*1_000 + patch.
func ParseVersionCode(version string) (int64, error) {
	v := strings.TrimSpace(version)
	v = strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V")

	if idx := strings.IndexAny(v, "+-"); idx >= 0 {
		v = v[:idx]
	}

	parts := strings.Split(v, ".")
	if len(parts) > 3 {
		return 0, fmt.Errorf("invalid version: %s", version)
	}

	nums := []int64{0, 0, 0}
	for i, p := range parts {
		if p == "" {
			return 0, fmt.Errorf("invalid version: %s", version)
		}
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid version: %s", version)
		}
		nums[i] = n
	}

	return nums[0]*1_000_000 + nums[1]*1_000 + nums[2], nil
}

// archAliases maps the many ways build tools spell a CPU architecture onto the
// single token Lodestar stores, so a CI job reporting "x64" still matches a
// client asking for "amd64".
var archAliases = map[string]string{
	"amd64": "amd64", "x86_64": "amd64", "x86-64": "amd64", "x64": "amd64", "64bit": "amd64",
	"arm64": "arm64", "aarch64": "arm64", "arm64-v8a": "arm64", "arm64v8": "arm64",
	"386": "386", "i386": "386", "i686": "386", "x86": "386", "ia32": "386", "32bit": "386",
	"arm": "arm", "armv7": "arm", "armv7l": "arm", "armeabi-v7a": "arm",
	"universal": "universal", "any": "universal", "all": "universal", "noarch": "universal",
}

// NormalizeArch folds architecture aliases into one canonical token.
// Unknown values are returned lower-cased and trimmed so they still match exactly.
func NormalizeArch(arch string) string {
	v := strings.ToLower(strings.TrimSpace(arch))
	if v == "" {
		return "universal"
	}
	if canonical, ok := archAliases[v]; ok {
		return canonical
	}
	return v
}

var platformAliases = map[string]string{
	"win": "windows", "win32": "windows", "win64": "windows", "windows": "windows",
	"mac": "darwin", "macos": "darwin", "osx": "darwin", "darwin": "darwin",
	"linux": "linux", "android": "android", "ios": "ios", "all": "all",
}

// NormalizePlatform folds platform aliases into one canonical token.
func NormalizePlatform(platform string) string {
	v := strings.ToLower(strings.TrimSpace(platform))
	if v == "" {
		return "all"
	}
	if canonical, ok := platformAliases[v]; ok {
		return canonical
	}
	return v
}
