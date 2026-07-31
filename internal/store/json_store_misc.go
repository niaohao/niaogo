package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func (s *jsonStore) UpsertTaskStatus(uid int64, taskID, status int) error {
	if taskID <= 0 {
		return fmt.Errorf("invalid taskID")
	}
	if status < 0 {
		status = 0
	}
	if status > 255 {
		status = 255
	}
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		k := strconv.Itoa(taskID)
		t := doc.Tasks[k]
		t.TaskID = taskID
		t.Status = status
		if t.Buf == nil {
			t.Buf = []byte{}
		}
		doc.Tasks[k] = t
		return nil
	})
}

func (s *jsonStore) UpsertTaskBuf(uid int64, taskID int, buf []byte) error {
	if taskID <= 0 {
		return fmt.Errorf("invalid taskID")
	}
	padded := make([]byte, 20)
	copy(padded, buf)
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		k := strconv.Itoa(taskID)
		t := doc.Tasks[k]
		t.TaskID = taskID
		if t.Status == 0 {
			t.Status = 1
		}
		t.Buf = padded
		doc.Tasks[k] = t
		return nil
	})
}

func (s *jsonStore) GetTask(uid int64, taskID int) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return nil, err
	}
	t, ok := doc.Tasks[strconv.Itoa(taskID)]
	if !ok {
		return nil, nil
	}
	cp := t
	return &cp, nil
}

func (s *jsonStore) ListTaskStatuses(uid int64) (map[int]int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return nil, err
	}
	out := make(map[int]int, len(doc.Tasks))
	for _, t := range doc.Tasks {
		out[t.TaskID] = t.Status
	}
	return out, nil
}

func (s *jsonStore) EncodeLoginTaskList(uid int64, size int) ([]byte, error) {
	if size <= 0 {
		size = 1000
	}
	out := make([]byte, size)
	statuses, err := s.ListTaskStatuses(uid)
	if err != nil {
		return out, err
	}
	for id, st := range statuses {
		if id < 1 || id > size {
			continue
		}
		if st < 0 {
			st = 0
		} else if st > 255 {
			st = 255
		}
		if id == 8 && st == 1 {
			st = 0
		}
		out[id-1] = byte(st)
	}
	return out, nil
}

func (s *jsonStore) DeleteTask(uid int64, taskID int) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		delete(doc.Tasks, strconv.Itoa(taskID))
		return nil
	})
}

// —— 邮件 ——

func (s *jsonStore) ListMails(uid int64, start int) (total int, mails []Mail, err error) {
	if start < 1 {
		start = 1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return 0, nil, err
	}
	total = len(doc.Mails)
	offset := start - 1
	if offset >= total {
		return total, nil, nil
	}
	end := offset + mailPageSize
	if end > total {
		end = total
	}
	// 按时间倒序：假设写入时 append，这里倒序切片
	ordered := make([]Mail, len(doc.Mails))
	copy(ordered, doc.Mails)
	for i, j := 0, len(ordered)-1; i < j; i, j = i+1, j-1 {
		ordered[i], ordered[j] = ordered[j], ordered[i]
	}
	return total, ordered[offset:end], nil
}

func (s *jsonStore) GetMail(uid, mailID int64) (*Mail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return nil, err
	}
	for i := range doc.Mails {
		if doc.Mails[i].ID == mailID {
			m := doc.Mails[i]
			return &m, nil
		}
	}
	return nil, nil
}

func (s *jsonStore) CountUnreadMails(uid int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return 0, err
	}
	n := 0
	for _, m := range doc.Mails {
		if !m.Read {
			n++
		}
	}
	return n, nil
}

func (s *jsonStore) InsertMail(uid int64, template int, fromID int64, fromNick, content string) (int64, error) {
	return s.InsertMailWithReward(uid, template, fromID, fromNick, content, MailReward{})
}

func (s *jsonStore) InsertMailWithReward(uid int64, template int, fromID int64, fromNick, content string, reward MailReward) (int64, error) {
	var id int64
	err := s.withDoc(uid, func(doc *jsonUserDoc) error {
		id = s.mailSeq.Add(1)
		doc.Mails = append(doc.Mails, Mail{
			ID: id, Template: template, MailTime: time.Now().Unix(),
			FromID: fromID, FromNick: fromNick, Content: content, Reward: reward,
		})
		_ = s.saveMetaLocked()
		return nil
	})
	return id, err
}

func (s *jsonStore) MarkMailsRead(uid int64, ids []int64) error {
	set := map[int64]bool{}
	for _, id := range ids {
		set[id] = true
	}
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		for i := range doc.Mails {
			if set[doc.Mails[i].ID] {
				doc.Mails[i].Read = true
			}
		}
		return nil
	})
}

