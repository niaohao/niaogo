package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// NonoVipSignState 超 No 每月签到（9297/9298/9299）。
type NonoVipSignState struct {
	YearMonth string `json:"ym"`      // "2006-01"
	DayMask   uint32 `json:"dayMask"` // bit(day-1)=已签
	BeeTaken  bool   `json:"bee"`
}

func (s *sqlBackend) ensureNonoVipSignSchema() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS user_nono_vip_sign (
  user_id BIGINT NOT NULL PRIMARY KEY,
  state_json JSON NOT NULL,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	return err
}

func (s *sqlBackend) GetNonoVipSign(uid int64) (NonoVipSignState, error) {
	_ = s.ensureNonoVipSignSchema()
	var raw []byte
	err := s.db.QueryRow(`SELECT state_json FROM user_nono_vip_sign WHERE user_id=?`, uid).Scan(&raw)
	if err == sql.ErrNoRows {
		return NonoVipSignState{}, nil
	}
	if err != nil {
		return NonoVipSignState{}, err
	}
	var st NonoVipSignState
	if err := json.Unmarshal(raw, &st); err != nil {
		return NonoVipSignState{}, nil
	}
	return st, nil
}

func (s *sqlBackend) SetNonoVipSign(uid int64, st NonoVipSignState) error {
	_ = s.ensureNonoVipSignSchema()
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
INSERT INTO user_nono_vip_sign(user_id, state_json) VALUES(?,?)
ON DUPLICATE KEY UPDATE state_json=VALUES(state_json)`, uid, raw)
	return err
}

// NonoVipSignYM 当前月键（东八区）。
func NonoVipSignYM(now time.Time) string {
	loc := chinaLoc()
	t := now.In(loc)
	return fmt.Sprintf("%04d-%02d", t.Year(), int(t.Month()))
}

func chinaLoc() *time.Location {
	if loc, err := time.LoadLocation("Asia/Shanghai"); err == nil {
		return loc
	}
	return time.FixedZone("CST", 8*3600)
}

// NormalizeNonoVipSign 跨月清零。
func NormalizeNonoVipSign(st NonoVipSignState, now time.Time) NonoVipSignState {
	ym := NonoVipSignYM(now)
	if st.YearMonth != ym {
		return NonoVipSignState{YearMonth: ym}
	}
	return st
}

func (st NonoVipSignState) SignedCount() int {
	n := 0
	m := st.DayMask
	for m != 0 {
		n += int(m & 1)
		m >>= 1
	}
	return n
}

func (st NonoVipSignState) HasDay(day int) bool {
	if day < 1 || day > 31 {
		return false
	}
	return st.DayMask&(1<<uint(day-1)) != 0
}

func (st *NonoVipSignState) SetDay(day int) {
	if day < 1 || day > 31 {
		return
	}
	st.DayMask |= 1 << uint(day-1)
}
