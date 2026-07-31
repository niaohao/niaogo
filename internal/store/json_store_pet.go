package store

import (
	"fmt"
	"sort"
	"time"
)

func (s *jsonStore) findPetIdx(doc *jsonUserDoc, catchTime int64) int {
	for i := range doc.Pets {
		if doc.Pets[i].CatchTime == catchTime {
			return i
		}
	}
	return -1
}

func (s *jsonStore) UpsertPet(p *Pet) error {
	if p == nil {
		return fmt.Errorf("nil pet")
	}
	return s.withDoc(p.UserID, func(doc *jsonUserDoc) error {
		if p.Skills == nil {
			p.Skills = []int{}
		}
		idx := s.findPetIdx(doc, p.CatchTime)
		if idx >= 0 {
			doc.Pets[idx] = *p
		} else {
			doc.Pets = append(doc.Pets, *p)
		}
		return nil
	})
}

func (s *jsonStore) GetPetByCatchTime(uid, catchTime int64) (*Pet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return nil, err
	}
	idx := s.findPetIdx(doc, catchTime)
	if idx < 0 {
		return nil, nil
	}
	p := doc.Pets[idx]
	return &p, nil
}

func (s *jsonStore) patchPet(uid, catchTime int64, fn func(*Pet)) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		idx := s.findPetIdx(doc, catchTime)
		if idx < 0 {
			return fmt.Errorf("pet not found")
		}
		fn(&doc.Pets[idx])
		return nil
	})
}

func (s *jsonStore) SetPetCurrentHP(uid, catchTime int64, hp int) error {
	return s.patchPet(uid, catchTime, func(p *Pet) {
		if hp <= 0 {
			p.CurrentHP = 0
		} else {
			p.CurrentHP = hp
		}
	})
}

func (s *jsonStore) SetPetEnergyBall(uid, catchTime int64, itemID, leftCount, effectID int) error {
	return s.patchPet(uid, catchTime, func(p *Pet) {
		if itemID <= 0 || leftCount <= 0 {
			p.EnergyBallItemID, p.EnergyBallLeftCount, p.EnergyBallEffectID = 0, 0, 0
			return
		}
		p.EnergyBallItemID, p.EnergyBallLeftCount, p.EnergyBallEffectID = itemID, leftCount, effectID
	})
}

func (s *jsonStore) SetPetTrait(uid, catchTime int64, trait int) error {
	return s.patchPet(uid, catchTime, func(p *Pet) {
		if trait <= 0 {
			p.Trait = 0
			return
		}
		p.Trait = trait
	})
}

func (s *jsonStore) SetPetEV(uid, catchTime int64, ev [6]int) error {
	return s.patchPet(uid, catchTime, func(p *Pet) { p.EV = ev })
}

func (s *jsonStore) SetPetElite(uid, catchTime int64, elite bool) error {
	return s.patchPet(uid, catchTime, func(p *Pet) { p.IsElite = elite })
}

func (s *jsonStore) SetPetFormDisplay(uid, catchTime int64, formLocked, displayFormID, lockedDisplayFormID int) error {
	return s.patchPet(uid, catchTime, func(p *Pet) {
		p.FormLocked = formLocked
		p.DisplayFormID = displayFormID
		p.LockedDisplayFormID = lockedDisplayFormID
	})
}

func (s *jsonStore) SetPetExe(uid, catchTime int64, start int64, course int) error {
	return s.patchPet(uid, catchTime, func(p *Pet) {
		if start <= 0 {
			p.ExeStart, p.ExeCourse = 0, 0
			return
		}
		p.ExeStart, p.ExeCourse = start, course
	})
}

func (s *jsonStore) SetPetTrainBonus(uid, catchTime int64, bonus [6]int) error {
	return s.patchPet(uid, catchTime, func(p *Pet) { p.Bonus = bonus })
}

func (s *jsonStore) SetPetGMStats(uid, catchTime int64, stats [6]int) error {
	return s.patchPet(uid, catchTime, func(p *Pet) {
		p.GMStats = stats
		p.HasGMStats = true
	})
}

func (s *jsonStore) ClearPetGMStats(uid, catchTime int64) error {
	return s.patchPet(uid, catchTime, func(p *Pet) {
		p.GMStats = [6]int{}
		p.HasGMStats = false
	})
}

func (s *jsonStore) SetPetLearnedSkillBank(uid, catchTime int64, skills []int) error {
	return s.patchPet(uid, catchTime, func(p *Pet) {
		if skills == nil {
			p.LearnedSkillBank = []int{}
			return
		}
		p.LearnedSkillBank = append([]int(nil), skills...)
	})
}

