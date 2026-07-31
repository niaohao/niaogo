package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"niaohao/server/internal/config"
	"niaohao/server/internal/conslog"
	"niaohao/server/internal/defaults"
	"niaohao/server/internal/gm"
	"niaohao/server/internal/server/gameserver"
	"niaohao/server/internal/server/loginserver"
	"niaohao/server/internal/server/resserver"
	"niaohao/server/internal/store"
	"niaohao/server/internal/tableloader"
)

func main() {
	conslog.Enable()
	root := flag.String("root", findServerRoot(), "server 根目录")
	gmPort := flag.Int("gm-port", defaults.GMHTTP, "GM HTTP 端口")
	resPort := flag.Int("res-port", defaults.ResHTTP, "资源 HTTP 端口")
	loginPort := flag.Int("login-tcp-port", defaults.LoginTCP, "登录 TCP 端口")
	gamePort := flag.Int("game-port", defaults.GameTCP, "游戏 TCP 端口")
	skipRes := flag.Bool("skip-res", false, "不启动资源服")
	mysqlHost := flag.String("mysql-host", envOr("NIAO_MYSQL_HOST", defaults.MySQLHost), "MySQL host")
	mysqlPort := flag.Int("mysql-port", envInt("NIAO_MYSQL_PORT", defaults.MySQLPort), "MySQL port")
	mysqlUser := flag.String("mysql-user", envOr("NIAO_MYSQL_USER", defaults.MySQLUser), "MySQL user")
	mysqlPass := flag.String("mysql-pass", envOr("NIAO_MYSQL_PASS", defaults.MySQLPass), "MySQL password")
	mysqlDB := flag.String("mysql-db", envOr("NIAO_MYSQL_DB", defaults.MySQLDB), "MySQL database")
	flag.Parse()

	paths := config.Resolve(*root)
	fmt.Println("======== 尼尔号后端 ========")
	fmt.Println("root     :", paths.Root)
	fmt.Println("tables   :", paths.TablesXML)
	fmt.Println("data     :", paths.Data)
	fmt.Println("NieoData :", paths.NieoData)

	host, _ := config.ReadAdvertiseHost(paths.Advertise)
	if host == "" {
		host = "127.0.0.1"
	}
	ipLine := fmt.Sprintf("%s:%d", host, *loginPort)
	if err := config.WriteIP(paths.IPFile, ipLine); err != nil {
		log.Printf("[warn] write ip.txt: %v", err)
	}
	// 同步到仓库 tmp/
	_ = config.WriteIP(filepath.Join(paths.Root, "..", "tmp", "ip.txt"), ipLine)
	fmt.Println("advertise:", host)
	fmt.Println("ip.txt   :", ipLine)

	dsn := store.DSN(*mysqlHost, *mysqlPort, *mysqlUser, *mysqlPass, *mysqlDB)
	jsonDir := filepath.Join(paths.Data, "saves")
	db, backend, err := store.OpenAuto(dsn, jsonDir)
	if err != nil {
		log.Fatalf("[store] open failed (mysql then json): %v", err)
	}
	defer db.Close()
	if backend == "mysql" {
		fmt.Printf("[store] backend=mysql %s@%s:%d/%s\n", *mysqlUser, *mysqlHost, *mysqlPort, *mysqlDB)
	} else {
		fmt.Printf("[store] backend=json dir=%s (MySQL unavailable, fallback)\n", jsonDir)
	}

	cat := tableloader.New(paths.TablesXML)
	if err := cat.Load(); err != nil {
		log.Printf("[warn] 加载静态表失败: %v", err)
	} else {
		items, pets, skills, _ := cat.StatsFull()
		fmt.Printf("[tables] items=%d pets=%d skills=%d\n", items, pets, skills)
	}

	game := gameserver.New(gameserver.Config{
		Port:          *gamePort,
		Store:         db,
		Catalog:       cat,
		DataDir:       paths.Data,
		AdvertiseHost: host,
	})
	if err := game.Start(); err != nil {
		log.Fatalf("[game] %v", err)
	}

	login := loginserver.New(loginserver.Config{
		LoginPort:     *loginPort,
		GamePort:      *gamePort,
		AdvertiseHost: host,
		Store:         db,
		OnAuthed:      game.ForceDisconnect,
	})
	if err := login.Start(); err != nil {
		log.Fatalf("[login] %v", err)
	}

	gmSrv := gm.New(gm.Config{
		Catalog:   cat,
		Store:     db,
		ConfigDir: paths.Configs,
		StaticDir: paths.StaticGM,
		Notify: gm.NotifyFuncs{
			PushItem:     game.PushItemGain,
			PushPet:      game.PushPetGain,
			RefreshPet:   game.PushPetRefresh,
			PushCurrency: game.PushCurrencyBalance,
			Kick:         game.KickUser,
			ListOnline: func() []gm.OnlinePlayer {
				raw := game.ListOnlinePlayers()
				out := make([]gm.OnlinePlayer, len(raw))
				for i, p := range raw {
					out[i] = gm.OnlinePlayer{
						UserID: p.UserID, MapID: p.MapID, Remote: p.Remote, InBattle: p.InBattle,
					}
				}
				return out
			},
		},
		PanelCalc: gameserver.PetPanelSnapshot,
	})
	go func() {
		addr := fmt.Sprintf(":%d", *gmPort)
		fmt.Printf("[gm] http://127.0.0.1%s/  默认账号 admin/admin\n", addr)
		if err := http.ListenAndServe(addr, gmSrv.Mux); err != nil {
			log.Printf("[gm] exit: %v", err)
		}
	}()

	if !*skipRes {
		go func() {
			addr := fmt.Sprintf(":%d", *resPort)
			if err := resserver.ListenAndServe(addr, paths.NieoData, paths.IPFile); err != nil {
				log.Printf("[res] exit: %v", err)
			}
		}()
	}

	fmt.Printf("[ports] GM=%d RES=%d LOGIN=%d GAME=%d\n", *gmPort, *resPort, *loginPort, *gamePort)
	fmt.Println("链路: /ip.txt -> 登录104/(108)/105 -> 游戏1001")
	fmt.Println("日志: [RES]=资源请求  [CMD] OK=已实现  [CMD] UNIMPL=未实现(空回)  [login]/[game]=连接")
	fmt.Println("按 Ctrl+C 结束")
	select {}
}

func findServerRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	if fileExists(filepath.Join(wd, "tables", "xml", "items.xml")) {
		return wd
	}
	if fileExists(filepath.Join(wd, "server", "tables", "xml", "items.xml")) {
		return filepath.Join(wd, "server")
	}
	cand := filepath.Clean(filepath.Join(wd, "..", ".."))
	if fileExists(filepath.Join(cand, "tables", "xml", "items.xml")) {
		return cand
	}
	return wd
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
