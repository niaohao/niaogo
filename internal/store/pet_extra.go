package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// —— 精灵收集计划 ——

func (s *sqlBackend) ensurePetExtraSchema() error {
	alters := []string{
		`ALTER TABLE user_progress ADD COLUMN pet_collect_mask INT NOT NULL DEFAULT 0`,
		`ALTER TABLE user_progress ADD COLUMN pet_king_301 TINYINT NOT NULL DEFAULT 0`,
		`ALTER TABLE user_progress ADD COLUMN hatch_pet_id INT NOT NULL DEFAULT 0`,
		`ALTER TABLE user_progress ADD COLUMN hatch_item_id INT NOT NULL DEFAULT 0`,
		`ALTER TABLE user_progress ADD COLUMN hatch_start BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE user_progress ADD COLUMN hatch_duration INT NOT NULL DEFAULT 0`,
	}
	for _, q := range alters {
		if _, err := s.db.Exec(q); err != nil && !isDupColumnErr(err) {
			return err
		}
	}
	if _, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS user_soul_beads (
  user_id BIGINT NOT NULL,
  obtain_time BIGINT NOT NULL,
  item_id INT NOT NULL,
  result_pet_id INT NOT NULL DEFAULT 0,
  result_dv INT NOT NULL DEFAULT 0,
  status_json JSON NULL,
  transform_start BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, obtain_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return err
	}
	_, _ = s.db.Exec(`ALTER TABLE user_soul_beads ADD COLUMN result_dv INT NOT NULL DEFAULT 0`)
	_, _ = s.db.Exec(`ALTER TABLE user_soul_beads ADD COLUMN result_nature INT NOT NULL DEFAULT 0`)
	_, _ = s.db.Exec(`ALTER TABLE user_soul_beads ADD COLUMN result_trait INT NOT NULL DEFAULT 0`)
	return nil
}

func (s *sqlBackend) IsPetCollectClaimed(uid int64, period int) (bool, error) {
	if period == 301 {
		var v int
		err := s.db.QueryRow(`SELECT pet_king_301 FROM user_progress WHERE user_id=?`, uid).Scan(&v)
		if err == sql.ErrNoRows {
			return false, nil
		}
		return v != 0, err
	}
	if period < 1 || period > 31 {
		return false, nil
	}
	var mask int
	err := s.db.QueryRow(`SELECT pet_collect_mask FROM user_progress WHERE user_id=?`, uid).Scan(&mask)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return mask&(1<<(period-1)) != 0, nil
}

func (s *sqlBackend) MarkPetCollectClaimed(uid int64, period int) error {
	p, err := s.GetProgress(uid)
	if err != nil {
		return err
	}
	if period == 301 {
		_, err = s.db.Exec(`
INSERT INTO user_progress (user_id, brave_cur, brave_max, fresh_cur, fresh_max, pet_king_301)
VALUES (?,?,?,?,?,1)
ON DUPLICATE KEY UPDATE pet_king_301=1`,
			uid, p.BraveCur, p.BraveMax, p.FreshCur, p.FreshMax)
		return err
	}
	if period < 1 || period > 31 {
		return nil
	}
	bit := 1 << (period - 1)
	_, err = s.db.Exec(`
INSERT INTO user_progress (user_id, brave_cur, brave_max, fresh_cur, fresh_max, pet_collect_mask)
VALUES (?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE pet_collect_mask = pet_collect_mask | VALUES(pet_collect_mask)`,
		uid, p.BraveCur, p.BraveMax, p.FreshCur, p.FreshMax, bit)
	return err
}

// —— 元神珠 ——

	type SoulBead struct {
	ObtainTime     uint32
	ItemID         int
	ResultPetID    int
	ResultDV       int
	ResultNature   int
	ResultTrait    int
	Status         [20]bool
	TransformStart int64
}

