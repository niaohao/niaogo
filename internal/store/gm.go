package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// UserBrief GM 列表用简要信息。
type UserBrief struct {
	UserID   int64  `json:"userId"`
	Nickname string `json:"nickname"`
	Email    string `json:"email"`
	Coins    int    `json:"coins"`
	Gold     int    `json:"gold"`
	MapID    int    `json:"mapId"`
}

// SearchUsers 按 UID / 昵称 / 邮箱模糊搜；q 为空则最近登录若干。
func (s *sqlBackend) SearchUsers(q string, limit int) ([]UserBrief, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	q = strings.TrimSpace(q)
	var (
		rows *sql.Rows
		err  error
	)
	if q == "" {
		rows, err = s.db.Query(`
SELECT user_id, nickname, email, coins, gold, map_id
FROM users ORDER BY last_online DESC, user_id DESC LIMIT ?`, limit)
	} else if uid, ok := parsePositiveInt64(q); ok {
		rows, err = s.db.Query(`
SELECT user_id, nickname, email, coins, gold, map_id
FROM users WHERE user_id=? OR nickname LIKE ? OR email LIKE ?
ORDER BY user_id LIMIT ?`, uid, "%"+q+"%", "%"+q+"%", limit)
	} else {
		like := "%" + q + "%"
		rows, err = s.db.Query(`
SELECT user_id, nickname, email, coins, gold, map_id
FROM users WHERE nickname LIKE ? OR email LIKE ?
ORDER BY user_id LIMIT ?`, like, like, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]UserBrief, 0)
	for rows.Next() {
		var u UserBrief
		if err := rows.Scan(&u.UserID, &u.Nickname, &u.Email, &u.Coins, &u.Gold, &u.MapID); err != nil {
			return out, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func parsePositiveInt64(s string) (int64, bool) {
	var n int64
	if s == "" {
		return 0, false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int64(c-'0')
		if n > 1e15 {
			return 0, false
		}
	}
	return n, true
}

// InsertGMAudit 写入 GM 操作审计。
func (s *sqlBackend) InsertGMAudit(admin, action string, targetUID int64, detail any) (int64, error) {
	if admin == "" {
		admin = "gm"
	}
	var raw []byte
	if detail != nil {
		var err error
		raw, err = json.Marshal(detail)
		if err != nil {
			return 0, err
		}
	}
	res, err := s.db.Exec(`
INSERT INTO gm_audit (admin, action, target_user, detail_json) VALUES (?,?,?,?)`,
		admin, action, targetUID, nullJSON(raw))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func nullJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}

// ListGMAudit 最近审计。
func (s *sqlBackend) ListGMAudit(limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(`
SELECT id, admin, action, target_user, detail_json, UNIX_TIMESTAMP(created_at)
FROM gm_audit ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, target, ts int64
		var admin, action string
		var detail *string
		if err := rows.Scan(&id, &admin, &action, &target, &detail, &ts); err != nil {
			return out, err
		}
		m := map[string]any{
			"id": id, "admin": admin, "action": action,
			"targetUser": target, "createdAt": ts,
		}
		if detail != nil {
			m["detail"] = json.RawMessage(*detail)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AddGoldWithLedger 增减金豆并记流水。
func (s *sqlBackend) AddGoldWithLedger(uid int64, delta int, reason, ref string) (balance int, err error) {
	if delta == 0 {
		return s.getGold(uid)
	}
	if err = s.AddGold(uid, delta); err != nil {
		return 0, err
	}
	balance, err = s.getGold(uid)
	if err != nil {
		return 0, err
	}
	_, err = s.db.Exec(`
INSERT INTO gold_ledger (user_id, delta, balance_after, reason, ref_id) VALUES (?,?,?,?,?)`,
		uid, delta, balance, reason, ref)
	return balance, err
}

func (s *sqlBackend) getGold(uid int64) (int, error) {
	var g int
	err := s.db.QueryRow(`SELECT gold FROM users WHERE user_id=?`, uid).Scan(&g)
	return g, err
}

// GrantPet 发放一只精灵；背包满则进仓库。返回 catchTime。
func (s *sqlBackend) GrantPet(uid int64, petID int, name string, level, dv, nature int, skills []int) (catchTime int64, err error) {
	if petID <= 0 {
		return 0, fmt.Errorf("bad petId")
	}
	if level <= 0 {
		level = 1
	}
	if name == "" {
		name = "精灵"
	}
	if skills == nil {
		skills = []int{10001}
	}
	catchTime = time.Now().Unix()
	// 避免同秒冲突
	for i := 0; i < 5; i++ {
		if existing, _ := s.GetPetByCatchTime(uid, catchTime); existing == nil {
			break
		}
		catchTime++
	}
	p := &Pet{
		UserID:    uid,
		CatchTime: catchTime,
		PetID:     petID,
		Name:      name,
		Level:     level,
		DV:        dv,
		Nature:    nature,
		InBag:     true,
		BagPos:   99,
		Skills:    skills,
	}
	n, _ := s.CountBagPets(uid)
	if n >= MaxBagPets {
		p.InBag = false
		p.BagPos = -1
	}
	if err = s.UpsertPet(p); err != nil {
		return 0, err
	}
	return catchTime, nil
}
