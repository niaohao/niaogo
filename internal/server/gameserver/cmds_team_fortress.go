package gameserver

import (
	"encoding/binary"
	"log"
	"time"

	"niaohao/server/internal/cmdname"
)

const (
	armDefaultFrame = 800001 // SolidType.FRAME
	armDefaultHQ    = 1
	armUpBuyCost    = 1000
	armEntry2941    = 20
	armEntry2942    = 12
	armEntry2965    = 56 // setFor2967_2965：14×u32
	armEntry2966    = 16
)

func (s *Server) teamArmResolveTeamID(uid uint32, body []byte) uint32 {
	s.initTeamHub()
	h := s.teams
	h.mu.Lock()
	defer h.mu.Unlock()
	if tid := h.uidIndex[int64(uid)]; tid != 0 {
		return tid
	}
	if len(body) >= 4 {
		req := binary.BigEndian.Uint32(body[0:4])
		if req >= teamMinID {
			return req
		}
	}
	return 0
}

func (s *Server) teamArmSendRetZero(c *Client, uid uint32, cmd int32) {
	out := make([]byte, 4)
	s.send(c, cmd, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(cmd), uid)
}

func (s *Server) teamArmSendEmpty(c *Client, uid uint32, cmd int32) {
	s.send(c, cmd, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(cmd), uid)
}

func (t *teamRuntime) ensureArmDefaults() {
	if t == nil {
		return
	}
	if t.HeadquartersID == 0 {
		t.HeadquartersID = armDefaultHQ
	}
	if t.Arms == nil {
		t.Arms = []teamArmPlace{}
	}
	if t.ArmStock == nil {
		t.ArmStock = []teamArmStock{}
	}
	if t.UpArms == nil {
		t.UpArms = []teamArmUp{}
	}
	// 空装饰时给一个默认房型，避免地图无 FRAME
	hasFrame := false
	for _, a := range t.Arms {
		if a.ID >= 800001 && a.ID <= 800200 {
			hasFrame = true
			break
		}
	}
	if !hasFrame {
		t.Arms = append(t.Arms, teamArmPlace{ID: armDefaultFrame, X: 480, Y: 280})
	}
	if len(t.ArmStock) == 0 {
		t.ArmStock = []teamArmStock{{ID: armDefaultFrame, Used: 1, All: 1}}
	}
}

func (s *Server) teamArmOf(uid uint32, body []byte) (uint32, *teamRuntime) {
	s.initTeamHub()
	teamID := s.teamArmResolveTeamID(uid, body)
	if teamID == 0 {
		return 0, nil
	}
	h := s.teams
	h.mu.Lock()
	defer h.mu.Unlock()
	t := h.teams[teamID]
	if t != nil {
		t.ensureArmDefaults()
	}
	return teamID, t
}

func (s *Server) teamArmSave() {
	if s.teams == nil {
		return
	}
	s.teams.mu.Lock()
	s.teams.saveLocked()
	s.teams.mu.Unlock()
}