func (s *sqlBackend) ListSoulBeads(uid int64) ([]SoulBead, error) {
	rows, err := s.db.Query(`
SELECT obtain_time, item_id, result_pet_id, COALESCE(result_dv,0), COALESCE(result_nature,0), COALESCE(result_trait,0), status_json, transform_start
FROM user_soul_beads WHERE user_id=? ORDER BY obtain_time`, uid)
	if err != nil {
		return s.listSoulBeadsLegacy(uid)
	}
	defer rows.Close()
	out := make([]SoulBead, 0)
	for rows.Next() {
		var b SoulBead
		var raw sql.NullString
		if err := rows.Scan(&b.ObtainTime, &b.ItemID, &b.ResultPetID, &b.ResultDV, &b.ResultNature, &b.ResultTrait, &raw, &b.TransformStart); err != nil {
			return out, err
		}
		if raw.Valid && raw.String != "" {
			var arr []bool
			_ = json.Unmarshal([]byte(raw.String), &arr)
			for i := 0; i < 20 && i < len(arr); i++ {
				b.Status[i] = arr[i]
			}
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *sqlBackend) listSoulBeadsLegacy(uid int64) ([]SoulBead, error) {
	rows, err := s.db.Query(`
SELECT obtain_time, item_id, result_pet_id, COALESCE(result_dv,0), COALESCE(result_nature,0), status_json, transform_start
FROM user_soul_beads WHERE user_id=? ORDER BY obtain_time`, uid)
	if err != nil {
		// 再回落无 nature
		rows2, err2 := s.db.Query(`
SELECT obtain_time, item_id, result_pet_id, COALESCE(result_dv,0), status_json, transform_start
FROM user_soul_beads WHERE user_id=? ORDER BY obtain_time`, uid)
		if err2 != nil {
			return nil, err2
		}
		defer rows2.Close()
		out := make([]SoulBead, 0)
		for rows2.Next() {
			var b SoulBead
			var raw sql.NullString
			if err := rows2.Scan(&b.ObtainTime, &b.ItemID, &b.ResultPetID, &b.ResultDV, &raw, &b.TransformStart); err != nil {
				return out, err
			}
			if raw.Valid && raw.String != "" {
				var arr []bool
				_ = json.Unmarshal([]byte(raw.String), &arr)
				for i := 0; i < 20 && i < len(arr); i++ {
					b.Status[i] = arr[i]
				}
			}
			out = append(out, b)
		}
		return out, rows2.Err()
	}
	defer rows.Close()
	out := make([]SoulBead, 0)
	for rows.Next() {
		var b SoulBead
		var raw sql.NullString
		if err := rows.Scan(&b.ObtainTime, &b.ItemID, &b.ResultPetID, &b.ResultDV, &b.ResultNature, &raw, &b.TransformStart); err != nil {
			return out, err
		}
		if raw.Valid && raw.String != "" {
			var arr []bool
			_ = json.Unmarshal([]byte(raw.String), &arr)
			for i := 0; i < 20 && i < len(arr); i++ {
				b.Status[i] = arr[i]
			}
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *sqlBackend) UpsertSoulBead(uid int64, b SoulBead) error {
	arr := make([]bool, 20)
	copy(arr, b.Status[:])
	raw, _ := json.Marshal(arr)
	_, err := s.db.Exec(`
INSERT INTO user_soul_beads (user_id, obtain_time, item_id, result_pet_id, result_dv, result_nature, result_trait, status_json, transform_start)
VALUES (?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE item_id=VALUES(item_id), result_pet_id=VALUES(result_pet_id),
 result_dv=VALUES(result_dv), result_nature=VALUES(result_nature), result_trait=VALUES(result_trait),
 status_json=VALUES(status_json), transform_start=VALUES(transform_start)`,
		uid, b.ObtainTime, b.ItemID, b.ResultPetID, b.ResultDV, b.ResultNature, b.ResultTrait, string(raw), b.TransformStart)
	if err != nil {
		_, err = s.db.Exec(`
INSERT INTO user_soul_beads (user_id, obtain_time, item_id, result_pet_id, result_dv, result_nature, status_json, transform_start)
VALUES (?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE item_id=VALUES(item_id), result_pet_id=VALUES(result_pet_id),
 result_dv=VALUES(result_dv), result_nature=VALUES(result_nature),
 status_json=VALUES(status_json), transform_start=VALUES(transform_start)`,
			uid, b.ObtainTime, b.ItemID, b.ResultPetID, b.ResultDV, b.ResultNature, string(raw), b.TransformStart)
	}
	return err
}

func (s *sqlBackend) DeleteSoulBead(uid int64, obtainTime uint32) error {
	_, err := s.db.Exec(`DELETE FROM user_soul_beads WHERE user_id=? AND obtain_time=?`, uid, obtainTime)
	return err
}

func (s *sqlBackend) GetSoulBead(uid int64, obtainTime uint32) (*SoulBead, error) {
	list, err := s.ListSoulBeads(uid)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ObtainTime == obtainTime {
			b := list[i]
			return &b, nil
		}
	}
	return nil, nil
}

// —— 分子转化仪孵化态 ——

type HatchState struct {
	PetID     int
	ItemID    int
	StartUnix int64
	Duration  int // 秒
}

func (s *sqlBackend) GetHatchState(uid int64) (HatchState, error) {
	var h HatchState
	err := s.db.QueryRow(`
SELECT hatch_pet_id, hatch_item_id, hatch_start, hatch_duration FROM user_progress WHERE user_id=?`, uid).
		Scan(&h.PetID, &h.ItemID, &h.StartUnix, &h.Duration)
	if err == sql.ErrNoRows {
		return HatchState{}, nil
	}
	return h, err
}

func (s *sqlBackend) SetHatchState(uid int64, h HatchState) error {
	p, err := s.GetProgress(uid)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
INSERT INTO user_progress (user_id, brave_cur, brave_max, fresh_cur, fresh_max,
 hatch_pet_id, hatch_item_id, hatch_start, hatch_duration)
VALUES (?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE hatch_pet_id=VALUES(hatch_pet_id), hatch_item_id=VALUES(hatch_item_id),
 hatch_start=VALUES(hatch_start), hatch_duration=VALUES(hatch_duration)`,
		uid, p.BraveCur, p.BraveMax, p.FreshCur, p.FreshMax,
		h.PetID, h.ItemID, h.StartUnix, h.Duration)
	return err
}

func (s *sqlBackend) ClearHatchState(uid int64) error {
	return s.SetHatchState(uid, HatchState{})
}

// DeletePet 按 catchTime 删除精灵（融合消耗）。
func (s *sqlBackend) DeletePet(uid, catchTime int64) error {
	res, err := s.db.Exec(`DELETE FROM pets WHERE user_id=? AND catch_time=?`, uid, catchTime)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("pet not found")
	}
	return nil
}