func (s *jsonStore) MarkMailsReadAndClaim(uid int64, ids []int64) ([]Mail, error) {
	set := map[int64]bool{}
	for _, id := range ids {
		if id > 0 {
			set[id] = true
		}
	}
	out := make([]Mail, 0)
	err := s.withDoc(uid, func(doc *jsonUserDoc) error {
		for i := range doc.Mails {
			m := &doc.Mails[i]
			if !set[m.ID] {
				continue
			}
			if m.Claimed {
				m.Read = true
				continue
			}
			m.Read = true
			m.Claimed = true
			if m.Reward.HasReward() {
				out = append(out, *m)
			}
		}
		return nil
	})
	return out, err
}

func (s *jsonStore) DeleteMails(uid int64, ids []int64) error {
	set := map[int64]bool{}
	for _, id := range ids {
		set[id] = true
	}
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		kept := doc.Mails[:0]
		for _, m := range doc.Mails {
			if !set[m.ID] {
				kept = append(kept, m)
			}
		}
		doc.Mails = kept
		return nil
	})
}

func (s *jsonStore) DeleteAllMails(uid int64) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		doc.Mails = nil
		return nil
	})
}

// —— 社交 / 装扮 ——

func (s *jsonStore) ListFriends(uid int64) ([]FriendEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return nil, err
	}
	out := append([]FriendEntry(nil), doc.Friends...)
	return out, nil
}

func (s *jsonStore) ListBlacklist(uid int64) ([]BlackEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return nil, err
	}
	return append([]BlackEntry(nil), doc.Blacklist...), nil
}

func (s *jsonStore) IsFriend(uid, friendID int64) (bool, error) {
	friends, err := s.ListFriends(uid)
	if err != nil {
		return false, err
	}
	for _, f := range friends {
		if f.UserID == friendID {
			return true, nil
		}
	}
	return false, nil
}

func (s *jsonStore) AddFriend(uid, friendID int64) error {
	if uid <= 0 || friendID <= 0 || uid == friendID {
		return nil
	}
	tp := uint32(time.Now().Unix())
	add := func(a, b int64) error {
		return s.withDoc(a, func(doc *jsonUserDoc) error {
			found := false
			for i := range doc.Friends {
				if doc.Friends[i].UserID == b {
					doc.Friends[i].TimePoke = tp
					found = true
					break
				}
			}
			if !found {
				doc.Friends = append(doc.Friends, FriendEntry{UserID: b, TimePoke: tp})
			}
			kept := doc.Blacklist[:0]
			for _, bl := range doc.Blacklist {
				if bl.UserID != b {
					kept = append(kept, bl)
				}
			}
			doc.Blacklist = kept
			return nil
		})
	}
	if err := add(uid, friendID); err != nil {
		return err
	}
	return add(friendID, uid)
}

func (s *jsonStore) RemoveFriend(uid, friendID int64) error {
	rm := func(a, b int64) error {
		return s.withDoc(a, func(doc *jsonUserDoc) error {
			kept := doc.Friends[:0]
			for _, f := range doc.Friends {
				if f.UserID != b {
					kept = append(kept, f)
				}
			}
			doc.Friends = kept
			return nil
		})
	}
	_ = rm(uid, friendID)
	_ = rm(friendID, uid)
	return nil
}

func (s *jsonStore) AddBlacklist(uid, blackID int64) error {
	if uid <= 0 || blackID <= 0 || uid == blackID {
		return nil
	}
	_ = s.RemoveFriend(uid, blackID)
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		for _, b := range doc.Blacklist {
			if b.UserID == blackID {
				return nil
			}
		}
		doc.Blacklist = append(doc.Blacklist, BlackEntry{UserID: blackID})
		return nil
	})
}

func (s *jsonStore) RemoveBlacklist(uid, blackID int64) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		kept := doc.Blacklist[:0]
		for _, b := range doc.Blacklist {
			if b.UserID != blackID {
				kept = append(kept, b)
			}
		}
		doc.Blacklist = kept
		return nil
	})
}

func (s *jsonStore) ListWornClothes(uid int64) ([]WornCloth, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return nil, err
	}
	return append([]WornCloth(nil), doc.Clothes...), nil
}

func (s *jsonStore) SetWornClothes(uid int64, clothes []WornCloth) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		out := make([]WornCloth, 0, len(clothes))
		for i, w := range clothes {
			if w.ItemID <= 0 {
				continue
			}
			w.SlotIdx = i + 1
			out = append(out, w)
		}
		doc.Clothes = out
		return nil
	})
}

