package store

import "database/sql"

// GetEquipLevel 装备强化等级；无记录返回 0。
func (s *sqlBackend) GetEquipLevel(uid int64, itemID int) (int, error) {
	var lv int
	err := s.db.QueryRow(`SELECT enhance_lv FROM equips WHERE user_id=? AND item_id=?`, uid, itemID).Scan(&lv)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lv, err
}

// EnsureEquip 拥有可强化装备时保证 equips 行存在（默认 lv）。
func (s *sqlBackend) EnsureEquip(uid int64, itemID, level int) error {
	if itemID <= 0 {
		return nil
	}
	if level < 1 {
		level = 1
	}
	_, err := s.db.Exec(`
INSERT INTO equips (user_id, item_id, count, enhance_lv)
VALUES (?,?,1,?)
ON DUPLICATE KEY UPDATE count=GREATEST(count,1)`,
		uid, itemID, level)
	return err
}

// SetEquipLevel 设置强化等级。
func (s *sqlBackend) SetEquipLevel(uid int64, itemID, level int) error {
	if itemID <= 0 || level < 0 {
		return nil
	}
	_, err := s.db.Exec(`
INSERT INTO equips (user_id, item_id, count, enhance_lv)
VALUES (?,?,1,?)
ON DUPLICATE KEY UPDATE enhance_lv=VALUES(enhance_lv), count=GREATEST(count,1)`,
		uid, itemID, level)
	return err
}

// ListEquipLevels 返回用户全部装备等级 map[itemID]level。
func (s *sqlBackend) ListEquipLevels(uid int64) (map[int]int, error) {
	rows, err := s.db.Query(`SELECT item_id, enhance_lv FROM equips WHERE user_id=? AND enhance_lv>0`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int]int)
	for rows.Next() {
		var id, lv int
		if err := rows.Scan(&id, &lv); err != nil {
			return nil, err
		}
		out[id] = lv
	}
	return out, rows.Err()
}
