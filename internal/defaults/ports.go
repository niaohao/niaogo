package defaults

// 本项目端口（刻意避开参考原代码：28080/40320/28088/40321/21863/27500）
const (
	GMHTTP     = 29180 // GM 后台 HTTP
	ResHTTP    = 41520 // 资源服主 HTTP（NieoData + /ip.txt）
	ResHTTP80  = 29188 // 资源服备用
	LoginIPHTTP = 41521 // 备用 /ip.txt HTTP（命名保留兼容）
	LoginTCP   = 22973 // 登录服 TCP（104/105/108）
	GameTCP    = 28610 // 游戏频道服 TCP（1001）
)

// MySQL 默认（可用环境变量 / CLI 覆盖）
const (
	MySQLHost = "127.0.0.1"
	MySQLPort = 3306
	MySQLUser = "root"
	MySQLPass = "root"
	MySQLDB   = "niao"
)
