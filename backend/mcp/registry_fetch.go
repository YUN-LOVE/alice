package mcp

import (
	"context"
	"io"
	"net/http"
	"time"
)

// fetchURL 下载远程注册表
func fetchURL(url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &MCPError{"注册表下载失败: " + resp.Status}
	}
	return io.ReadAll(io.LimitReader(resp.Body, 2<<20))
}