// —— 进度 / NoNo / 装备 / 成就 ——

func (s *jsonStore) GetProgress(uid int64) (UserProgress, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return UserProgress{BraveCur: 1, BraveMax: 1, FreshCur: 1, FreshMax: 1}, err
	}
	return doc.Progress, nil
}

func (s *jsonStore) SetBraveProgress(uid int64, cur int) error {
	if cur < 1 {
		cur = 1
	}
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		doc.Progress.BraveCur = cur
		if cur > doc.Progress.BraveMax {
			doc.Progress.BraveMax = cur
		}
		return nil
	})
}

func (s *jsonStore) SetFreshProgress(uid int64, cur int) error {
	if cur < 1 {
		cur = 1
	}
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		doc.Progress.FreshCur = cur
		if cur > doc.Progress.FreshMax {
			doc.Progress.FreshMax = cur
		}
		return nil
	})
}

func (s *jsonStore) GetGaiyaEffect(uid int64) (defID, mask int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return 0, 0, err
	}
	return doc.GaiyaDef, doc.GaiyaMask, nil
}

func (s *jsonStore) SetGaiyaEffect(uid int64, defID, mask int) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		doc.GaiyaDef, doc.GaiyaMask = defID, mask
		return nil
	})
}

func (s *jsonStore) GetLeiyiTrain(uid int64) (LeiyiTrainProgress, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return LeiyiTrainProgress{}, err
	}
	return doc.LeiyiTrain, nil
}

func (s *jsonStore) SetLeiyiTrain(uid int64, st LeiyiTrainProgress) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		doc.LeiyiTrain = st
		return nil
	})
}

func (s *jsonStore) GetBoostTimes(uid int64) (BoostTimes, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return BoostTimes{}, err
	}
	return doc.Boost, nil
}

func (s *jsonStore) setBoostField(uid int64, apply func(*BoostTimes)) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		apply(&doc.Boost)
		return nil
	})
}

func (s *jsonStore) SetLearnTimes(uid int64, n int) error {
	return s.setBoostField(uid, func(b *BoostTimes) { b.LearnTimes = clampBoost(n) })
}

func (s *jsonStore) SetTwoTimes(uid int64, n int) error {
	return s.setBoostField(uid, func(b *BoostTimes) { b.TwoTimes = clampBoost(n) })
}

func (s *jsonStore) SetThreeTimes(uid int64, n int) error {
	return s.setBoostField(uid, func(b *BoostTimes) { b.ThreeTimes = clampBoost(n) })
}

func (s *jsonStore) SetEnergyTimes(uid int64, n int) error {
	return s.setBoostField(uid, func(b *BoostTimes) { b.EnergyTimes = clampBoost(n) })
}

func (s *jsonStore) SetAutoFightTimes(uid int64, n int) error {
	return s.setBoostField(uid, func(b *BoostTimes) { b.AutoFightTimes = clampBoost(n) })
}

func (s *jsonStore) SetAutoFight(uid int64, n int) error {
	if n < 0 {
		n = 0
	}
	if n > 3 {
		n = 3
	}
	return s.setBoostField(uid, func(b *BoostTimes) { b.AutoFight = n })
}

func (s *jsonStore) AddLearnTimes(uid int64, delta int) (int, error) {
	var out int
	err := s.withDoc(uid, func(doc *jsonUserDoc) error {
		out = clampBoost(doc.Boost.LearnTimes + delta)
		doc.Boost.LearnTimes = out
		return nil
	})
	return out, err
}

func (s *jsonStore) AddTwoTimes(uid int64, delta int) (int, error) {
	var out int
	err := s.withDoc(uid, func(doc *jsonUserDoc) error {
		out = clampBoost(doc.Boost.TwoTimes + delta)
		doc.Boost.TwoTimes = out
		return nil
	})
	return out, err
}

func (s *jsonStore) AddThreeTimes(uid int64, delta int) (int, error) {
	var out int
	err := s.withDoc(uid, func(doc *jsonUserDoc) error {
		out = clampBoost(doc.Boost.ThreeTimes + delta)
		doc.Boost.ThreeTimes = out
		return nil
	})
	return out, err
}

func (s *jsonStore) AddEnergyTimes(uid int64, delta int) (int, error) {
	var out int
	err := s.withDoc(uid, func(doc *jsonUserDoc) error {
		out = clampBoost(doc.Boost.EnergyTimes + delta)
		doc.Boost.EnergyTimes = out
		return nil
	})
	return out, err
}

func (s *jsonStore) GetGoldPromoCount(uid int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return 0, err
	}
	return doc.GoldPromoN, nil
}

