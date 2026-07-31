package gameserver

import (
	"encoding/binary"
	"fmt"
	"log"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/store"
)

// handleGetSimUserInfo CMD 2051：请求 targetUID(4)；应答对齐 UserInfo.setForSimpleInfo。
func (s *Server) handleGetSimUserInfo(c *Client, uid uint32, body []byte) {
	target := int64(uid)
	if len(body) >= 4 {
		if id := binary.BigEndian.Uint32(body[0:4]); id != 0 {
			target = int64(id)
		}
	}
	out := s.buildSimUserInfo(target)
	s.send(c, 2051, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d target=%d body=%d", cmdname.Format(2051), uid, target, len(out))
}

// handleGetMoreUserInfo CMD 2052：请求 targetUID(4)；应答对齐 UserInfo.setForMoreInfo（本端无 curTopLevel）。
func (s *Server) handleGetMoreUserInfo(c *Client, uid uint32, body []byte) {
	target := int64(uid)
	if len(body) >= 4 {
		if id := binary.BigEndian.Uint32(body[0:4]); id != 0 {
			target = int64(id)
		}
	}
	out := s.buildMoreUserInfo(target)
	s.send(c, 2052, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d target=%d body=%d", cmdname.Format(2052), uid, target, len(out))
}

func fixedNick16(nick string) []byte {
	b := make([]byte, 16)
	nb := []byte(nick)
	if len(nb) > 16 {
		nb = nb[:16]
	}
	copy(b, nb)
	return b
}

func (s *Server) buildSimUserInfo(targetUID int64) []byte {
	u := s.loadUserOrStub(targetUID)
	nick := u.Nickname
	if nick == "" {
		nick = fmt.Sprintf("%d", targetUID)
	}
	mapID := u.MapID
	if mapID <= 0 {
		mapID = defaultMapID
	}
	if oc := s.clientOf(targetUID); oc != nil && oc.MapID > 0 {
		mapID = oc.MapID
	}

	vipFlag, vipLevel := s.userVipFlags(targetUID)
	teacherOK := uint32(0)
	if s.cfg.Store != nil {
		if t, err := s.cfg.Store.GetTask(targetUID, 201); err == nil && t != nil && t.Status >= 3 {
			teacherOK = 1
		}
	}

	var b []byte
	put := func(v uint32) {
		var t [4]byte
		binary.BigEndian.PutUint32(t[:], v)
		b = append(b, t[:]...)
	}
	put(uint32(targetUID))
	b = append(b, fixedNick16(nick)...)
	put(uint32(u.Color))
	put(0) // texture
	put(vipFlag)
	put(0) // status
	put(0) // mapType
	put(uint32(mapID))
	put(teacherOK)
	teacherID, studentID, graduation := s.teacherIDsForUser(targetUID)
	put(teacherID)
	put(studentID)
	put(graduation)
	put(vipLevel)
	ts := s.userTeamSnapshot(targetUID)
	put(ts.ID)
	put(ts.IsShow)

	cloth := s.wornClothIDs(targetUID)
	put(uint32(len(cloth)))
	for _, id := range cloth {
		put(id)
		put(0) // level
	}
	return b
}

func (s *Server) buildMoreUserInfo(targetUID int64) []byte {
	u := s.loadUserOrStub(targetUID)
	nick := u.Nickname
	if nick == "" {
		nick = fmt.Sprintf("%d", targetUID)
	}
	reg := u.RegisterTime
	if reg == 0 {
		reg = 1
	}
	petAll, petMax := s.petCollectStats(targetUID)
	boss := s.buildBossAchievementBytes(targetUID)
	prog := store.UserProgress{}
	if s.cfg.Store != nil {
		prog, _ = s.cfg.Store.GetProgress(targetUID)
	}
	maxStage := prog.BraveMax
	if maxStage < 1 {
		maxStage = 1
	}
	curTitle := s.currentTitleID(targetUID)

	var b []byte
	put := func(v uint32) {
		var t [4]byte
		binary.BigEndian.PutUint32(t[:], v)
		b = append(b, t[:]...)
	}
	put(uint32(targetUID))
	b = append(b, fixedNick16(nick)...)
	put(uint32(reg))
	put(uint32(petAll))
	put(uint32(petMax))
	b = append(b, boss...)
	_, _, graduation := s.teacherIDsForUser(targetUID)
	put(graduation)
	put(0) // monKingWin
	put(0) // messWin
	put(uint32(maxStage))
	put(0) // maxArenaWins
	put(uint32(curTitle))
	return b
}

func (s *Server) loadUserOrStub(uid int64) *store.User {
	if s.cfg.Store != nil {
		if u, err := s.cfg.Store.FindByUserID(uid); err == nil && u != nil {
			return u
		}
	}
	return &store.User{UserID: uid, MapID: defaultMapID}
}

func (s *Server) userVipFlags(uid int64) (vipFlag, vipLevel uint32) {
	n := s.loadNonoForLogin(uid)
	if n == nil || n.HasNono == 0 || n.SuperNono <= 0 {
		return 0, 0
	}
	lv := uint32(n.SuperLevel)
	if lv == 0 {
		lv = 1
	}
	if lv > 5 {
		lv = 5
	}
	return 1, lv
}

func (s *Server) petCollectStats(uid int64) (allNum, maxLev int) {
	if s.cfg.Store == nil {
		return 0, 0
	}
	if bag, err := s.cfg.Store.ListBagPets(uid); err == nil {
		allNum += len(bag)
		for i := range bag {
			if bag[i].Level > maxLev {
				maxLev = bag[i].Level
			}
		}
	}
	if st, err := s.cfg.Store.ListStoragePets(uid); err == nil {
		allNum += len(st)
		for i := range st {
			if st[i].Level > maxLev {
				maxLev = st[i].Level
			}
		}
	}
	return allNum, maxLev
}

func (s *Server) currentTitleID(uid int64) int {
	if s.cfg.Store == nil {
		return 0
	}
	ids, err := s.cfg.Store.ListTitles(uid)
	if err != nil || len(ids) == 0 {
		return 0
	}
	best := ids[0]
	for _, id := range ids[1:] {
		if id > best {
			best = id
		}
	}
	return best
}

// buildBossAchievementBytes 200B：AllUserInfoPanel 用 [0]=301…[19]=320。
func (s *Server) buildBossAchievementBytes(uid int64) []byte {
	out := make([]byte, 200)
	if s.cfg.Store == nil {
		return out
	}
	keys, err := s.cfg.Store.ListDefeatedSPTKeys(uid)
	if err != nil {
		return out
	}
	for _, k := range keys {
		th := k
		if mapped, ok := sptPetToAchieveThreshold[k]; ok {
			th = mapped
		}
		if th < 301 || th >= 301+200 {
			continue
		}
		out[th-301] = 1
	}
	return out
}
