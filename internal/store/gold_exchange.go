package store

import "database/sql"

// GetGoldPromoCount 优惠兑换金豆累计次数（80007/70004）。
func (s *sqlBackend) GetGoldPromoCount(uid int64) (int, error) {
	if err := s.ensureGoldPromoSchema(); err != nil {
		return 0, err
	}
	var n int
	err := s.db.QueryRow(`SELECT gold_promo_n FROM user_progress WHERE user_id=?`, uid).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return n, err
}

// AddGoldPromoCount 优惠兑换次数 +1，返回新值。
func (s *sqlBackend) AddGoldPromoCount(uid int64) (int, error) {
	if err := s.ensureGoldPromoSchema(); err != nil {
		return 0, err
	}
	p, err := s.GetProgress(uid)
	if err != nil {
		return 0, err
	}
	_, err = s.db.Exec(`
INSERT INTO user_progress (user_id, brave_cur, brave_max, fresh_cur, fresh_max, gold_promo_n)
VALUES(?,?,?,?,?,1)
ON DUPLICATE KEY UPDATE gold_promo_n = gold_promo_n + 1`,
		uid, p.BraveCur, p.BraveMax, p.FreshCur, p.FreshMax)
	if err != nil {
		return 0, err
	}
	return s.GetGoldPromoCount(uid)
}

func (s *sqlBackend) ensureGoldPromoSchema() error {
	_, err := s.db.Exec(`ALTER TABLE user_progress ADD COLUMN gold_promo_n INT NOT NULL DEFAULT 0`)
	if err != nil && !isDupColumnErr(err) {
		return err
	}
	return nil
}
