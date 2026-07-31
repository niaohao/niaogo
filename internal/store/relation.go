package store

import (
	"database/sql"
	"time"
)

// FriendEntry 好友列表项（2150）。
type FriendEntry struct {
	UserID   int64
	TimePoke uint32
}

// BlackEntry 黑名单项（2150）。
type BlackEntry struct {
	UserID int64
}

// ListFriends 返回 user_id 的好友列表。
func (s *sqlBackend) ListFriends(uid int64) ([]FriendEntry, error) {
	rows, err := s.db.Query(`
SELECT friend_id, time_poke FROM user_friends WHERE user_id=? ORDER BY friend_id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FriendEntry
	for rows.Next() {
		var e FriendEntry
		var tp int
		if err := rows.Scan(&e.UserID, &tp); err != nil {
			return nil, err
		}
		e.TimePoke = uint32(tp)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListBlacklist 返回黑名单 user_id 列表。
func (s *sqlBackend) ListBlacklist(uid int64) ([]BlackEntry, error) {
	rows, err := s.db.Query(`
SELECT black_id FROM user_blacklist WHERE user_id=? ORDER BY black_id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BlackEntry
	for rows.Next() {
		var e BlackEntry
		if err := rows.Scan(&e.UserID); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// IsFriend 是否已是好友（单向查表即可，接受时双向写入）。
func (s *sqlBackend) IsFriend(uid, friendID int64) (bool, error) {
	var n int
	err := s.db.QueryRow(`
SELECT COUNT(1) FROM user_friends WHERE user_id=? AND friend_id=?`, uid, friendID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// AddFriend 双向添加好友；已存在则更新 time_poke。
func (s *sqlBackend) AddFriend(uid, friendID int64) error {
	if uid <= 0 || friendID <= 0 || uid == friendID {
		return nil
	}
	tp := int(time.Now().Unix())
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := upsertFriend(tx, uid, friendID, tp); err != nil {
		return err
	}
	if err := upsertFriend(tx, friendID, uid, tp); err != nil {
		return err
	}
	_, _ = tx.Exec(`DELETE FROM user_blacklist WHERE (user_id=? AND black_id=?) OR (user_id=? AND black_id=?)`,
		uid, friendID, friendID, uid)
	return tx.Commit()
}

func upsertFriend(tx *sql.Tx, uid, friendID int64, timePoke int) error {
	_, err := tx.Exec(`
INSERT INTO user_friends (user_id, friend_id, time_poke)
VALUES (?,?,?)
ON DUPLICATE KEY UPDATE time_poke=VALUES(time_poke)`, uid, friendID, timePoke)
	return err
}

// RemoveFriend 删除双向好友关系。
func (s *sqlBackend) RemoveFriend(uid, friendID int64) error {
	if uid <= 0 || friendID <= 0 {
		return nil
	}
	_, err := s.db.Exec(`
DELETE FROM user_friends
WHERE (user_id=? AND friend_id=?) OR (user_id=? AND friend_id=?)`,
		uid, friendID, friendID, uid)
	return err
}

// AddBlacklist 加黑名单并移除双向好友。
func (s *sqlBackend) AddBlacklist(uid, blackID int64) error {
	if uid <= 0 || blackID <= 0 || uid == blackID {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec(`
INSERT IGNORE INTO user_blacklist (user_id, black_id) VALUES (?,?)`, uid, blackID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
DELETE FROM user_friends
WHERE (user_id=? AND friend_id=?) OR (user_id=? AND friend_id=?)`,
		uid, blackID, blackID, uid)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// RemoveBlacklist 移出黑名单。
func (s *sqlBackend) RemoveBlacklist(uid, blackID int64) error {
	if uid <= 0 || blackID <= 0 {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM user_blacklist WHERE user_id=? AND black_id=?`, uid, blackID)
	return err
}
