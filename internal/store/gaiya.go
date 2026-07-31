package store

import "database/sql"

// GetGaiyaEffect 盖亚默认魂印 + 解锁位图（bit0=嗜血 bit1=邪气 bit2=石破）。
func (s *sqlBackend) GetGaiyaEffect(uid int64) (defID, mask int, err error) {
	err = s.db.QueryRow(`
SELECT gaiya_def, gaiya_mask FROM user_progress WHERE user_id=?`, uid).Scan(&defID, &mask)
	if err == sql.ErrNoRows {
		return 0, 0, nil
	}
	return
}

// SetGaiyaEffect 写入盖亚魂印状态。
func (s *sqlBackend) SetGaiyaEffect(uid int64, defID, mask int) error {
	p, err := s.GetProgress(uid)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
INSERT INTO user_progress (user_id, brave_cur, brave_max, fresh_cur, fresh_max, gaiya_def, gaiya_mask)
VALUES (?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE gaiya_def=VALUES(gaiya_def), gaiya_mask=VALUES(gaiya_mask)`,
		uid, p.BraveCur, p.BraveMax, p.FreshCur, p.FreshMax, defID, mask)
	return err
}
