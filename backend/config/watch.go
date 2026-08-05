package config

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Watch 监听 config 目录下的配置文件变化，变化时重新加载并回调 onReload。
// 采用轮询 mtime 方式（比 fsnotify 更稳，兼容编辑器原子替换/挂载目录）。
// 重新加载失败时保留旧配置，仅打印错误（避免写入一半的配置导致服务中断）。
func Watch(configDir string, onReload func(*Config)) {
	go func() {
		last := fingerprint(configDir)
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			cur := fingerprint(configDir)
			if equalFingerprint(cur, last) {
				continue
			}
			cfg, err := Load(configDir)
			if err != nil {
				log.Printf("[config] 热重载失败（保留当前配置）: %v", err)
				continue
			}
			last = cur
			log.Printf("[config] 检测到配置变化，正在热重载...")
			onReload(cfg)
		}
	}()
}

// fingerprint 返回 config 目录下所有配置文件的 (mtime, size)
func fingerprint(dir string) map[string][2]int64 {
	out := make(map[string][2]int64)
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".yaml" || ext == ".yml" || ext == ".txt" {
			out[path] = [2]int64{info.ModTime().UnixNano(), info.Size()}
		}
		return nil
	})
	return out
}

func equalFingerprint(a, b map[string][2]int64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}
