package main

// 一次性：按新 DefaultSkillsAtLevel 重算指定账号全部精灵技能栏。
// go run ./scripts/fix_pet_skills.go 50002 50003

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	_ "github.com/go-sql-driver/mysql"

	"niaohao/server/internal/defaults"
	"niaohao/server/internal/store"
	"niaohao/server/internal/tableloader"
)

func main() {
	uids := make([]int64, 0, len(os.Args)-1)
	for _, a := range os.Args[1:] {
		n, err := strconv.ParseInt(a, 10, 64)
		if err != nil || n <= 0 {
			fmt.Fprintf(os.Stderr, "bad uid %q\n", a)
			os.Exit(1)
		}
		uids = append(uids, n)
	}
	if len(uids) == 0 {
		uids = []int64{50002, 50003}
	}

	root, _ := os.Getwd()
	xmlDir := filepath.Join(root, "tables", "xml")
	if _, err := os.Stat(xmlDir); err != nil {
		xmlDir = filepath.Join(root, "server", "tables", "xml")
	}
	cat := tableloader.New(xmlDir)
	if err := cat.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "load tables: %v\n", err)
		os.Exit(1)
	}

	dsn := store.DSN(defaults.MySQLHost, defaults.MySQLPort, defaults.MySQLUser, defaults.MySQLPass, defaults.MySQLDB)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mysql: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	for _, uid := range uids {
		rows, err := db.Query(`SELECT catch_time, pet_id, level FROM pets WHERE user_id=?`, uid)
		if err != nil {
			fmt.Fprintf(os.Stderr, "query %d: %v\n", uid, err)
			os.Exit(1)
		}
		type row struct {
			ct    int64
			petID int
			level int
		}
		var list []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.ct, &r.petID, &r.level); err != nil {
				rows.Close()
				fmt.Fprintf(os.Stderr, "scan: %v\n", err)
				os.Exit(1)
			}
			list = append(list, r)
		}
		rows.Close()

		updated := 0
		for _, r := range list {
			skills := cat.DefaultSkillsAtLevel(r.petID, r.level)
			raw, _ := json.Marshal(skills)
			res, err := db.Exec(`UPDATE pets SET skills_json=? WHERE user_id=? AND catch_time=?`, string(raw), uid, r.ct)
			if err != nil {
				fmt.Fprintf(os.Stderr, "update uid=%d ct=%d: %v\n", uid, r.ct, err)
				os.Exit(1)
			}
			n, _ := res.RowsAffected()
			updated += int(n)
		}
		fmt.Printf("uid=%d pets=%d updated=%d\n", uid, len(list), updated)
	}
}
