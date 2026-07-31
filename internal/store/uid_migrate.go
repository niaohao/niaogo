package store

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
)

// MinUserID 客户端 MapManager.ID_MAX=49999：回基地 changeMap(actorID)，
// 仅当 userID>49999 才走房型 styleID（500001.swf）；否则会误拉 map/{uid}.swf。
const MinUserID int64 = 50000

// lowUIDBump 将历史小号 UID 抬到基地可用区间：10002 → 50002。
const lowUIDBump int64 = 40000

func remapLowUID(uid int64) int64 {
	if uid <= 0 || uid > 49999 {
		return uid
	}
	return uid + lowUIDBump
}

func (s *sqlBackend) migrateLowUserIDs() error {
	rows, err := s.db.Query(`SELECT user_id FROM users WHERE user_id<=49999 ORDER BY user_id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	olds := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		olds = append(olds, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(olds) == 0 {
		return nil
	}

	for _, oldID := range olds {
		newID := remapLowUID(oldID)
		if newID == oldID {
			continue
		}
		var exists int
		_ = s.db.QueryRow(`SELECT COUNT(1) FROM users WHERE user_id=?`, newID).Scan(&exists)
		if exists > 0 {
			log.Printf("[store] skip UID remap %d→%d (target exists)", oldID, newID)
			continue
		}
		if err := s.remapUserIDMySQL(oldID, newID); err != nil {
			return fmt.Errorf("remap %d→%d: %w", oldID, newID, err)
		}
		log.Printf("[store] remapped UID %d→%d (基地需 userID>49999)", oldID, newID)
	}
	return nil
}

func (s *sqlBackend) remapUserIDMySQL(oldID, newID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// 子表 user_id
	for _, q := range []string{
		`UPDATE pets SET user_id=? WHERE user_id=?`,
		`UPDATE user_tasks SET user_id=? WHERE user_id=?`,
		`UPDATE items SET user_id=? WHERE user_id=?`,
		`UPDATE props SET user_id=? WHERE user_id=?`,
		`UPDATE exp_ledger SET user_id=? WHERE user_id=?`,
		`UPDATE gold_ledger SET user_id=? WHERE user_id=?`,
		`UPDATE equips SET user_id=? WHERE user_id=?`,
		`UPDATE essences SET user_id=? WHERE user_id=?`,
		`UPDATE user_worn_clothes SET user_id=? WHERE user_id=?`,
		`UPDATE user_mails SET user_id=? WHERE user_id=?`,
		`UPDATE user_progress SET user_id=? WHERE user_id=?`,
		`UPDATE user_spt_defeated SET user_id=? WHERE user_id=?`,
		`UPDATE user_nono SET user_id=? WHERE user_id=?`,
		`UPDATE user_fitments SET user_id=? WHERE user_id=?`,
		`UPDATE user_breed SET user_id=? WHERE user_id=?`,
		`UPDATE user_room_pets SET user_id=? WHERE user_id=?`,
		`UPDATE user_nono_vip_sign SET user_id=? WHERE user_id=?`,
		`UPDATE user_ops_state SET user_id=? WHERE user_id=?`,
		`UPDATE user_achieve_rules SET user_id=? WHERE user_id=?`,
		`UPDATE user_achieve_branch SET user_id=? WHERE user_id=?`,
		`UPDATE user_titles SET user_id=? WHERE user_id=?`,
		`UPDATE user_soul_beads SET user_id=? WHERE user_id=?`,
		`UPDATE user_friends SET user_id=? WHERE user_id=?`,
		`UPDATE user_blacklist SET user_id=? WHERE user_id=?`,
	} {
		if _, err := tx.Exec(q, newID, oldID); err != nil {
			// 表可能尚未创建
			continue
		}
	}
	// 双向关系另一侧
	_, _ = tx.Exec(`UPDATE user_friends SET friend_id=? WHERE friend_id=?`, newID, oldID)
	_, _ = tx.Exec(`UPDATE user_blacklist SET black_id=? WHERE black_id=?`, newID, oldID)
	_, _ = tx.Exec(`UPDATE user_mails SET from_id=? WHERE from_id=?`, newID, oldID)
	// 主表最后
	if _, err := tx.Exec(`UPDATE users SET user_id=? WHERE user_id=?`, newID, oldID); err != nil {
		return err
	}
	// map_id 若误存成旧 uid（基地图）一并抬升
	_, _ = tx.Exec(`UPDATE users SET map_id=? WHERE user_id=? AND map_id=?`, newID, newID, oldID)
	return tx.Commit()
}

func (s *jsonStore) migrateLowUserIDs() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(filepath.Join(s.dir, "users"))
	if err != nil {
		return err
	}
	type pair struct{ old, neu int64 }
	moves := make([]pair, 0)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id, err := strconv.ParseInt(stringsTrimSuffixJSON(e.Name()), 10, 64)
		if err != nil || id > 49999 {
			continue
		}
		neu := remapLowUID(id)
		if neu == id {
			continue
		}
		if _, err := os.Stat(s.userPath(neu)); err == nil {
			log.Printf("[store] skip JSON UID remap %d→%d (target exists)", id, neu)
			continue
		}
		moves = append(moves, pair{id, neu})
	}
	for _, m := range moves {
		doc, err := s.loadDoc(m.old)
		if err != nil || doc == nil {
			continue
		}
		doc.User.UserID = m.neu
		if doc.User.MapID == int(m.old) {
			doc.User.MapID = int(m.neu)
		}
		if err := s.saveDoc(doc); err != nil {
			return err
		}
		_ = os.Remove(s.userPath(m.old))
		for email, uid := range s.emails {
			if uid == m.old {
				s.emails[email] = m.neu
			}
		}
		log.Printf("[store] remapped JSON UID %d→%d (基地需 userID>49999)", m.old, m.neu)
	}
	if cur := s.nextID.Load(); cur < MinUserID {
		s.nextID.Store(MinUserID)
	}
	_ = s.saveMetaLocked()
	return nil
}

func stringsTrimSuffixJSON(name string) string {
	if len(name) > 5 && name[len(name)-5:] == ".json" {
		return name[:len(name)-5]
	}
	return name
}
