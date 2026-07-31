package store

import "database/sql"

// GetRecruitClaimMask 招募官奖励已领位图（bit0=槽1 … bit3=槽4）。
func (s *sqlBackend) GetRecruitClaimMask(uid int64) (uint32, error) {
	var mask sql.NullInt64
	err := s.db.QueryRow(`SELECT recruit_mask FROM user_progress WHERE user_id=?`, uid).Scan(&mask)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !mask.Valid || mask.Int64 < 0 {
		return 0, nil
	}
	return uint32(mask.Int64), nil
}

// SetRecruitClaimMask 写入招募已领位图（保留塔进度）。
func (s *sqlBackend) SetRecruitClaimMask(uid int64, mask uint32) error {
	p, err := s.GetProgress(uid)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
INSERT INTO user_progress (user_id, brave_cur, brave_max, fresh_cur, fresh_max, recruit_mask)
VALUES (?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE recruit_mask=VALUES(recruit_mask)`,
		uid, p.BraveCur, p.BraveMax, p.FreshCur, p.FreshMax, mask)
	return err
}

// ClaimRecruitSlot 标记槽位已领；若已领返回 claimed=true。
func (s *sqlBackend) ClaimRecruitSlot(uid int64, slot uint32) (already bool, mask uint32, err error) {
	mask, err = s.GetRecruitClaimMask(uid)
	if err != nil {
		return false, 0, err
	}
	if slot < 1 || slot > 4 {
		return false, mask, nil
	}
	bit := uint32(1) << (slot - 1)
	if mask&bit != 0 {
		return true, mask, nil
	}
	mask |= bit
	if err = s.SetRecruitClaimMask(uid, mask); err != nil {
		return false, mask, err
	}
	return false, mask, nil
}
