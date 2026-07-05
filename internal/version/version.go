package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
)

// ldflags 注入变量（scripts/build.sh 通过 -X 设置）。
// 为空时 fallback 到 runtime/debug.ReadBuildInfo 的 VCS 嵌入信息。
var (
	buildVersion string // git describe --tags --always --dirty
	buildCommit  string // git rev-parse --short=8 HEAD
	buildTime    string // 构建时间 ISO8601 UTC
)

// Info 描述构建版本信息。
type Info struct {
	Version   string // tag 或 dev-<short>[-dirty] 或 unknown
	Commit    string // 短 commit hash（8 位）
	BuildTime string // 构建时间
	Dirty     bool   // 工作区是否有未提交改动
	GoVersion string // runtime.Version()
}

var (
	once sync.Once
	info Info
)

// Get 返回版本信息，首次调用时懒加载。
func Get() Info {
	once.Do(load)
	return info
}

// String 返回多行格式化的版本信息，供 -version 命令行输出。
// 空字段（commit/built）自动省略，适配 go run 等无 VCS 嵌入的场景。
func String() string {
	i := Get()
	var b strings.Builder
	fmt.Fprintf(&b, "vistty %s", i.Version)
	if i.Commit != "" {
		fmt.Fprintf(&b, "\n  commit: %s", i.Commit)
	}
	if i.BuildTime != "" {
		fmt.Fprintf(&b, "\n  built:  %s", i.BuildTime)
	}
	if i.Dirty {
		fmt.Fprintf(&b, "\n  dirty:  true")
	}
	fmt.Fprintf(&b, "\n  go:     %s", i.GoVersion)
	return b.String()
}

func load() {
	info.GoVersion = runtime.Version()

	// ldflags 注入优先
	if buildVersion != "" {
		info.Version = buildVersion
		if strings.Contains(buildVersion, "-dirty") {
			info.Dirty = true
		}
	}
	if buildCommit != "" {
		info.Commit = buildCommit
	}
	if buildTime != "" {
		info.BuildTime = buildTime
	}

	// fallback: Go 1.18+ 在 git 仓库内构建自动嵌入的 VCS 信息
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if info.Commit == "" && len(s.Value) >= 8 {
					info.Commit = s.Value[:8]
				}
			case "vcs.time":
				if info.BuildTime == "" {
					info.BuildTime = s.Value
				}
			case "vcs.modified":
				if s.Value == "true" {
					info.Dirty = true
				}
			}
		}
	}

	// 未注入 ldflags 时，由 commit 构造开发版本号
	if info.Version == "" {
		if info.Commit != "" {
			info.Version = "dev-" + info.Commit
			if info.Dirty {
				info.Version += "-dirty"
			}
		} else {
			info.Version = "develop"
		}
	}
}
