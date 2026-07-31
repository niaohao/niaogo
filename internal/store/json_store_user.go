package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (s *jsonStore) FindByEmail(email string) (*User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	s.mu.Lock()
	uid, ok := s.emails[email]
	s.mu.Unlock()
	if !ok {
		return nil, nil
	}
	return s.FindByUserID(uid)
}

func (s *jsonStore) FindByUserID(uid int64) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return nil, err
	}
	u := doc.User
	return &u, nil
}

func (s *jsonStore) CreateUser(email, passwordMD5 string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	email = strings.TrimSpace(strings.ToLower(email))
	if _, ok := s.emails[email]; ok {
		return nil, fmt.Errorf("email exists")
	}
	uid := s.nextID.Add(1)
	now := time.Now().Unix()
	u := User{
		UserID: uid, Email: email, Password: passwordMD5,
		Energy: 100, MapID: 1, PosX: 475, PosY: 395, RegisterTime: now,
	}
	doc := s.emptyDoc(u)
	if err := s.saveDoc(doc); err != nil {
		return nil, err
	}
	s.emails[email] = uid
	_ = s.saveMetaLocked()
	return &u, nil
}

func (s *jsonStore) SaveUser(u *User) error {
	if u == nil {
		return nil
	}
	return s.withDoc(u.UserID, func(doc *jsonUserDoc) error {
		oldEmail := strings.TrimSpace(strings.ToLower(doc.User.Email))
		newEmail := strings.TrimSpace(strings.ToLower(u.Email))
		doc.User = *u
		if oldEmail != newEmail {
			delete(s.emails, oldEmail)
			if newEmail != "" {
				s.emails[newEmail] = u.UserID
			}
			_ = s.saveMetaLocked()
		}
		return nil
	})
}

func (s *jsonStore) SearchUsers(q string, limit int) ([]UserBrief, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(filepath.Join(s.dir, "users"))
	if err != nil {
		return nil, err
	}
	q = strings.TrimSpace(q)
	out := make([]UserBrief, 0)
	for _, e := range entries {
		if len(out) >= limit {
			break
		}
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		uid, err := strconv.ParseInt(strings.TrimSuffix(e.Name(), ".json"), 10, 64)
		if err != nil {
			continue
		}
		doc, err := s.loadDoc(uid)
		if err != nil || doc == nil {
			continue
		}
		u := doc.User
		if q != "" {
			like := strings.Contains(u.Nickname, q) || strings.Contains(u.Email, q) || strconv.FormatInt(u.UserID, 10) == q
			if !like {
				continue
			}
		}
		out = append(out, UserBrief{
			UserID: u.UserID, Nickname: u.Nickname, Email: u.Email,
			Coins: u.Coins, Gold: u.Gold, MapID: u.MapID,
		})
	}
	return out, nil
}

// —— 货币 / 经验池 ——

func (s *jsonStore) AddCoins(uid int64, delta int) error {
	if delta == 0 {
		return nil
	}
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		doc.User.Coins += delta
		return nil
	})
}

func (s *jsonStore) AddGold(uid int64, delta int) error {
	if delta == 0 {
		return nil
	}
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		doc.User.Gold += delta
		return nil
	})
}

func (s *jsonStore) GetGold(uid int64) (int, error) {
	u, err := s.FindByUserID(uid)
	if err != nil || u == nil {
		return 0, err
	}
	return u.Gold, nil
}

func (s *jsonStore) GetCoins(uid int64) (int, error) {
	u, err := s.FindByUserID(uid)
	if err != nil || u == nil {
		return 0, err
	}
	return u.Coins, nil
}

func (s *jsonStore) TrySpendGold(uid int64, amount int) (balance int, ok bool, err error) {
	if amount < 0 {
		return 0, false, fmt.Errorf("bad amount")
	}
	err = s.withDoc(uid, func(doc *jsonUserDoc) error {
		if doc.User.Gold < amount {
			balance = doc.User.Gold
			ok = false
			return nil
		}
		doc.User.Gold -= amount
		balance = doc.User.Gold
		ok = true
		return nil
	})
	return
}

func (s *jsonStore) TrySpendCoins(uid int64, amount int) (balance int, ok bool, err error) {
	if amount < 0 {
		return 0, false, fmt.Errorf("bad amount")
	}
	err = s.withDoc(uid, func(doc *jsonUserDoc) error {
		if doc.User.Coins < amount {
			balance = doc.User.Coins
			ok = false
			return nil
		}
		doc.User.Coins -= amount
		balance = doc.User.Coins
		ok = true
		return nil
	})
	return
}

func (s *jsonStore) GetExpPool(uid int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return 0, err
	}
	return doc.ExpPool, nil
}

func (s *jsonStore) AddExpPool(uid int64, delta int) (int, error) {
	var bal int
	err := s.withDoc(uid, func(doc *jsonUserDoc) error {
		doc.ExpPool += delta
		if doc.ExpPool < 0 {
			doc.ExpPool = 0
		}
		bal = doc.ExpPool
		return nil
	})
	return bal, err
}

func (s *jsonStore) SetExpPool(uid int64, value int) error {
	if value < 0 {
		value = 0
	}
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		doc.ExpPool = value
		return nil
	})
}

func (s *jsonStore) AddGoldWithLedger(uid int64, delta int, reason, ref string) (balance int, err error) {
	_ = reason
	_ = ref
	if err = s.AddGold(uid, delta); err != nil {
		return 0, err
	}
	return s.GetGold(uid)
}

// —— 道具 ——

func itemKey(id int) string { return strconv.Itoa(id) }

func (s *jsonStore) AddItem(uid int64, itemID, delta int) error {
	if itemID <= 0 || delta == 0 {
		return nil
	}
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		k := itemKey(itemID)
		it := doc.Items[k]
		it.ItemID = itemID
		it.Count += delta
		if it.ExpireTime == 0 {
			it.ExpireTime = defaultItemExpire
		}
		if it.Count <= 0 {
			delete(doc.Items, k)
		} else {
			doc.Items[k] = it
		}
		return nil
	})
}

func (s *jsonStore) GetItemCount(uid int64, itemID int) (int, error) {
	if itemID <= 0 {
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return 0, err
	}
	return doc.Items[itemKey(itemID)].Count, nil
}

func (s *jsonStore) ConsumeItem(uid int64, itemID, amount int) error {
	if itemID <= 0 || amount <= 0 {
		return nil
	}
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		k := itemKey(itemID)
		it := doc.Items[k]
		if it.Count < amount {
			return fmt.Errorf("item %d stock insufficient", itemID)
		}
		it.Count -= amount
		if it.Count <= 0 {
			delete(doc.Items, k)
		} else {
			doc.Items[k] = it
		}
		return nil
	})
}

func (s *jsonStore) ListItemsInRange(uid int64, lo, hi, extra int) ([]Item, error) {
	all, err := s.ListAllItems(uid)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0)
	for _, it := range all {
		id := it.ItemID
		ok := false
		if lo == 0 && hi == 0 && extra == 0 {
			ok = isCollectionItem(id)
		} else if (id >= lo && id <= hi) || id == extra {
			ok = true
		}
		if ok {
			out = append(out, it)
		}
	}
	return out, nil
}

func (s *jsonStore) ListAllItems(uid int64) ([]Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return nil, err
	}
	out := make([]Item, 0, len(doc.Items))
	for _, it := range doc.Items {
		if it.Count > 0 {
			if it.ExpireTime == 0 {
				it.ExpireTime = defaultItemExpire
			}
			out = append(out, it)
		}
	}
	return out, nil
}
