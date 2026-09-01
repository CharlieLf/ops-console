package main

import (
	"fmt"
	"syscall"
)

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit && exp < 4; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTP"[exp])
}

func pluralize(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

type DiskStat struct {
	Path      string  `json:"path"`
	Total     int64   `json:"total"`
	Used      int64   `json:"used"`
	Free      int64   `json:"free"`
	UsedRatio float64 `json:"usedRatio"`
}

// diskStat reports usage of the filesystem backing path. The data directory is
// a bind mount from the host, so this reflects the host disk.
func diskStat(path string) (DiskStat, error) {
	var fs syscall.Statfs_t
	if err := syscall.Statfs(path, &fs); err != nil {
		return DiskStat{}, err
	}
	total := int64(fs.Blocks) * int64(fs.Bsize)
	free := int64(fs.Bavail) * int64(fs.Bsize)
	used := total - int64(fs.Bfree)*int64(fs.Bsize)
	stat := DiskStat{Path: path, Total: total, Used: used, Free: free}
	if total > 0 {
		stat.UsedRatio = float64(used) / float64(total)
	}
	return stat, nil
}
