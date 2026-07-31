package store

import (
	"database/sql"
	"encoding/json"
)

// RoomPets 基地房间展示中的精灵 catchTime 列表（最多 5 只）。
type RoomPets []int64

func (s *sqlBackend) ensureRoomPetsSchema() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS user_room_pets (
  user_id BIGINT NOT NULL PRIMARY KEY,
  catch_times_json JSON NOT NULL,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	return err
}

func (s *sqlBackend) GetRoomPets(uid int64) (RoomPets, error) {
	_ = s.ensureRoomPetsSchema()
	var raw []byte
	err := s.db.QueryRow(`SELECT catch_times_json FROM user_room_pets WHERE user_id=?`, uid).Scan(&raw)
	if err == sql.ErrNoRows {
		return RoomPets{}, nil
	}
	if err != nil {
		return nil, err
	}
	var list RoomPets
	if err := json.Unmarshal(raw, &list); err != nil {
		return RoomPets{}, nil
	}
	return sanitizeRoomPets(list), nil
}

func (s *sqlBackend) SetRoomPets(uid int64, list RoomPets) error {
	_ = s.ensureRoomPetsSchema()
	list = sanitizeRoomPets(list)
	raw, err := json.Marshal(list)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
INSERT INTO user_room_pets(user_id, catch_times_json) VALUES(?,?)
ON DUPLICATE KEY UPDATE catch_times_json=VALUES(catch_times_json)`, uid, raw)
	return err
}

func sanitizeRoomPets(list RoomPets) RoomPets {
	out := make(RoomPets, 0, 5)
	seen := map[int64]struct{}{}
	for _, ct := range list {
		if ct <= 0 {
			continue
		}
		if _, ok := seen[ct]; ok {
			continue
		}
		seen[ct] = struct{}{}
		out = append(out, ct)
		if len(out) >= 5 {
			break
		}
	}
	return out
}
