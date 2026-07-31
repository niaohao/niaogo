package store

// Store 增量存档接口（最小面）。
// 实际注入类型为 *MySQL：优先连接 MySQL；检测不到数据库时自动回落 JSON 文件（data/saves）。
type Store interface {
	Ping() error
	Backend() string
}
