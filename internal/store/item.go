package store

import (
	"database/sql"
	"fmt"
)

// Item 背包道具（对齐 items 表）。
type Item struct {
	ItemID     int
	Count      int
	ExpireTime int
}

const defaultItemExpire = 0x057E40

func (s *sqlBackend) AddItem(uid int64, itemID, delta int) error {
	if itemID <= 0 || delta == 0 {
		return nil
	}
	_, err := s.db.Exec(`
INSERT INTO items(user_id, item_id, count, expire_time)
VALUES(?,?,?,?)
ON DUPLICATE KEY UPDATE count=count+VALUES(count)`,
		uid, itemID, delta, defaultItemExpire)
	return err
}

// GetItemCount 读道具数量；无记录返回 0。
func (s *sqlBackend) GetItemCount(uid int64, itemID int) (int, error) {
	if itemID <= 0 {
		return 0, nil
	}
	var n int
	err := s.db.QueryRow(`SELECT count FROM items WHERE user_id=? AND item_id=?`, uid, itemID).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return n, err
}

// ConsumeItem 扣除道具；库存不足返回错误。
func (s *sqlBackend) ConsumeItem(uid int64, itemID, amount int) error {
	if itemID <= 0 || amount <= 0 {
		return nil
	}
	res, err := s.db.Exec(`
UPDATE items SET count=count-? WHERE user_id=? AND item_id=? AND count>=?`,
		amount, uid, itemID, amount)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("item %d stock insufficient", itemID)
	}
	return nil
}

func (s *sqlBackend) ListItemsInRange(uid int64, lo, hi, extra int) ([]Item, error) {
	rows, err := s.db.Query(`
SELECT item_id, count, expire_time FROM items
WHERE user_id=? AND count>0`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Item, 0)
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ItemID, &it.Count, &it.ExpireTime); err != nil {
			return nil, err
		}
		id := it.ItemID
		ok := false
		if lo == 0 && hi == 0 && extra == 0 {
			ok = isCollectionItem(id)
		} else if (id >= lo && id <= hi) || id == extra {
			ok = true
		}
		if !ok {
			continue
		}
		if it.ExpireTime == 0 {
			it.ExpireTime = defaultItemExpire
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// ListAllItems GM：背包全部 count>0 道具。
func (s *sqlBackend) ListAllItems(uid int64) ([]Item, error) {
	rows, err := s.db.Query(`
SELECT item_id, count, expire_time FROM items
WHERE user_id=? AND count>0 ORDER BY item_id`, uid)
	if err != nil {
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

func isCollectionItem(id int) bool {
	return (id > 10000 && id < 100000) ||
		(id >= 400001 && id <= 500000) ||
		(id >= 1200001 && id <= 1300000) ||
		(id > 1500000 && id < 1600000)
}

func (s *sqlBackend) AddCoins(uid int64, delta int) error {
	if delta == 0 {
		return nil
	}
	res, err := s.db.Exec(`UPDATE users SET coins=coins+? WHERE user_id=?`, delta, uid)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user %d not found", uid)
	}
	return nil
}

// AddGold 增减钻石/黄金豆。
func (s *sqlBackend) AddGold(uid int64, delta int) error {
	if delta == 0 {
		return nil
	}
	res, err := s.db.Exec(`UPDATE users SET gold=gold+? WHERE user_id=?`, delta, uid)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user %d not found", uid)
	}
	return nil
}

// GetGold 读金豆余额。
func (s *sqlBackend) GetGold(uid int64) (int, error) {
	var g int
	err := s.db.QueryRow(`SELECT gold FROM users WHERE user_id=?`, uid).Scan(&g)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return g, err
}

// TrySpendGold 扣金豆；余额不足返回 ok=false。
func (s *sqlBackend) TrySpendGold(uid int64, amount int) (balance int, ok bool, err error) {
	if amount < 0 {
		return 0, false, fmt.Errorf("bad amount")
	}
	if amount == 0 {
		b, e := s.GetGold(uid)
		return b, true, e
	}
	res, err := s.db.Exec(`UPDATE users SET gold=gold-? WHERE user_id=? AND gold>=?`, amount, uid, amount)
	if err != nil {
		return 0, false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		b, _ := s.GetGold(uid)
		return b, false, nil
	}
	b, err := s.GetGold(uid)
	return b, true, err
}

// TrySpendCoins 扣赛尔豆；余额不足返回 ok=false。
func (s *sqlBackend) TrySpendCoins(uid int64, amount int) (balance int, ok bool, err error) {
	if amount < 0 {
		return 0, false, fmt.Errorf("bad amount")
	}
	if amount == 0 {
		b, e := s.GetCoins(uid)
		return b, true, e
	}
	res, err := s.db.Exec(`UPDATE users SET coins=coins-? WHERE user_id=? AND coins>=?`, amount, uid, amount)
	if err != nil {
		return 0, false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		b, _ := s.GetCoins(uid)
		return b, false, nil
	}
	b, err := s.GetCoins(uid)
	return b, true, err
}

func (s *sqlBackend) GetCoins(uid int64) (int, error) {
	var coins int
	err := s.db.QueryRow(`SELECT coins FROM users WHERE user_id=?`, uid).Scan(&coins)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return coins, err
}