func (s *jsonStore) AddGoldPromoCount(uid int64) (int, error) {
	var out int
	err := s.withDoc(uid, func(doc *jsonUserDoc) error {
		doc.GoldPromoN++
		out = doc.GoldPromoN
		return nil
	})
	return out, err
}

func (s *jsonStore) AddAutoFightTimes(uid int64, delta int) (int, error) {
	var out int
	err := s.withDoc(uid, func(doc *jsonUserDoc) error {
		out = clampBoost(doc.Boost.AutoFightTimes + delta)
		doc.Boost.AutoFightTimes = out
		return nil
	})
	return out, err
}

func (s *jsonStore) ConsumeLearnTimes(uid int64, n int) (ok bool, left int, err error) {
	err = s.withDoc(uid, func(doc *jsonUserDoc) error {
		if n <= 0 {
			ok, left = true, doc.Boost.LearnTimes
			return nil
		}
		if doc.Boost.LearnTimes < n {
			ok, left = false, doc.Boost.LearnTimes
			return nil
		}
		doc.Boost.LearnTimes -= n
		ok, left = true, doc.Boost.LearnTimes
		return nil
	})
	return
}

func (s *jsonStore) ConsumeTwoTimes(uid int64, n int) (ok bool, left int, err error) {
	err = s.withDoc(uid, func(doc *jsonUserDoc) error {
		if doc.Boost.TwoTimes < n {
			ok, left = false, doc.Boost.TwoTimes
			return nil
		}
		doc.Boost.TwoTimes -= n
		ok, left = true, doc.Boost.TwoTimes
		return nil
	})
	return
}

func (s *jsonStore) ConsumeThreeTimes(uid int64, n int) (ok bool, left int, err error) {
	err = s.withDoc(uid, func(doc *jsonUserDoc) error {
		if doc.Boost.ThreeTimes < n {
			ok, left = false, doc.Boost.ThreeTimes
			return nil
		}
		doc.Boost.ThreeTimes -= n
		ok, left = true, doc.Boost.ThreeTimes
		return nil
	})
	return
}

func (s *jsonStore) ConsumeAutoFightTimes(uid int64, n int) (ok bool, left int, err error) {
	err = s.withDoc(uid, func(doc *jsonUserDoc) error {
		if doc.Boost.AutoFightTimes < n {
			ok, left = false, doc.Boost.AutoFightTimes
			return nil
		}
		doc.Boost.AutoFightTimes -= n
		ok, left = true, doc.Boost.AutoFightTimes
		if left == 0 {
			doc.Boost.AutoFight = 0
		}
		return nil
	})
	return
}

func (s *jsonStore) GetRecruitClaimMask(uid int64) (uint32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return 0, err
	}
	return doc.RecruitMask, nil
}

func (s *jsonStore) SetRecruitClaimMask(uid int64, mask uint32) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		doc.RecruitMask = mask
		return nil
	})
}

func (s *jsonStore) ClaimRecruitSlot(uid int64, slot uint32) (already bool, mask uint32, err error) {
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

func (s *jsonStore) GetNono(uid int64) (*Nono, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return nil, err
	}
	if doc.Nono == nil {
		return nil, nil
	}
	n := *doc.Nono
	return &n, nil
}

func (s *jsonStore) GetOrInitNono(uid int64) (*Nono, error) {
	n, err := s.GetNono(uid)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return DefaultNono(uid), nil
	}
	return n, nil
}

func (s *jsonStore) UpsertNono(n *Nono) error {
	if n == nil {
		return nil
	}
	return s.withDoc(n.UserID, func(doc *jsonUserDoc) error {
		cp := *n
		doc.Nono = &cp
		return nil
	})
}

func (s *jsonStore) GetEquipLevel(uid int64, itemID int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return 0, err
	}
	return doc.Equips[strconv.Itoa(itemID)], nil
}

func (s *jsonStore) EnsureEquip(uid int64, itemID, level int) error {
	if itemID <= 0 {
		return nil
	}
	if level < 1 {
		level = 1
	}
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		k := strconv.Itoa(itemID)
		if _, ok := doc.Equips[k]; !ok {
			doc.Equips[k] = level
		}
		return nil
	})
}

func (s *jsonStore) SetEquipLevel(uid int64, itemID, level int) error {
	if itemID <= 0 || level < 0 {
		return nil
	}
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		doc.Equips[strconv.Itoa(itemID)] = level
		return nil
	})
}

func (s *jsonStore) ListEquipLevels(uid int64) (map[int]int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return nil, err
	}
	out := map[int]int{}
	for k, lv := range doc.Equips {
		if lv > 0 {
			id, _ := strconv.Atoi(k)
			out[id] = lv
		}
	}
	return out, nil
}

