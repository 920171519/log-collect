package collector

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Remote paths are always POSIX paths, even when the desktop app runs on
// Windows. Local paths use filepath so drive letters are preserved.
func pathJoin(parts ...string) string {
	return path.Join(parts...)
}

func pathJoinLocal(parts ...string) string {
	return filepath.Join(parts...)
}

func expandDownloadRoot(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "~" || strings.HasPrefix(dir, "~/") || strings.HasPrefix(dir, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("无法解析用户目录：%w", err)
		}
		dir = filepath.Join(home, strings.TrimLeft(dir[1:], "/\\"))
	}
	if dir == "" {
		return "", fmt.Errorf("本地下载目录不能为空")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("本地下载目录无效：%w", err)
	}
	return abs, nil
}
