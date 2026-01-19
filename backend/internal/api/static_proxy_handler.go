package api

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// StaticProxyHandler 处理静态资源代理请求
type StaticProxyHandler struct {
	targetURL  *url.URL
	httpClient *http.Client
}

// NewStaticProxyHandler 创建静态资源代理处理器
// targetURL: MinIO 服务器地址，如 http://localhost:9005
func NewStaticProxyHandler(targetURL string) (*StaticProxyHandler, error) {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}

	return &StaticProxyHandler{
		targetURL: parsed,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// ServeHTTP 代理请求到 MinIO
func (h *StaticProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 只允许 GET 和 HEAD 方法
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 构建目标 URL
	// 请求: /ndr-assets/assets/12/image.png
	// 目标: http://localhost:9005/ndr-assets/assets/12/image.png
	targetURL := *h.targetURL
	targetURL.Path = r.URL.Path
	targetURL.RawQuery = r.URL.RawQuery

	// 创建代理请求
	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL.String(), nil)
	if err != nil {
		log.Printf("[static-proxy] Failed to create request: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// 复制必要的请求头
	copyHeaders := []string{"Accept", "Accept-Encoding", "Range", "If-Modified-Since", "If-None-Match"}
	for _, header := range copyHeaders {
		if val := r.Header.Get(header); val != "" {
			proxyReq.Header.Set(header, val)
		}
	}

	// 发送请求
	resp, err := h.httpClient.Do(proxyReq)
	if err != nil {
		log.Printf("[static-proxy] Request failed: %v", err)
		http.Error(w, "Bad gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// 复制响应头
	for key, values := range resp.Header {
		// 跳过 hop-by-hop 头
		if isHopByHopHeader(key) {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// 添加 CORS 头（允许跨域访问静态资源）
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// 写入状态码和响应体
	w.WriteHeader(resp.StatusCode)
	if r.Method != http.MethodHead {
		io.Copy(w, resp.Body)
	}
}

// isHopByHopHeader 判断是否为 hop-by-hop 头
func isHopByHopHeader(header string) bool {
	hopByHopHeaders := map[string]bool{
		"Connection":          true,
		"Keep-Alive":          true,
		"Proxy-Authenticate":  true,
		"Proxy-Authorization": true,
		"Te":                  true,
		"Trailers":            true,
		"Transfer-Encoding":   true,
		"Upgrade":             true,
	}
	return hopByHopHeaders[strings.Title(strings.ToLower(header))]
}
