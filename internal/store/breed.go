package store

import (
	"database/sql"
	"encoding/json"
)

// BreedEggEntry 培育仓蛋槽。
type BreedEggEntry struct {
	EggID        int   `json:"eggId"`
	EggCatchTime int64 `json:"eggCatchTime"`
}

// BreedState 精灵培育仓面板状态（独立于分子转化仪 HatchState）。
type BreedState struct {
	MalePetID         int             `json:"malePetId"`
	MaleCatchTime     int64           `json:"maleCatchTime"`
	FemalePetID       int             `json:"femalePetId"`
	FemaleCatchTime   int64           `json:"femaleCatchTime"`
	EggID             int             `json:"eggId"`
	EggCatchTime      int64           `json:"eggCatchTime"`
	HatchState        int             `json:"hatchState"` // 0 无 / 1 孵化中 / 2 可领取
	HatchLeftTime     int64           `json:"hatchLeftTime"`
	Intimacy          int             `json:"intimacy"` // 1..5
	Eggs              []BreedEggEntry `json:"eggs"`
}

func (s *sqlBackend) ensureBreedSchema() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS user_breed (
  user_id BIGINT NOT NULL PRIMARY KEY,
  state_json JSON NOT NULL,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	return err
}

func (s *sqlBackend) GetBreedState(uid int64) (BreedState, error) {
	_ = s.ensureBreedSchema()
	var raw []byte
	err := s.db.QueryRow(`SELECT state_json FROM user_breed WHERE user_id=?`, uid).Scan(&raw)
	if err == sql.ErrNoRows {
		return BreedState{Intimacy: 1}, nil
	}
	if err != nil {
		return BreedState{Intimacy: 1}, err
	}
	var st BreedState
	if err := json.Unmarshal(raw, &st); err != nil {
		return BreedState{Intimacy: 1}, err
	}
	if st.Intimacy < 1 {
		st.Intimacy = 1
	}
	return st, nil
}

func (s *sqlBackend) SetBreedState(uid int64, st BreedState) error {
	_ = s.ensureBreedSchema()
	if st.Intimacy < 1 {
		st.Intimacy = 1
	}
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
INSERT INTO user_breed(user_id, state_json) VALUES(?,?)
ON DUPLICATE KEY UPDATE state_json=VALUES(state_json)`, uid, raw)
	return err
}
