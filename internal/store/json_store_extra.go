package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func (s *jsonStore) IsPetCollectClaimed(uid int64, period int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return false, err
	}
	if period == 301 {
		return doc.PetKing301, nil
	}
	if period < 1 || period > 31 {
		return false, nil
	}
	return doc.PetCollectMask&(1<<(period-1)) != 0, nil
}

func (s *jsonStore) MarkPetCollectClaimed(uid int64, period int) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		if period == 301 {
			doc.PetKing301 = true
			return nil
		}
		if period >= 1 && period <= 31 {
			doc.PetCollectMask |= 1 << (period - 1)
		}
		return nil
	})
}

func (s *jsonStore) ListSoulBeads(uid int64) ([]SoulBead, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return nil, err
	}
	return append([]SoulBead(nil), doc.SoulBeads...), nil
}

func (s *jsonStore) UpsertSoulBead(uid int64, b SoulBead) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		for i := range doc.SoulBeads {
			if doc.SoulBeads[i].ObtainTime == b.ObtainTime {
				doc.SoulBeads[i] = b
				return nil
			}
		}
		doc.SoulBeads = append(doc.SoulBeads, b)
		return nil
	})
}

func (s *jsonStore) DeleteSoulBead(uid int64, obtainTime uint32) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		kept := doc.SoulBeads[:0]
		for _, b := range doc.SoulBeads {
			if b.ObtainTime != obtainTime {
				kept = append(kept, b)
			}
		}
		doc.SoulBeads = kept
		return nil
	})
}

func (s *jsonStore) GetSoulBead(uid int64, obtainTime uint32) (*SoulBead, error) {
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

func (s *jsonStore) GetHatchState(uid int64) (HatchState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return HatchState{}, err
	}
	return doc.Hatch, nil
}

func (s *jsonStore) SetHatchState(uid int64, h HatchState) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		doc.Hatch = h
		return nil
	})
}

func (s *jsonStore) ClearHatchState(uid int64) error {
	return s.SetHatchState(uid, HatchState{})
}

func (s *jsonStore) DeletePet(uid, catchTime int64) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		kept := doc.Pets[:0]
		found := false
		for _, p := range doc.Pets {
			if p.CatchTime == catchTime {
				found = true
				continue
			}
			kept = append(kept, p)
		}
		if !found {
			return fmt.Errorf("pet not found")
		}
		doc.Pets = kept
		s.reindexBag(doc)
		return nil
	})
}

func (s *jsonStore) ListFitments(uid int64) ([]Fitment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return nil, err
	}
	return append([]Fitment(nil), doc.Fitments...), nil
}

func (s *jsonStore) ReplaceFitments(uid int64, list []Fitment) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		doc.Fitments = append([]Fitment(nil), list...)
		return nil
	})
}

func (s *jsonStore) ListFitmentItems(uid int64) ([]Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return nil, err
	}
	out := make([]Item, 0)
	for k, it := range doc.Items {
		id := it.ItemID
		if id <= 0 {
			id, _ = strconv.Atoi(k)
			it.ItemID = id
		}
		if it.Count > 0 && id >= 500001 && id <= 599999 {
			if it.ExpireTime == 0 {
				it.ExpireTime = defaultItemExpire
			}
			out = append(out, it)
		}
	}
	return out, nil
}

func (s *jsonStore) GetBreedState(uid int64) (BreedState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return BreedState{Intimacy: 1}, err
	}
	st := doc.Breed
	if st.Intimacy < 1 {
		st.Intimacy = 1
	}
	return st, nil
}

func (s *jsonStore) SetBreedState(uid int64, st BreedState) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		if st.Intimacy < 1 {
			st.Intimacy = 1
		}
		doc.Breed = st
		return nil
	})
}

func (s *jsonStore) GetRoomPets(uid int64) (RoomPets, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return RoomPets{}, err
	}
	return sanitizeRoomPets(doc.RoomPets), nil
}

func (s *jsonStore) SetRoomPets(uid int64, list RoomPets) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		doc.RoomPets = sanitizeRoomPets(list)
		return nil
	})
}

func (s *jsonStore) GetNonoVipSign(uid int64) (NonoVipSignState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return NonoVipSignState{}, err
	}
	return doc.NonoVipSign, nil
}

func (s *jsonStore) SetNonoVipSign(uid int64, st NonoVipSignState) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		doc.NonoVipSign = st
		return nil
	})
}

func (s *jsonStore) GetUserOps(uid int64) (UserOpsState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return UserOpsState{}, err
	}
	return doc.UserOps, nil
}

func (s *jsonStore) SetUserOps(uid int64, st UserOpsState) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		doc.UserOps = st
		return nil
	})
}

// ListTopWarRanks JSON 存档：扫描 users/*.json，按 curTopLevel 降序。
func (s *jsonStore) ListTopWarRanks(limit int) ([]TopWarRankEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(filepath.Join(s.dir, "users"))
	if err != nil {
		return nil, err
	}
	out := make([]TopWarRankEntry, 0)
	for _, e := range entries {
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
		sc := ClampTopLevel(doc.UserOps.CurTopLevel)
		if sc <= 0 {
			continue
		}
		out = append(out, TopWarRankEntry{
			UserID:   doc.User.UserID,
			Nickname: doc.User.Nickname,
			Score:    sc,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].UserID < out[j].UserID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