// handleTeamArmGetUsedInfo CMD 2941：teamID+hqID+count + n×20B。
func (s *Server) handleTeamArmGetUsedInfo(c *Client, uid uint32, body []byte) {
	teamID, t := s.teamArmOf(uid, body)
	hq, arms := uint32(armDefaultHQ), []teamArmPlace{}
	if t != nil {
		hq = t.HeadquartersID
		arms = t.Arms
	}
	out := make([]byte, 12+len(arms)*armEntry2941)
	binary.BigEndian.PutUint32(out[0:4], teamID)
	binary.BigEndian.PutUint32(out[4:8], hq)
	binary.BigEndian.PutUint32(out[8:12], uint32(len(arms)))
	off := 12
	for _, a := range arms {
		binary.BigEndian.PutUint32(out[off:off+4], a.ID)
		binary.BigEndian.PutUint32(out[off+4:off+8], a.X)
		binary.BigEndian.PutUint32(out[off+8:off+12], a.Y)
		binary.BigEndian.PutUint32(out[off+12:off+16], a.Dir)
		binary.BigEndian.PutUint32(out[off+16:off+20], a.Status)
		off += armEntry2941
	}
	s.send(c, 2941, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d teamID=%d count=%d", cmdname.Format(2941), uid, teamID, len(arms))
}

// handleTeamArmGetAllInfo CMD 2942：teamID+count + n×12B。
func (s *Server) handleTeamArmGetAllInfo(c *Client, uid uint32, body []byte) {
	teamID, t := s.teamArmOf(uid, body)
	stock := []teamArmStock{}
	if t != nil {
		stock = t.ArmStock
	}
	out := make([]byte, 8+len(stock)*armEntry2942)
	binary.BigEndian.PutUint32(out[0:4], teamID)
	binary.BigEndian.PutUint32(out[4:8], uint32(len(stock)))
	off := 8
	for _, a := range stock {
		binary.BigEndian.PutUint32(out[off:off+4], a.ID)
		binary.BigEndian.PutUint32(out[off+4:off+8], a.Used)
		binary.BigEndian.PutUint32(out[off+8:off+12], a.All)
		off += armEntry2942
	}
	s.send(c, 2942, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d teamID=%d count=%d", cmdname.Format(2942), uid, teamID, len(stock))
}

func (s *Server) handleTeamArmPlace2943(c *Client, uid uint32, body []byte) {
	s.teamArmSendEmpty(c, uid, 2943)
}

// handleTeamArmSetInfo2944 CMD 2944：请求 count + n×(id,x,y,dir,status)；空 ACK。
func (s *Server) handleTeamArmSetInfo2944(c *Client, uid uint32, body []byte) {
	s.initTeamHub()
	teamID := s.teamArmResolveTeamID(uid, nil)
	if teamID != 0 && len(body) >= 4 {
		count := int(binary.BigEndian.Uint32(body[0:4]))
		if count < 0 {
			count = 0
		}
		if count > 64 {
			count = 64
		}
		arms := make([]teamArmPlace, 0, count)
		off := 4
		for i := 0; i < count && off+armEntry2941 <= len(body); i++ {
			a := teamArmPlace{
				ID:     binary.BigEndian.Uint32(body[off : off+4]),
				X:      binary.BigEndian.Uint32(body[off+4 : off+8]),
				Y:      binary.BigEndian.Uint32(body[off+8 : off+12]),
				Dir:    binary.BigEndian.Uint32(body[off+12 : off+16]),
				Status: binary.BigEndian.Uint32(body[off+16 : off+20]),
			}
			off += armEntry2941
			if a.ID == 0 {
				continue
			}
			arms = append(arms, a)
		}
		s.teams.mu.Lock()
		if t := s.teams.teams[teamID]; t != nil {
			t.ensureArmDefaults()
			t.Arms = arms
			// 同步仓库 used 计数
			usedByID := map[uint32]uint32{}
			for _, a := range arms {
				usedByID[a.ID]++
			}
			for i := range t.ArmStock {
				t.ArmStock[i].Used = usedByID[t.ArmStock[i].ID]
				if t.ArmStock[i].All < t.ArmStock[i].Used {
					t.ArmStock[i].All = t.ArmStock[i].Used
				}
				delete(usedByID, t.ArmStock[i].ID)
			}
			for id, u := range usedByID {
				t.ArmStock = append(t.ArmStock, teamArmStock{ID: id, Used: u, All: u})
			}
			s.teams.saveLocked()
		}
		s.teams.mu.Unlock()
	}
	s.send(c, 2944, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d teamID=%d", cmdname.Format(2944), uid, teamID)
}

func (s *Server) handleTeamArmTakeBack2945(c *Client, uid uint32, body []byte) {
	s.teamArmSendEmpty(c, uid, 2945)
}

func (s *Server) handleTeamArm2951(c *Client, uid uint32, body []byte) {
	s.teamArmSendEmpty(c, uid, 2951)
}

func (s *Server) handleTeamArm2952(c *Client, uid uint32, body []byte) {
	s.teamArmSendEmpty(c, uid, 2952)
}

func (s *Server) handleTeamArm2953(c *Client, uid uint32, body []byte) {
	s.teamArmSendEmpty(c, uid, 2953)
}

func (s *Server) handleTeamArm2954(c *Client, uid uint32, body []byte) {
	s.teamArmSendEmpty(c, uid, 2954)
}

// handleTeamArmUpBuy2961 CMD 2961：请求 id；应答 coins+id+form+buyTime。
func (s *Server) handleTeamArmUpBuy2961(c *Client, uid uint32, body []byte) {
	itemID := uint32(0)
	if len(body) >= 4 {
		itemID = binary.BigEndian.Uint32(body[0:4])
	}
	coins := uint32(0)
	buyTime := uint32(time.Now().Unix())
	form := uint32(1)
	if s.cfg.Store != nil {
		if u, err := s.cfg.Store.FindByUserID(int64(uid)); err == nil && u != nil {
			coins = uint32(u.Coins)
		}
		if itemID > 0 {
			if bal, ok, err := s.cfg.Store.TrySpendCoins(int64(uid), armUpBuyCost); err == nil && ok {
				coins = uint32(bal)
			}
		}
	}
	s.initTeamHub()
	teamID := s.teamArmResolveTeamID(uid, nil)
	if teamID != 0 && itemID > 0 {
		s.teams.mu.Lock()
		if t := s.teams.teams[teamID]; t != nil {
			t.ensureArmDefaults()
			t.UpArms = append(t.UpArms, teamArmUp{
				ID: itemID, BuyTime: buyTime, Form: form, HP: 100,
			})
			s.teams.saveLocked()
		}
		s.teams.mu.Unlock()
	}
	out := make([]byte, 16)
	binary.BigEndian.PutUint32(out[0:4], coins)
	binary.BigEndian.PutUint32(out[4:8], itemID)
	binary.BigEndian.PutUint32(out[8:12], form)
	binary.BigEndian.PutUint32(out[12:16], buyTime)
	s.send(c, 2961, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d id=%d buyTime=%d", cmdname.Format(2961), uid, itemID, buyTime)
}

func (s *Server) handleTeamArmUpWork2962(c *Client, uid uint32, body []byte) {
	s.teamArmSendRetZero(c, uid, 2962)
}

func (s *Server) handleTeamArmUpDonate2963(c *Client, uid uint32, body []byte) {
	s.teamArmSendRetZero(c, uid, 2963)
}

// handleTeamArmUpSetInfo2964：请求 count + n×(id,buyTime,x,y,dir,status)；回 teamID+count0 或空。
func (s *Server) handleTeamArmUpSetInfo2964(c *Client, uid uint32, body []byte) {
	s.initTeamHub()
	teamID := s.teamArmResolveTeamID(uid, nil)
	if teamID != 0 && len(body) >= 4 {
		count := int(binary.BigEndian.Uint32(body[0:4]))
		if count > 64 {
			count = 64
		}
		off := 4
		placed := map[uint32]teamArmUp{}
		for i := 0; i < count && off+24 <= len(body); i++ {
			id := binary.BigEndian.Uint32(body[off : off+4])
			bt := binary.BigEndian.Uint32(body[off+4 : off+8])
			x := binary.BigEndian.Uint32(body[off+8 : off+12])
			y := binary.BigEndian.Uint32(body[off+12 : off+16])
			dir := binary.BigEndian.Uint32(body[off+16 : off+20])
			st := binary.BigEndian.Uint32(body[off+20 : off+24])
			off += 24
			if id == 1 || bt == 0 {
				continue
			}
			placed[bt] = teamArmUp{ID: id, BuyTime: bt, X: x, Y: y, Dir: dir, Status: st, IsUsed: true, Form: 1, HP: 100}
		}
		s.teams.mu.Lock()
		if t := s.teams.teams[teamID]; t != nil {
			t.ensureArmDefaults()
			for i := range t.UpArms {
				if p, ok := placed[t.UpArms[i].BuyTime]; ok {
					t.UpArms[i].X = p.X
					t.UpArms[i].Y = p.Y
					t.UpArms[i].Dir = p.Dir
					t.UpArms[i].Status = p.Status
					t.UpArms[i].IsUsed = true
					delete(placed, t.UpArms[i].BuyTime)
				} else {
					t.UpArms[i].IsUsed = false
					t.UpArms[i].X, t.UpArms[i].Y = 0, 0
				}
			}
			for _, p := range placed {
				t.UpArms = append(t.UpArms, p)
			}
			s.teams.saveLocked()
		}
		s.teams.mu.Unlock()
	}
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], teamID)
	s.send(c, 2964, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d teamID=%d", cmdname.Format(2964), uid, teamID)
}

func encodeArmUp2965(a teamArmUp) []byte {
	out := make([]byte, armEntry2965)
	binary.BigEndian.PutUint32(out[0:4], a.ID)
	binary.BigEndian.PutUint32(out[4:8], a.BuyTime)
	binary.BigEndian.PutUint32(out[8:12], a.Form)
	binary.BigEndian.PutUint32(out[12:16], a.HP)
	binary.BigEndian.PutUint32(out[16:20], a.Work)
	binary.BigEndian.PutUint32(out[20:24], a.Donate)
	for i := 0; i < 4; i++ {
		binary.BigEndian.PutUint32(out[24+i*4:28+i*4], a.Res[i])
	}
	binary.BigEndian.PutUint32(out[40:44], a.X)
	binary.BigEndian.PutUint32(out[44:48], a.Y)
	binary.BigEndian.PutUint32(out[48:52], a.Dir)
	binary.BigEndian.PutUint32(out[52:56], a.Status)
	return out
}

// handleTeamArmUpGetUsedInfo2965：teamID+count + n×56B（仅已摆放）。
func (s *Server) handleTeamArmUpGetUsedInfo2965(c *Client, uid uint32, body []byte) {
	teamID, t := s.teamArmOf(uid, body)
	used := []teamArmUp{}
	if t != nil {
		for _, a := range t.UpArms {
			if a.IsUsed {
				used = append(used, a)
			}
		}
	}
	out := make([]byte, 8+len(used)*armEntry2965)
	binary.BigEndian.PutUint32(out[0:4], teamID)
	binary.BigEndian.PutUint32(out[4:8], uint32(len(used)))
	off := 8
	for _, a := range used {
		copy(out[off:], encodeArmUp2965(a))
		off += armEntry2965
	}
	s.send(c, 2965, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d teamID=%d count=%d", cmdname.Format(2965), uid, teamID, len(used))
}

// handleTeamArmUpGetAllInfo2966：teamID+count + n×16B。
func (s *Server) handleTeamArmUpGetAllInfo2966(c *Client, uid uint32, body []byte) {
	teamID, t := s.teamArmOf(uid, body)
	list := []teamArmUp{}
	if t != nil {
		list = t.UpArms
	}
	out := make([]byte, 8+len(list)*armEntry2966)
	binary.BigEndian.PutUint32(out[0:4], teamID)
	binary.BigEndian.PutUint32(out[4:8], uint32(len(list)))
	off := 8
	for _, a := range list {
		binary.BigEndian.PutUint32(out[off:off+4], a.BuyTime)
		binary.BigEndian.PutUint32(out[off+4:off+8], a.ID)
		binary.BigEndian.PutUint32(out[off+8:off+12], a.Form)
		used := uint32(0)
		if a.IsUsed {
			used = 1
		}
		binary.BigEndian.PutUint32(out[off+12:off+16], used)
		off += armEntry2966
	}
	s.send(c, 2966, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d teamID=%d count=%d", cmdname.Format(2966), uid, teamID, len(list))
}

// handleTeamArmUpGetOneInfo2967：请求 teamID+buyTime；应答 56B 建筑详情。
func (s *Server) handleTeamArmUpGetOneInfo2967(c *Client, uid uint32, body []byte) {
	buyTime := uint32(0)
	if len(body) >= 8 {
		buyTime = binary.BigEndian.Uint32(body[4:8])
	} else if len(body) >= 4 {
		buyTime = binary.BigEndian.Uint32(body[0:4])
	}
	_, t := s.teamArmOf(uid, body)
	var found teamArmUp
	if t != nil {
		for _, a := range t.UpArms {
			if a.BuyTime == buyTime || (buyTime == 0 && a.ID != 0) {
				found = a
				break
			}
		}
	}
	if found.ID == 0 {
		found.HP = 100
		found.Form = 1
		found.BuyTime = buyTime
	}
	out := encodeArmUp2965(found)
	s.send(c, 2967, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d buyTime=%d id=%d", cmdname.Format(2967), uid, buyTime, found.ID)
}

func (s *Server) handleTeamArmBuy2968(c *Client, uid uint32, body []byte) {
	itemID := uint32(0)
	if len(body) >= 4 {
		itemID = binary.BigEndian.Uint32(body[0:4])
	}
	coins := uint32(0)
	if s.cfg.Store != nil {
		if u, err := s.cfg.Store.FindByUserID(int64(uid)); err == nil && u != nil {
			coins = uint32(u.Coins)
		}
	}
	s.initTeamHub()
	teamID := s.teamArmResolveTeamID(uid, nil)
	if teamID != 0 && itemID > 0 {
		s.teams.mu.Lock()
		if t := s.teams.teams[teamID]; t != nil {
			t.ensureArmDefaults()
			found := false
			for i := range t.ArmStock {
				if t.ArmStock[i].ID == itemID {
					t.ArmStock[i].All++
					found = true
					break
				}
			}
			if !found {
				t.ArmStock = append(t.ArmStock, teamArmStock{ID: itemID, All: 1})
			}
			s.teams.saveLocked()
		}
		s.teams.mu.Unlock()
	}
	out := make([]byte, 12)
	binary.BigEndian.PutUint32(out[0:4], coins)
	binary.BigEndian.PutUint32(out[4:8], itemID)
	binary.BigEndian.PutUint32(out[8:12], 1)
	s.send(c, 2968, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d id=%d", cmdname.Format(2968), uid, itemID)
}

func (s *Server) handleTeamArmUpOpenUpdate2969(c *Client, uid uint32, body []byte) {
	s.teamArmSendEmpty(c, uid, 2969)
}

func (s *Server) handleTeamArmUpGetUpdate2970(c *Client, uid uint32, body []byte) {
	s.teamArmSendRetZero(c, uid, 2970)
}
