package store

import "database/sql"

// UserProgress 勇者之塔 / 试炼塔进度（1001 登录包 + 2414/2428）。
type UserProgress struct {
	BraveCur  int
	BraveMax  int
	FreshCur  int
	FreshMax  int
}

// GetProgress 读塔进度；无记录时返回 cur=max=1。
func (s *sqlBackend) GetProgress(uid int64) (UserProgress, error) {
	var p UserProgress
	err := s.db.QueryRow(`
SELECT brave_cur, brave_max, fresh_cur, fresh_max FROM user_progress WHERE user_id=?`, uid).Scan(
		&p.BraveCur, &p.BraveMax, &p.FreshCur, &p.FreshMax)
	if err == sql.ErrNoRows {
		return UserProgress{BraveCur: 1, BraveMax: 1, FreshCur: 1, FreshMax: 1}, nil
	}
	if err != nil {
		return p, err
	}
	if p.BraveCur < 1 {
		p.BraveCur = 1
	}
	if p.BraveMax < p.BraveCur {
		p.BraveMax = p.BraveCur
	}
	if p.FreshCur < 1 {
		p.FreshCur = 1
	}
	if p.FreshMax < p.FreshCur {
		p.FreshMax = p.FreshCur
	}
	return p, nil
}

func (s *sqlBackend) upsertProgress(uid int64, p UserProgress) error {
	_, err := s.db.Exec(`
INSERT INTO user_progress (user_id, brave_cur, brave_max, fresh_cur, fresh_max)
VALUES (?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  brave_cur=VALUES(brave_cur), brave_max=VALUES(brave_max),
  fresh_cur=VALUES(fresh_cur), fresh_max=VALUES(fresh_max)`,
		uid, p.BraveCur, p.BraveMax, p.FreshCur, p.FreshMax)
	return err
}

// SetBraveProgress 更新勇者之塔层数。
func (s *sqlBackend) SetBraveProgress(uid int64, cur int) error {
	p, err := s.GetProgress(uid)
	if err != nil {
		return err
	}
	if cur < 1 {
		cur = 1
	}
	p.BraveCur = cur
	if cur > p.BraveMax {
		p.BraveMax = cur
	}
	return s.upsertProgress(uid, p)
}

// SetFreshProgress 更新试炼塔层数。
func (s *sqlBackend) SetFreshProgress(uid int64, cur int) error {
	p, err := s.GetProgress(uid)
	if err != nil {
		return err
	}
	if cur < 1 {
		cur = 1
	}
	p.FreshCur = cur
	if cur > p.FreshMax {
		p.FreshMax = cur
	}
	return s.upsertProgress(uid, p)
}
