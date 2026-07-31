package store

import "database/sql"

// HasDefeatedSPT 是否已记录击败标记（如谱尼封印 key=30000+region）。
func (s *sqlBackend) HasDefeatedSPT(uid int64, bossKey int) (bool, error) {
	var n int
	err := s.db.QueryRow(`
SELECT 1 FROM user_spt_defeated WHERE user_id=? AND boss_key=? LIMIT 1`, uid, bossKey).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// MarkDefeatedSPT 写入首次击败标记（已存在则忽略）。
func (s *sqlBackend) MarkDefeatedSPT(uid int64, bossKey int) error {
	if uid <= 0 || bossKey == 0 {
		return nil
	}
	_, err := s.db.Exec(`
INSERT IGNORE INTO user_spt_defeated (user_id, boss_key) VALUES (?,?)`, uid, bossKey)
	return err
}
