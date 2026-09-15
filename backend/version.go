package main

import "strings"

// 版本号由发布流程通过 -ldflags 注入，未注入时为 dev：
//
//	go build -ldflags "-X main.version=1.2.0 -X main.commit=abc1234 -X main.buildTime=2026-01-01T00:00:00Z"
//
// 源码运行与本地构建保持 dev，更新检查会提示“尚不支持自动更新”。
var (
	version   = "dev"
	commit    = ""
	buildTime = ""
)

// looksLikeReleaseTag reports whether a tag is a usable release number. The
// check mirrors versionParts: digits, dots and optional "-"/"+" suffixes are
// accepted, while names such as "dev" or "nightly" are description-only.
func looksLikeReleaseTag(tag string) bool {
	core := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	if core == "" {
		return false
	}
	for i := 0; i < len(core); i++ {
		c := core[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c == '.' || c == '-' || c == '+' || c == '_':
		default:
			return false
		}
	}
	_, err := versionParts(core)
	return err == nil
}

// devVersion reports whether this build cannot accept self-updates.
func devVersion() bool {
	v := strings.TrimSpace(version)
	return v == "" || strings.EqualFold(v, "dev") || !looksLikeReleaseTag(v)
}
