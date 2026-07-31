package store

import "database/sql"

// Fitment 小屋已摆放家具（对齐前端 FitmentInfo / 原码 userdb.Fitment）。
type Fitment struct {
	ID     int
	X      int
	Y      int
	Dir    int
	Status int
}

func (s *sqlBackend) ensureFitmentSchema() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS user_fitments (
  user_id BIGINT NOT NULL,
  slot INT NOT NULL,
  item_id INT NOT NULL,
  pos_x INT NOT NULL DEFAULT 0,
  pos_y INT NOT NULL DEFAULT 0,
  dir INT NOT NULL DEFAULT 0,
  status INT NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, slot),
  INDEX idx_fitment_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	return err
}

func (s *sqlBackend) ListFitments(uid int64) ([]Fitment, error) {
	_ = s.ensureFitmentSchema()
	rows, err := s.db.Query(`
SELECT item_id, pos_x, pos_y, dir, status FROM user_fitments
WHERE user_id=? ORDER BY slot`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Fitment, 0)
	for rows.Next() {
		var f Fitment
		if err := rows.Scan(&f.ID, &f.X, &f.Y, &f.Dir, &f.Status); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ReplaceFitments 整表替换用户摆放布局。
func (s *sqlBackend) ReplaceFitments(uid int64, list []Fitment) error {
	_ = s.ensureFitmentSchema()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM user_fitments WHERE user_id=?`, uid); err != nil {
		return err
	}
	for i, f := range list {
		if f.ID <= 0 {
			continue
		}
		if _, err := tx.Exec(`
INSERT INTO user_fitments(user_id, slot, item_id, pos_x, pos_y, dir, status)
VALUES(?,?,?,?,?,?,?)`, uid, i, f.ID, f.X, f.Y, f.Dir, f.Status); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListFitmentItems 仓库中的基地家具（items 表 item_id 500001–599999）。
func (s *sqlBackend) ListFitmentItems(uid int64) ([]Item, error) {
	rows, err := s.db.Query(`
SELECT item_id, count, expire_time FROM items
WHERE user_id=? AND count>0 AND item_id>=500001 AND item_id<=599999
ORDER BY item_id`, uid)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	out := make([]Item, 0)
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ItemID, &it.Count, &it.ExpireTime); err != nil {
			return nil, err
		}
		if it.ExpireTime == 0 {
			it.ExpireTime = defaultItemExpire
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
