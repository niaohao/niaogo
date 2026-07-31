package store

import "database/sql"

// BoostTimes 加速器 / 能量 / 学习力双倍仪 / 自动战斗器剩余次数与开关。
type BoostTimes struct {
	TwoTimes       int
	ThreeTimes     int
	AutoFight      int // 0关 1有装置未开 3开启
	AutoFightTimes int
	EnergyTimes    int
	LearnTimes     int
}

const boostTimesMax = 99999

func clampBoost(n int) int {
	if n < 0 {
		return 0
	}
	if n > boostTimesMax {
		return boostTimesMax
	}
	return n
}

// GetBoostTimes 读剩余次数；无记录全 0。
func (s *sqlBackend) GetBoostTimes(uid int64) (BoostTimes, error) {
	var t BoostTimes
	err := s.db.QueryRow(`
SELECT COALESCE(two_times,0), COALESCE(three_times,0), COALESCE(auto_fight,0),
       COALESCE(auto_fight_times,0), COALESCE(energy_times,0), COALESCE(learn_times,0)
FROM user_progress WHERE user_id=?`, uid).Scan(
		&t.TwoTimes, &t.ThreeTimes, &t.AutoFight, &t.AutoFightTimes, &t.EnergyTimes, &t.LearnTimes)
	if err == sql.ErrNoRows {
		return BoostTimes{}, nil
	}
	return t, err
}

func (s *sqlBackend) ensureProgressRow(uid int64) error {
	p, err := s.GetProgress(uid)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
INSERT INTO user_progress (user_id, brave_cur, brave_max, fresh_cur, fresh_max)
VALUES (?,?,?,?,?)
ON DUPLICATE KEY UPDATE user_id=user_id`,
		uid, p.BraveCur, p.BraveMax, p.FreshCur, p.FreshMax)
	return err
}

func (s *sqlBackend) setBoostCol(uid int64, col string, n int) error {
	n = clampBoost(n)
	if err := s.ensureProgressRow(uid); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE user_progress SET `+col+`=? WHERE user_id=?`, n, uid)
	return err
}

func (s *sqlBackend) SetLearnTimes(uid int64, n int) error {
	return s.setBoostCol(uid, "learn_times", n)
}

func (s *sqlBackend) SetTwoTimes(uid int64, n int) error {
	return s.setBoostCol(uid, "two_times", n)
}

func (s *sqlBackend) SetThreeTimes(uid int64, n int) error {
	return s.setBoostCol(uid, "three_times", n)
}

func (s *sqlBackend) SetEnergyTimes(uid int64, n int) error {
	return s.setBoostCol(uid, "energy_times", n)
}

func (s *sqlBackend) SetAutoFightTimes(uid int64, n int) error {
	return s.setBoostCol(uid, "auto_fight_times", n)
}

func (s *sqlBackend) SetAutoFight(uid int64, n int) error {
	if n < 0 {
		n = 0
	}
	if n > 3 {
		n = 3
	}
	if err := s.ensureProgressRow(uid); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE user_progress SET auto_fight=? WHERE user_id=?`, n, uid)
	return err
}

func (s *sqlBackend) AddLearnTimes(uid int64, delta int) (int, error) {
	t, err := s.GetBoostTimes(uid)
	if err != nil {
		return 0, err
	}
	n := clampBoost(t.LearnTimes + delta)
	if err := s.SetLearnTimes(uid, n); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *sqlBackend) AddTwoTimes(uid int64, delta int) (int, error) {
	t, err := s.GetBoostTimes(uid)
	if err != nil {
		return 0, err
	}
	n := clampBoost(t.TwoTimes + delta)
	if err := s.SetTwoTimes(uid, n); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *sqlBackend) AddThreeTimes(uid int64, delta int) (int, error) {
	t, err := s.GetBoostTimes(uid)
	if err != nil {
		return 0, err
	}
	n := clampBoost(t.ThreeTimes + delta)
	if err := s.SetThreeTimes(uid, n); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *sqlBackend) AddEnergyTimes(uid int64, delta int) (int, error) {
	t, err := s.GetBoostTimes(uid)
	if err != nil {
		return 0, err
	}
	n := clampBoost(t.EnergyTimes + delta)
	if err := s.SetEnergyTimes(uid, n); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *sqlBackend) AddAutoFightTimes(uid int64, delta int) (int, error) {
	t, err := s.GetBoostTimes(uid)
	if err != nil {
		return 0, err
	}
	n := clampBoost(t.AutoFightTimes + delta)
	if err := s.SetAutoFightTimes(uid, n); err != nil {
		return 0, err
	}
	return n, nil
}

// ConsumeLearnTimes 战后触发双倍时扣 n；不足则返回 false。
func (s *sqlBackend) ConsumeLearnTimes(uid int64, n int) (ok bool, left int, err error) {
	if n <= 0 {
		t, e := s.GetBoostTimes(uid)
		return true, t.LearnTimes, e
	}
	t, err := s.GetBoostTimes(uid)
	if err != nil {
		return false, 0, err
	}
	if t.LearnTimes < n {
		return false, t.LearnTimes, nil
	}
	left = t.LearnTimes - n
	if err := s.SetLearnTimes(uid, left); err != nil {
		return false, t.LearnTimes, err
	}
	return true, left, nil
}

func (s *sqlBackend) ConsumeTwoTimes(uid int64, n int) (ok bool, left int, err error) {
	t, err := s.GetBoostTimes(uid)
	if err != nil {
		return false, 0, err
	}
	if t.TwoTimes < n {
		return false, t.TwoTimes, nil
	}
	left = t.TwoTimes - n
	if err := s.SetTwoTimes(uid, left); err != nil {
		return false, t.TwoTimes, err
	}
	return true, left, nil
}

func (s *sqlBackend) ConsumeThreeTimes(uid int64, n int) (ok bool, left int, err error) {
	t, err := s.GetBoostTimes(uid)
	if err != nil {
		return false, 0, err
	}
	if t.ThreeTimes < n {
		return false, t.ThreeTimes, nil
	}
	left = t.ThreeTimes - n
	if err := s.SetThreeTimes(uid, left); err != nil {
		return false, t.ThreeTimes, err
	}
	return true, left, nil
}

func (s *sqlBackend) ConsumeAutoFightTimes(uid int64, n int) (ok bool, left int, err error) {
	t, err := s.GetBoostTimes(uid)
	if err != nil {
		return false, 0, err
	}
	if t.AutoFightTimes < n {
		return false, t.AutoFightTimes, nil
	}
	left = t.AutoFightTimes - n
	if err := s.SetAutoFightTimes(uid, left); err != nil {
		return false, t.AutoFightTimes, err
	}
	if left == 0 {
		_ = s.SetAutoFight(uid, 0)
	}
	return true, left, nil
}
