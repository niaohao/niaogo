package store

// WornCloth 当前穿戴装扮（登录 1001 / 2604）。
type WornCloth struct {
	SlotIdx int
	ItemID  int
	Level   int
}

// ListWornClothes 按 slot_idx 顺序返回已穿戴装扮。
func (s *sqlBackend) ListWornClothes(uid int64) ([]WornCloth, error) {
	rows, err := s.db.Query(`
SELECT slot_idx, item_id, level FROM user_worn_clothes WHERE user_id=? ORDER BY slot_idx`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WornCloth
	for rows.Next() {
		var w WornCloth
		if err := rows.Scan(&w.SlotIdx, &w.ItemID, &w.Level); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// SetWornClothes 覆盖当前穿戴（先清后写）。
func (s *sqlBackend) SetWornClothes(uid int64, clothes []WornCloth) error {
	if uid <= 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM user_worn_clothes WHERE user_id=?`, uid); err != nil {
		return err
	}
	for i, w := range clothes {
		if w.ItemID <= 0 {
			continue
		}
		if _, err := tx.Exec(`
INSERT INTO user_worn_clothes (user_id, slot_idx, item_id, level) VALUES (?,?,?,?)`,
			uid, i+1, w.ItemID, w.Level); err != nil {
			return err
		}
	}
	return tx.Commit()
}
