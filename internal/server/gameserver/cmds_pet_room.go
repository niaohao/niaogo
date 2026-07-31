package gameserver

import (
	"encoding/binary"
	"log"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/store"
)

// handlePetRoomShow CMD 2323 PET_ROOM_SHOW
// 请求：count(4)；count==0 清空；否则 count + [catchTime(4)+petID(4)]*count
// 应答：与 2324 同布局（RoomPetManager.onList）
func (s *Server) handlePetRoomShow(c *Client, uid uint32, body []byte) {
	if s.cfg.Store == nil {
		s.send(c, 2323, uid, 0, buildRoomPetListBody(nil))
		return
	}
	list := store.RoomPets{}
	if len(body) >= 4 {
		count := int(binary.BigEndian.Uint32(body[0:4]))
		if count < 0 {
			count = 0
		}
		if count > 5 {
			count = 5
		}
		off := 4
		for i := 0; i < count && off+8 <= len(body); i++ {
			ct := int64(binary.BigEndian.Uint32(body[off : off+4]))
			off += 8 // skip petID
			if ct > 0 {
				list = append(list, ct)
			}
		}
	}
	if err := s.cfg.Store.SetRoomPets(int64(uid), list); err != nil {
		log.Printf("[CMD] WARN  %s UID=%d SetRoomPets: %v", cmdname.Format(2323), uid, err)
	}
	pets := s.resolveRoomPets(int64(uid), list)
	out := buildRoomPetListBody(pets)
	s.send(c, 2323, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d count=%d", cmdname.Format(2323), uid, len(pets))
}

// handlePetRoomList CMD 2324 PET_ROOM_LIST
// 请求：ownerId(4)；拜访他人基地时传房主 UID（通常=mapID）
// 应答：count(4)+[id(4)+catchTime(4)+skinID(4)]*n（PetListInfo 无 elite）
func (s *Server) handlePetRoomList(c *Client, uid uint32, body []byte) {
	owner := uid
	if len(body) >= 4 {
		if o := binary.BigEndian.Uint32(body[0:4]); o != 0 {
			owner = o
		}
	}
	var pets []store.Pet
	if s.cfg.Store != nil {
		list, _ := s.cfg.Store.GetRoomPets(int64(owner))
		pets = s.resolveRoomPets(int64(owner), list)
	}
	out := buildRoomPetListBody(pets)
	s.send(c, 2324, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d owner=%d count=%d", cmdname.Format(2324), uid, owner, len(pets))
}

// handlePetRoomInfo CMD 2325 PET_ROOM_INFO
// 请求：ownerId(4)+catchTime(4)
// 应答：RoomPetInfo（简略面板）
func (s *Server) handlePetRoomInfo(c *Client, uid uint32, body []byte) {
	owner := uid
	catch := uint32(0)
	if len(body) >= 4 {
		if o := binary.BigEndian.Uint32(body[0:4]); o != 0 {
			owner = o
		}
	}
	if len(body) >= 8 {
		catch = binary.BigEndian.Uint32(body[4:8])
	}
	var p *store.Pet
	if s.cfg.Store != nil && catch > 0 {
		p, _ = s.cfg.Store.GetPetByCatchTime(int64(owner), int64(catch))
	}
	out := buildRoomPetInfoBody(owner, catch, p)
	s.send(c, 2325, uid, 0, out)
	petID := 0
	lv := 0
	if p != nil {
		petID, lv = p.PetID, p.Level
	}
	log.Printf("[CMD] OK     %s UID=%d owner=%d catch=%d pet=%d lv=%d",
		cmdname.Format(2325), uid, owner, catch, petID, lv)
}

func (s *Server) resolveRoomPets(owner int64, list store.RoomPets) []store.Pet {
	out := make([]store.Pet, 0, len(list))
	if s.cfg.Store == nil {
		return out
	}
	for _, ct := range list {
		if len(out) >= 5 {
			break
		}
		p, err := s.cfg.Store.GetPetByCatchTime(owner, ct)
		if err != nil || p == nil {
			continue
		}
		// 展示位通常来自仓库；背包也允许（避免误删条目）
		out = append(out, *p)
	}
	return out
}

func buildRoomPetListBody(pets []store.Pet) []byte {
	// RoomPetManager.onList → new PetListInfo(ba) → 无 isElite，12B/只
	if pets == nil {
		pets = []store.Pet{}
	}
	out := make([]byte, 4+len(pets)*12)
	binary.BigEndian.PutUint32(out[0:4], uint32(len(pets)))
	off := 4
	for i := range pets {
		binary.BigEndian.PutUint32(out[off:off+4], uint32(pets[i].PetID))
		binary.BigEndian.PutUint32(out[off+4:off+8], uint32(pets[i].CatchTime))
		binary.BigEndian.PutUint32(out[off+8:off+12], petSkinID(&pets[i]))
		off += 12
	}
	return out
}

func buildRoomPetInfoBody(owner, catch uint32, p *store.Pet) []byte {
	// owner catch id nature lv hp atk def sa sd spd skillNum [id pp]*N ev*6 effNum(u16)
	petID, nature, level := uint32(0), uint32(0), uint32(5)
	hp, atk, defv, sa, sd, spd := 50, 50, 50, 50, 50, 50
	skills := make([]int, 0, 4)
	ev := [6]int{}
	if p != nil {
		petID = uint32(p.PetID)
		nature = uint32(p.Nature)
		if p.Level > 0 {
			level = uint32(p.Level)
		}
		hp, atk, defv, sa, sd, spd = petSixStatsFromPet(p)
		for i := 0; i < 6 && i < len(p.EV); i++ {
			ev[i] = p.EV[i]
		}
		for i := 0; i < 4 && i < len(p.Skills); i++ {
			if p.Skills[i] > 0 {
				skills = append(skills, p.Skills[i])
			}
		}
		if len(skills) == 0 {
			if def, ok := starterPets[p.PetID]; ok {
				skills = append(skills, def.Skills...)
			}
		}
		if len(skills) > 4 {
			skills = skills[:4]
		}
	}
	nSkill := len(skills)
	out := make([]byte, 4*12+4+nSkill*8+4*6+2)
	off := 0
	put := func(v uint32) {
		binary.BigEndian.PutUint32(out[off:off+4], v)
		off += 4
	}
	put(owner)
	put(catch)
	put(petID)
	put(nature)
	put(level)
	put(uint32(hp))
	put(uint32(atk))
	put(uint32(defv))
	put(uint32(sa))
	put(uint32(sd))
	put(uint32(spd))
	put(uint32(nSkill))
	for _, sid := range skills {
		put(uint32(sid))
		put(skillPPForBuild(sid))
	}
	for i := 0; i < 6; i++ {
		put(uint32(ev[i]))
	}
	binary.BigEndian.PutUint16(out[off:off+2], 0)
	off += 2
	return out[:off]
}
