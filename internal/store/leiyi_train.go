package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

// LeiyiTrainProgress 雷伊体能特训（CMD 2393）：6 组 today/current/total。
type LeiyiTrainProgress struct {
	Date    int64 `json:"date,omitempty"` // UnixDay = unix/86400
	Today   []int `json:"today,omitempty"`
	Current []int `json:"current,omitempty"`
	Total   []int `json:"total,omitempty"`
}

const leiyiTrainItems = 6

// LeiyiTrainDefaultTotals 面板目标：体力/防御/特防/攻击/特攻/速度。
func LeiyiTrainDefaultTotals() [leiyiTrainItems]int {
	return [leiyiTrainItems]int{60, 30, 20, 20, 10, 20}
}

func (s *sqlBackend) ensureLeiyiTrainSchema() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS user_leiyi_train (
  user_id BIGINT NOT NULL PRIMARY KEY,
  state_json JSON NOT NULL,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	return err
}

func (s *sqlBackend) GetLeiyiTrain(uid int64) (LeiyiTrainProgress, error) {
	_ = s.ensureLeiyiTrainSchema()
	var raw []byte
	err := s.db.QueryRow(`SELECT state_json FROM user_leiyi_train WHERE user_id=?`, uid).Scan(&raw)
	if err == sql.ErrNoRows {
		return LeiyiTrainProgress{}, nil
	}
	if err != nil {
		return LeiyiTrainProgress{}, err
	}
	var st LeiyiTrainProgress
	if err := json.Unmarshal(raw, &st); err != nil {
		return LeiyiTrainProgress{}, nil
	}
	return st, nil
}

func (s *sqlBackend) SetLeiyiTrain(uid int64, st LeiyiTrainProgress) error {
	_ = s.ensureLeiyiTrainSchema()
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
INSERT INTO user_leiyi_train(user_id, state_json) VALUES(?,?)
ON DUPLICATE KEY UPDATE state_json=VALUES(state_json)`, uid, raw)
	return err
}

// NormalizeLeiyiTrain 补齐长度、跨日清 Today、填默认 Total。
func NormalizeLeiyiTrain(st LeiyiTrainProgress, now time.Time) (LeiyiTrainProgress, bool) {
	changed := false
	day := now.Unix() / 86400
	defaults := LeiyiTrainDefaultTotals()
	ensure := func(s []int, fill int) []int {
		if s == nil {
			out := make([]int, leiyiTrainItems)
			for i := 0; i < leiyiTrainItems; i++ {
				out[i] = fill
			}
			changed = true
			return out
		}
		if len(s) < leiyiTrainItems {
			out := make([]int, leiyiTrainItems)
			copy(out, s)
			for i := len(s); i < leiyiTrainItems; i++ {
				out[i] = fill
			}
			changed = true
			return out
		}
		if len(s) > leiyiTrainItems {
			changed = true
			return s[:leiyiTrainItems]
		}
		return s
	}
	if st.Date != day {
		st.Date = day
		st.Today = make([]int, leiyiTrainItems)
		changed = true
	}
	st.Today = ensure(st.Today, 0)
	st.Current = ensure(st.Current, 0)
	if st.Total == nil || len(st.Total) != leiyiTrainItems {
		st.Total = make([]int, leiyiTrainItems)
		copy(st.Total, defaults[:])
		changed = true
	} else {
		for i := 0; i < leiyiTrainItems; i++ {
			if st.Total[i] <= 0 {
				st.Total[i] = defaults[i]
				changed = true
			}
		}
		if st.Total[1] == 20 {
			st.Total[1] = 30
			changed = true
		}
	}
	return st, changed
}
