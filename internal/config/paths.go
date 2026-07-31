package config

import (
	"os"
	"path/filepath"
	"strings"
)

// Paths 运行时关键路径（相对 server 根目录解析）。
type Paths struct {
	Root       string // server 根
	Configs    string
	TablesXML  string
	TablesBin  string
	StaticGM   string
	Data       string // data/ 属性表、野怪配置
	NieoData   string // 默认 ../NieoData
	IPFile     string // configs/ip.txt
	Advertise  string // configs/advertise_host.txt
}

func Resolve(serverRoot string) Paths {
	root := serverRoot
	if root == "" {
		root, _ = os.Getwd()
	}
	return Paths{
		Root:      root,
		Configs:   filepath.Join(root, "configs"),
		TablesXML: filepath.Join(root, "tables", "xml"),
		TablesBin: filepath.Join(root, "tables", "bin"),
		StaticGM:  filepath.Join(root, "static", "gm"),
		Data:      filepath.Join(root, "data"),
		NieoData:  filepath.Clean(filepath.Join(root, "..", "NieoData")),
		IPFile:    filepath.Join(root, "configs", "ip.txt"),
		Advertise: filepath.Join(root, "configs", "advertise_host.txt"),
	}
}

func ReadIP(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stripBOM(string(b))), nil
}

func ReadAdvertiseHost(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(stripBOM(string(b)))
	if i := strings.IndexAny(line, "\r\n"); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	return line, nil
}

func WriteIP(path, hostPort string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// 无 BOM，避免 Flash/客户端把 IP 解析坏
	return os.WriteFile(path, []byte(strings.TrimSpace(hostPort)+"\n"), 0o644)
}

func stripBOM(s string) string {
	return strings.TrimPrefix(s, "\ufeff")
}
