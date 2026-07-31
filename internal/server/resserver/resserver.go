package resserver

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"niaohao/server/internal/conslog"
)

// ListenAndServe 将 NieoData 作为静态资源根目录提供 HTTP 访问。
// 对 dll/NieoCore.swf 等关键文件：若 NieoData 缺失，自动回退到同级 tmp/dll。
func ListenAndServe(addr, nieoDataDir, ipFile string) error {
	mux := http.NewServeMux()
	tmpRoot := filepath.Clean(filepath.Join(nieoDataDir, "..", "tmp"))
	tmpDLL := filepath.Join(tmpRoot, "dll")

	mux.HandleFunc("/ip.txt", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		b, err := os.ReadFile(ipFile)
		if err != nil {
			http.Error(w, "ip.txt missing", http.StatusNotFound)
			logRes(r, 404, 0, "MISS ip.txt", start)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		n, _ := w.Write(b)
		logRes(r, 200, n, "OK configs/ip.txt", start)
	})

	files := http.FileServer(http.Dir(nieoDataDir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			logRes(r, 405, 0, "METHOD", start)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		// 浏览器常缓存旧 PetFightDLL（曾打 ifeq/petwar 补丁），Debug 投影器不走该缓存 → 同账号一边进战一边黑屏。
		setSWFNoCache(w, path)
		if path == "" || strings.HasSuffix(r.URL.Path, "/") {
			rw := &statusWriter{ResponseWriter: w, code: 200}
			files.ServeHTTP(rw, r)
			logRes(r, rw.code, rw.n, "DIR/INDEX", start)
			return
		}

		primary := filepath.Join(nieoDataDir, filepath.FromSlash(path))
		if fi, err := os.Stat(primary); err == nil && fi.Mode().IsRegular() {
			rw := &statusWriter{ResponseWriter: w, code: 200}
			files.ServeHTTP(rw, r)
			logRes(r, rw.code, rw.n, "OK NieoData", start)
			return
		}

		// dll/* → tmp/dll
		if strings.HasPrefix(strings.ToLower(path), "dll/") {
			fallback := filepath.Join(tmpDLL, filepath.Base(path))
			if fi, err := os.Stat(fallback); err == nil && fi.Mode().IsRegular() {
				rw := &statusWriter{ResponseWriter: w, code: 200}
				http.ServeFile(rw, r, fallback)
				logRes(r, rw.code, rw.n, "OK tmp/dll fallback", start)
				return
			}
		}

		// 根目录版本文件等 → tmp/
		base := filepath.Base(path)
		if !strings.Contains(path, "/") {
			for _, cand := range []string{
				filepath.Join(tmpRoot, base),
				filepath.Join(tmpDLL, base),
			} {
				if fi, err := os.Stat(cand); err == nil && fi.Mode().IsRegular() {
					rw := &statusWriter{ResponseWriter: w, code: 200}
					http.ServeFile(rw, r, cand)
					logRes(r, rw.code, rw.n, "OK tmp fallback", start)
					return
				}
			}
		}

		rw := &statusWriter{ResponseWriter: w, code: 200}
		files.ServeHTTP(rw, r)
		tag := "OK"
		if rw.code == 404 || rw.code >= 400 {
			tag = "MISS"
		}
		if rw.code == 404 {
			tag = "MISS"
		}
		logRes(r, rw.code, rw.n, tag, start)
	})

	fmt.Printf("[res] serving %s on %s (ip.txt -> %s)\n", nieoDataDir, addr, ipFile)
	fmt.Printf("[res] dll fallback: %s\n", tmpDLL)
	fmt.Println("[res] resource request logs: ON ([RES] ...)")
	return http.ListenAndServe(addr, mux)
}

type statusWriter struct {
	http.ResponseWriter
	code int
	n    int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.code == 0 {
		w.code = 200
	}
	n, err := w.ResponseWriter.Write(b)
	w.n += n
	return n, err
}

func logRes(r *http.Request, code, bytes int, tag string, start time.Time) {
	path := r.URL.Path
	if r.URL.RawQuery != "" {
		// 截断过长 query（版本号等保留前 48）
		q := r.URL.RawQuery
		if len(q) > 48 {
			q = q[:48] + "..."
		}
		path = path + "?" + q
	}
	ms := time.Since(start).Milliseconds()
	line := fmt.Sprintf("[RES] %3d %-6s %-48s %8dB %4dms  %s  from=%s",
		code, r.Method, trimPath(path, 48), bytes, ms, tag, r.RemoteAddr)
	// 黄字：缺资源 404 / MISS
	if code == 404 || strings.Contains(tag, "MISS") {
		log.Print(conslog.Yellow(line))
		return
	}
	log.Print(line)
}

func trimPath(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// setSWFNoCache 禁止浏览器/Pepper 长期缓存 .swf（尤其 PetFightDLL）。
func setSWFNoCache(w http.ResponseWriter, path string) {
	if !strings.HasSuffix(strings.ToLower(path), ".swf") {
		return
	}
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

func Join(elem ...string) string {
	return filepath.Join(elem...)
}
