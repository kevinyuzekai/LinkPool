package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/kevinyuzekai/LinkPool/internal/api"
	"github.com/kevinyuzekai/LinkPool/internal/app"
	"github.com/kevinyuzekai/LinkPool/web"
)

// Set via -ldflags "-X main.version=…"
var version = "0.1.1"

func main() {
	uiAddr := flag.String("ui", "127.0.0.1:8787", "控制面板监听地址")
	httpProxy := flag.String("http-proxy", "127.0.0.1:18080", "HTTP 代理默认地址")
	socksProxy := flag.String("socks-proxy", "127.0.0.1:11080", "SOCKS5 代理默认地址")
	openFlag := flag.Bool("open", false, "启动后打开浏览器")
	showVer := flag.Bool("version", false, "打印版本")
	flag.Parse()

	if *showVer {
		fmt.Println("LinkPool", version)
		return
	}

	autoOpen := *openFlag || runningFromAppBundle()

	application := app.New()
	_ = application.SetListenAddrs(*httpProxy, *socksProxy)
	_, _ = application.RefreshAdapters()

	mux := http.NewServeMux()
	apiHandler := &api.Handler{App: application}
	apiHandler.Mount(mux)

	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		b, err := fs.ReadFile(web.Static, "static/index.html")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(b)
	})

	server := &http.Server{Addr: *uiAddr, Handler: mux}

	go func() {
		log.Printf("LinkPool %s UI → http://%s", version, *uiAddr)
		log.Printf("HTTP 代理默认 %s · SOCKS5 默认 %s", *httpProxy, *socksProxy)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	if autoOpen {
		uiURL := fmt.Sprintf("http://%s", *uiAddr)
		if err := openBrowser(uiURL); err != nil {
			log.Printf("打开浏览器失败: %v — 请手动打开 %s（本应用无原生窗口）", err, uiURL)
		}
		if runningFromAppBundle() {
			notifyUIReady(uiURL)
		}
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	log.Println("正在停止…")
	_ = application.StopProxy()
	_ = server.Close()
}

func runningFromAppBundle() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	exe, _ = filepath.EvalSymlinks(exe)
	return strings.Contains(exe, ".app/Contents/MacOS/")
}