func (s *jsonStore) filterPets(uid int64, pred func(Pet) bool) ([]Pet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadDoc(uid)
	if err != nil || doc == nil {
		return nil, err
	}
	out := make([]Pet, 0)
	for _, p := range doc.Pets {
		if pred(p) {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *jsonStore) ListBagPets(uid int64) ([]Pet, error) {
	pets, err := s.filterPets(uid, func(p Pet) bool { return p.InBag })
	if err != nil {
		return nil, err
	}
	sort.SliceStable(pets, func(i, j int) bool {
		if pets[i].BagPos != pets[j].BagPos {
			return pets[i].BagPos < pets[j].BagPos
		}
		return pets[i].CatchTime < pets[j].CatchTime
	})
	if len(pets) > MaxBagPets {
		pets = pets[:MaxBagPets]
	}
	return pets, nil
}

func (s *jsonStore) ListStoragePets(uid int64) ([]Pet, error) {
	return s.filterPets(uid, func(p Pet) bool {
		return !p.InBag && p.BagPos != RoweiBagPos && p.BagPos != ExeBagPos
	})
}

func (s *jsonStore) ListExePets(uid int64) ([]Pet, error) {
	return s.filterPets(uid, func(p Pet) bool { return !p.InBag && p.BagPos == ExeBagPos })
}

func (s *jsonStore) ListRoweiPets(uid int64) ([]Pet, error) {
	return s.filterPets(uid, func(p Pet) bool { return !p.InBag && p.BagPos == RoweiBagPos })
}

func (s *jsonStore) CountBagPets(uid int64) (int, error) {
	pets, err := s.ListBagPets(uid)
	return len(pets), err
}

func (s *jsonStore) CountRoweiPets(uid int64) (int, error) {
	pets, err := s.ListRoweiPets(uid)
	return len(pets), err
}

func (s *jsonStore) SetPetBagFlag(uid, catchTime int64, inBag bool, bagPos int) error {
	return s.patchPet(uid, catchTime, func(p *Pet) {
		p.InBag = inBag
		p.BagPos = bagPos
	})
}

func (s *jsonStore) reindexBag(doc *jsonUserDoc) {
	bag := make([]int, 0)
	for i, p := range doc.Pets {
		if p.InBag {
			bag = append(bag, i)
		}
	}
	for pos, i := range bag {
		doc.Pets[i].BagPos = pos
		doc.Pets[i].InBag = true
	}
}

func (s *jsonStore) MovePetToExe(uid, catchTime int64, course int, startUnix int64) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		idx := s.findPetIdx(doc, catchTime)
		if idx < 0 {
			return fmt.Errorf("pet not found")
		}
		p := &doc.Pets[idx]
		if p.BagPos == RoweiBagPos {
			return fmt.Errorf("pet in rowei")
		}
		p.InBag = false
		p.BagPos = ExeBagPos
		if course < 1 {
			course = 1
		}
		p.ExeStart, p.ExeCourse = startUnix, course
		s.reindexBag(doc)
		return nil
	})
}

func (s *jsonStore) EndPetExe(uid, catchTime int64) (*Pet, error) {
	var out *Pet
	err := s.withDoc(uid, func(doc *jsonUserDoc) error {
		idx := s.findPetIdx(doc, catchTime)
		if idx < 0 {
			return fmt.Errorf("pet not found")
		}
		p := &doc.Pets[idx]
		if p.BagPos != ExeBagPos {
			return fmt.Errorf("pet not in exe")
		}
		p.InBag = false
		p.BagPos = -1
		p.ExeStart, p.ExeCourse = 0, 0
		cp := *p
		out = &cp
		return nil
	})
	return out, err
}

func (s *jsonStore) MovePetToRowei(uid, catchTime int64) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		idx := s.findPetIdx(doc, catchTime)
		if idx < 0 {
			return fmt.Errorf("pet not in storage")
		}
		p := &doc.Pets[idx]
		if p.InBag || p.BagPos == RoweiBagPos || p.BagPos == ExeBagPos {
			return fmt.Errorf("pet not in storage")
		}
		p.InBag = false
		p.BagPos = RoweiBagPos
		return nil
	})
}

func (s *jsonStore) RetrievePetFromRowei(uid, catchTime int64) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		idx := s.findPetIdx(doc, catchTime)
		if idx < 0 {
			return fmt.Errorf("pet not in rowei")
		}
		p := &doc.Pets[idx]
		if p.InBag || p.BagPos != RoweiBagPos {
			return fmt.Errorf("pet not in rowei")
		}
		p.BagPos = -1
		return nil
	})
}

func (s *jsonStore) MovePetToStorage(uid, catchTime int64) (firstCatch int64, ok bool, err error) {
	err = s.withDoc(uid, func(doc *jsonUserDoc) error {
		idx := s.findPetIdx(doc, catchTime)
		if idx < 0 || !doc.Pets[idx].InBag {
			ok = false
			return nil
		}
		doc.Pets[idx].InBag = false
		doc.Pets[idx].BagPos = -1
		s.reindexBag(doc)
		ok = true
		for _, p := range doc.Pets {
			if p.InBag {
				firstCatch = p.CatchTime
				break
			}
		}
		return nil
	})
	return
}

