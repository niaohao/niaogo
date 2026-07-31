package store

import (
	"database/sql"
)

// GetExpPool 读经验分配器（积累经验）。
func (s *sqlBackend) GetExpPool(uid int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COALESCE(exp_pool,0) FROM users WHERE user_id=?`, uid).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return n, err
}

// AddExpPool 增减积累经验；结果不会小于 0。
func (s *sqlBackend) AddExpPool(uid int64, delta int) (int, error) {
	if delta == 0 {
		return s.GetExpPool(uid)
	}
	_, err := s.db.Exec(`
UPDATE users SET exp_pool = GREATEST(0, COALESCE(exp_pool,0) + ?) WHERE user_id=?`, delta, uid)
	if err != nil {
		return 0, err
	}
	return s.GetExpPool(uid)
}

// SetExpPool 设置积累经验（≥0）。
func (s *sqlBackend) SetExpPool(uid int64, value int) error {
	if value < 0 {
		value = 0
	}
	_, err := s.db.Exec(`UPDATE users SET exp_pool=? WHERE user_id=?`, value, uid)
	return err
}
