package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"alice/config"
	"alice/kernel"
	"alice/server"
)

func main() {
	var configDir string
	flag.StringVar(&configDir, "config", "", "配置文件目录（默认: ../config）")
	flag.Parse()

	if configDir == "" {
		configDir = "../config"
	}

	absDir, err := filepath.Abs(configDir)
	if err != nil {
		log.Fatalf("配置文件目录无效: %v", err)
	}

	cfg, err := config.Load(absDir)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	log.SetOutput(os.Stdout)
	log.Printf("Alice 启动 | LLM: %s | Embedding: %s", cfg.Kernel.LLM.Provider, cfg.RAG.Embedding.Model)

	k := kernel.NewKernel(cfg)

	// 配置文件热重载（改配置无需重启）
	config.Watch(absDir, func(newCfg *config.Config) {
		k.Reload(newCfg)
	})

	hub := server.NewHub()

	// 主动推送：情绪超阈值时广播给所有连接
	k.OnProactive(func(text string) {
		payload, _ := json.Marshal(map[string]any{"text": text})
		hub.Broadcast(server.WsMessage{Type: "proactive_message", Payload: payload})
	})

	mux := http.NewServeMux()
	mux.Handle(cfg.Main.Server.WSPath, server.HandleWebSocket(hub, k))
	server.RegisterRoutes(mux, k)

	addr := fmt.Sprintf("%s:%d", cfg.Main.Server.Host, cfg.Main.Server.Port)
	log.Printf("监听 %s | WebSocket: %s", addr, cfg.Main.Server.WSPath)

	if err := http.ListenAndServe(addr, server.CORS(mux)); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