func (s *jsonStore) ListAchieveRules(uid int64) ([]AchieveRuleRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return nil, err
	}
	return append([]AchieveRuleRow(nil), doc.AchieveRules...), nil
}

func (s *jsonStore) ListAchieveRulesOfBranch(uid int64, branchID int) ([]AchieveRuleRow, error) {
	all, err := s.ListAchieveRules(uid)
	if err != nil {
		return nil, err
	}
	out := make([]AchieveRuleRow, 0)
	for _, r := range all {
		if r.BranchID == branchID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *jsonStore) UpsertAchieveRule(uid int64, r AchieveRuleRow) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		for i := range doc.AchieveRules {
			if doc.AchieveRules[i].BranchID == r.BranchID && doc.AchieveRules[i].RuleID == r.RuleID {
				doc.AchieveRules[i] = r
				return nil
			}
		}
		doc.AchieveRules = append(doc.AchieveRules, r)
		return nil
	})
}

func (s *jsonStore) GetAchieveBranchState(uid int64, branchID int) (value, status int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return 0, 0, err
	}
	v := doc.AchieveBranch[strconv.Itoa(branchID)]
	return v[0], v[1], nil
}

func (s *jsonStore) SetAchieveBranchState(uid int64, branchID, value, status int) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		doc.AchieveBranch[strconv.Itoa(branchID)] = [2]int{value, status}
		return nil
	})
}

func (s *jsonStore) ListTitles(uid int64) ([]int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return nil, err
	}
	return append([]int(nil), doc.Titles...), nil
}

func (s *jsonStore) GrantTitle(uid int64, titleID int) error {
	if titleID <= 0 {
		return nil
	}
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		for _, t := range doc.Titles {
			if t == titleID {
				return nil
			}
		}
		doc.Titles = append(doc.Titles, titleID)
		return nil
	})
}

func (s *jsonStore) ListDefeatedSPTKeys(uid int64) ([]int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return nil, err
	}
	return append([]int(nil), doc.SPTDefeated...), nil
}

func (s *jsonStore) HasDefeatedSPT(uid int64, bossKey int) (bool, error) {
	keys, err := s.ListDefeatedSPTKeys(uid)
	if err != nil {
		return false, err
	}
	for _, k := range keys {
		if k == bossKey {
			return true, nil
		}
	}
	return false, nil
}

func (s *jsonStore) MarkDefeatedSPT(uid int64, bossKey int) error {
	if uid <= 0 || bossKey == 0 {
		return nil
	}
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		for _, k := range doc.SPTDefeated {
			if k == bossKey {
				return nil
			}
		}
		doc.SPTDefeated = append(doc.SPTDefeated, bossKey)
		return nil
	})
}

func (s *jsonStore) InsertGMAudit(admin, action string, targetUID int64, detail any) (int64, error) {
	if admin == "" {
		admin = "gm"
	}
	var raw []byte
	if detail != nil {
		var err error
		raw, err = json.Marshal(detail)
		if err != nil {
			return 0, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.auditSeq.Add(1)
	row := jsonAuditRow{
		ID: id, Admin: admin, Action: action, TargetUser: targetUID,
		Detail: raw, CreatedAt: time.Now().Unix(),
	}
	b, _ := json.Marshal(row)
	f, err := os.OpenFile(filepath.Join(s.dir, "gm_audit.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	_, err = f.Write(append(b, '\n'))
	_ = f.Close()
	_ = s.saveMetaLocked()
	return id, err
}

func (s *jsonStore) ListGMAudit(limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	b, err := os.ReadFile(filepath.Join(s.dir, "gm_audit.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return []map[string]any{}, nil
		}
		return nil, err
	}
	lines := splitLines(b)
	out := make([]map[string]any, 0, limit)
	for i := len(lines) - 1; i >= 0 && len(out) < limit; i-- {
		if len(lines[i]) == 0 {
			continue
		}
		var row jsonAuditRow
		if json.Unmarshal(lines[i], &row) != nil {
			continue
		}
		m := map[string]any{
			"id": row.ID, "admin": row.Admin, "action": row.Action,
			"targetUser": row.TargetUser, "createdAt": row.CreatedAt,
		}
		if len(row.Detail) > 0 {
			m["detail"] = row.Detail
		}
		out = append(out, m)
	}
	return out, nil
}

func splitLines(b []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, c := range b {
		if c == '\n' {
			lines = append(lines, b[start:i])
			start = i + 1
		}
	}
	if start < len(b) {
		lines = append(lines, b[start:])
	}
	return lines
}