func (s *jsonStore) MovePetToBag(uid, catchTime int64) (*Pet, bool, error) {
	var out *Pet
	var ok bool
	err := s.withDoc(uid, func(doc *jsonUserDoc) error {
		idx := s.findPetIdx(doc, catchTime)
		if idx < 0 {
			return nil
		}
		p := &doc.Pets[idx]
		if p.InBag {
			cp := *p
			out, ok = &cp, true
			return nil
		}
		if p.BagPos == RoweiBagPos || p.BagPos == ExeBagPos {
			return fmt.Errorf("pet in special storage")
		}
		n := 0
		for _, x := range doc.Pets {
			if x.InBag {
				n++
			}
		}
		if n >= MaxBagPets {
			ok = false
			return nil
		}
		p.InBag = true
		p.BagPos = n
		cp := *p
		out, ok = &cp, true
		return nil
	})
	return out, ok, err
}

func (s *jsonStore) SetDefaultPet(uid, catchTime int64) error {
	return s.withDoc(uid, func(doc *jsonUserDoc) error {
		idx := s.findPetIdx(doc, catchTime)
		if idx < 0 || !doc.Pets[idx].InBag {
			return fmt.Errorf("pet not in bag")
		}
		target := doc.Pets[idx]
		rest := make([]Pet, 0)
		for i, p := range doc.Pets {
			if i == idx {
				continue
			}
			if p.InBag {
				rest = append(rest, p)
			}
		}
		// rebuild bag order
		for i := range doc.Pets {
			if doc.Pets[i].InBag {
				doc.Pets[i].InBag = false
				doc.Pets[i].BagPos = -1
			}
		}
		ordered := append([]Pet{target}, rest...)
		for pos, p := range ordered {
			j := s.findPetIdx(doc, p.CatchTime)
			if j >= 0 {
				doc.Pets[j].InBag = true
				doc.Pets[j].BagPos = pos
			}
		}
		return nil
	})
}

func (s *jsonStore) NormalizeBagOverflow(uid int64) (int, error) {
	moved := 0
	err := s.withDoc(uid, func(doc *jsonUserDoc) error {
		bagIdx := make([]int, 0)
		for i, p := range doc.Pets {
			if p.InBag {
				bagIdx = append(bagIdx, i)
			}
		}
		if len(bagIdx) <= MaxBagPets {
			s.reindexBag(doc)
			return nil
		}
		for _, i := range bagIdx[MaxBagPets:] {
			doc.Pets[i].InBag = false
			doc.Pets[i].BagPos = -1
			moved++
		}
		s.reindexBag(doc)
		return nil
	})
	return moved, err
}

func (s *jsonStore) GrantPet(uid int64, petID int, name string, level, dv, nature int, skills []int) (catchTime int64, err error) {
	if petID <= 0 {
		return 0, fmt.Errorf("bad petId")
	}
	if level <= 0 {
		level = 1
	}
	if name == "" {
		name = "精灵"
	}
	if skills == nil {
		skills = []int{10001}
	}
	catchTime = time.Now().Unix()
	for i := 0; i < 5; i++ {
		if existing, _ := s.GetPetByCatchTime(uid, catchTime); existing == nil {
			break
		}
		catchTime++
	}
	p := &Pet{
		UserID: uid, CatchTime: catchTime, PetID: petID, Name: name,
		Level: level, DV: dv, Nature: nature, InBag: true, BagPos: 99, Skills: skills,
	}
	n, _ := s.CountBagPets(uid)
	if n >= MaxBagPets {
		p.InBag = false
		p.BagPos = -1
	}
	if err = s.UpsertPet(p); err != nil {
		return 0, err
	}
	return catchTime, nil
}

// GrantPetsBatch JSON 后端批量发放（一次读写文档）。
func (s *jsonStore) GrantPetsBatch(uid int64, pets []Pet) (granted int, firstCatch int64, err error) {
	if uid <= 0 {
		return 0, 0, fmt.Errorf("bad uid")
	}
	if len(pets) == 0 {
		return 0, 0, nil
	}
	err = s.withDoc(uid, func(doc *jsonUserDoc) error {
		used := make(map[int64]struct{}, len(doc.Pets)+len(pets))
		bagCount := 0
		for i := range doc.Pets {
			used[doc.Pets[i].CatchTime] = struct{}{}
			if doc.Pets[i].InBag {
				bagCount++
			}
		}
		catchTime := time.Now().Unix()
		for i := range pets {
			p := pets[i]
			if p.PetID <= 0 {
				continue
			}
			if p.Level <= 0 {
				p.Level = 1
			}
			if p.Name == "" {
				p.Name = "精灵"
			}
			if p.Skills == nil {
				p.Skills = []int{10001}
			}
			for {
				if _, ok := used[catchTime]; !ok {
					break
				}
				catchTime++
			}
			used[catchTime] = struct{}{}
			p.UserID = uid
			p.CatchTime = catchTime
			if bagCount < MaxBagPets {
				p.InBag = true
				p.BagPos = 99
				bagCount++
			} else {
				p.InBag = false
				p.BagPos = -1
			}
			doc.Pets = append(doc.Pets, p)
			if firstCatch == 0 {
				firstCatch = catchTime
			}
			granted++
			catchTime++
		}
		return nil
	})
	return granted, firstCatch, err
}
