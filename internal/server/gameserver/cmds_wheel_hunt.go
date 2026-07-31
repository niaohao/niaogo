package gameserver

import (
	"encoding/binary"
	"log"
	"math/rand"
	"sync"

	"niaohao/server/internal/cmdname"
)

// ---------- 命运之轮楼层表（本客户端无静态表，服端构造 1–20 层） ----------

type wheelFloorDef struct {
	cardType uint32
	mons     []uint32
	cards    []uint32
	enemyLv  int
}

// wheelFloorTable 尘封命运约 20 层：普怪递增，10/20 层 Boss（PetBook 线索）。
var wheelFloorTable = func() []wheelFloorDef {
	out := make([]wheelFloorDef, 21) // 1-indexed
	baseMons := []uint32{10, 13, 16, 20, 25, 30, 34, 40, 45, 50}
	for f := 1; f <= 20; f++ {
		card := uint32(589 + (f-1)%20)
		ct := uint32(2) // 战斗向
		mons := []uint32{baseMons[(f-1)%len(baseMons)]}
		lv := 20 + f*2
		switch f {
		case 10:
			mons = []uint32{453} // 菲拉斯特
			lv = 50
			ct = 2
		case 20:
			mons = []uint32{816} // 奇德
			lv = 70
			ct = 2
		case 5, 15:
			ct = 7 // 宝箱向（客户端可跳过开战）
			mons = nil
		case 8:
			ct = 8 // 钥匙
			mons = nil
		}
		cards := []uint32{card, uint32(589 + f%20), uint32(590 + f%19)}
		out[f] = wheelFloorDef{cardType: ct, mons: mons, cards: cards, enemyLv: lv}
	}
	return out
}()

type wheelPlayer struct {
	floor int
	mons  []uint32
	lv    int
}

type wheelState struct {
	mu      sync.Mutex
	times   [3]uint32
	high    [3]uint32
	players map[uint32]*wheelPlayer
}

func (s *Server) ensureWheel() {
	s.wheel.mu.Lock()
	defer s.wheel.mu.Unlock()
	if s.wheel.players == nil {
		s.wheel.players = map[uint32]*wheelPlayer{}
	}
	for i := 0; i < 3; i++ {
		if s.wheel.times[i] == 0 && s.wheel.high[i] == 0 {
			s.wheel.times[i] = 5
			s.wheel.high[i] = 1
		}
	}
}

func (s *Server) wheelPlayerOf(uid uint32) *wheelPlayer {
	s.ensureWheel()
	s.wheel.mu.Lock()
	defer s.wheel.mu.Unlock()
	p := s.wheel.players[uid]
	if p == nil {
		p = &wheelPlayer{floor: 1}
		s.wheel.players[uid] = p
	}
	if p.floor < 1 {
		p.floor = 1
	}
	if p.floor > 20 {
		p.floor = 20
	}
	return p
}

// handleTurnFortuneWheel CMD 2452：按当前层回包，随后层数 +1（下次）。
func (s *Server) handleTurnFortuneWheel(c *Client, uid uint32, body []byte) {
	p := s.wheelPlayerOf(uid)
	s.wheel.mu.Lock()
	floor := p.floor
	def := wheelFloorTable[floor]
	if floor < 1 || floor > 20 {
		def = wheelFloorTable[1]
		floor = 1
	}
	mons := append([]uint32(nil), def.mons...)
	cards := append([]uint32(nil), def.cards...)
	p.mons = mons
	p.lv = def.enemyLv
	if p.floor < 20 {
		p.floor++
	}
	s.wheel.mu.Unlock()

	out := make([]byte, 20+4*len(mons)+4+4*len(cards))
	o := 0
	put := func(v uint32) {
		binary.BigEndian.PutUint32(out[o:o+4], v)
		o += 4
	}
	put(2) // wheelType=尘封
	put(def.cardType)
	put(uint32(floor))
	put(1) // difficult
	put(uint32(len(mons)))
	for _, id := range mons {
		put(id)
	}
	put(uint32(len(cards)))
	for _, id := range cards {
		put(id)
	}
	s.send(c, 2452, uid, 0, out[:o])
	log.Printf("[CMD] OK     %s UID=%d floor=%d type=%d mons=%v", cmdname.Format(2452), uid, floor, def.cardType, mons)
}

// handleStartFortuneWheel CMD 2453：用本层怪开战。
func (s *Server) handleStartFortuneWheel(c *Client, uid uint32, body []byte) {
	p := s.wheelPlayerOf(uid)
	s.wheel.mu.Lock()
	enemyID, lv := 10, 30
	if len(p.mons) > 0 {
		enemyID = int(p.mons[0])
	}
	if p.lv > 0 {
		lv = p.lv
	}
	s.wheel.mu.Unlock()
	s.send(c, 2453, uid, 0, nil)
	s.beginFightVsEnemy(c, uid, enemyID, lv, true, fightKindNormal)
	log.Printf("[CMD] OK     %s UID=%d enemy=%d lv=%d", cmdname.Format(2453), uid, enemyID, lv)
}

func (s *Server) handleLeaveFortuneWheel(c *Client, uid uint32, body []byte) {
	s.ensureWheel()
	s.wheel.mu.Lock()
	if p := s.wheel.players[uid]; p != nil {
		p.floor = 1
		p.mons = nil
	}
	s.wheel.mu.Unlock()
	s.send(c, 2454, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(2454), uid)
}

func (s *Server) handleGetWheelChoiceData(c *Client, uid uint32, body []byte) {
	s.ensureWheel()
	s.wheel.mu.Lock()
	out := make([]byte, 24)
	for i := 0; i < 3; i++ {
		binary.BigEndian.PutUint32(out[i*8:i*8+4], s.wheel.times[i])
		binary.BigEndian.PutUint32(out[i*8+4:i*8+8], s.wheel.high[i])
	}
	s.wheel.mu.Unlock()
	s.send(c, 70009, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(70009), uid)
}

func (s *Server) handleEnterWheelChoice(c *Client, uid uint32, body []byte) {
	stage := uint32(1)
	if len(body) >= 4 {
		stage = binary.BigEndian.Uint32(body[0:4])
	}
	s.ensureWheel()
	s.wheel.mu.Lock()
	idx := int(stage) - 1
	if idx >= 0 && idx < 3 && s.wheel.times[idx] > 0 {
		s.wheel.times[idx]--
	}
	if s.wheel.players == nil {
		s.wheel.players = map[uint32]*wheelPlayer{}
	}
	s.wheel.players[uid] = &wheelPlayer{floor: 1}
	s.wheel.mu.Unlock()
	s.send(c, 70010, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d stage=%d", cmdname.Format(70010), uid, stage)
}

// ---------- 狩猎战 80013–80016 ----------

const huntCapsuleItem = 300001

type huntSession struct {
	petID    uint32
	lv       uint32
	capsules uint32
}

type huntHub struct {
	mu   sync.Mutex
	byUID map[uint32]*huntSession
}

func (s *Server) huntOf(uid uint32) *huntSession {
	s.hunt.mu.Lock()
	defer s.hunt.mu.Unlock()
	if s.hunt.byUID == nil {
		s.hunt.byUID = map[uint32]*huntSession{}
	}
	return s.hunt.byUID[uid]
}

func (s *Server) huntSet(uid uint32, st *huntSession) {
	s.hunt.mu.Lock()
	defer s.hunt.mu.Unlock()
	if s.hunt.byUID == nil {
		s.hunt.byUID = map[uint32]*huntSession{}
	}
	if st == nil {
		delete(s.hunt.byUID, uid)
		return
	}
	s.hunt.byUID[uid] = st
}

func (s *Server) handleHuntFightStart(c *Client, uid uint32, body []byte) {
	petID, region := uint32(0), uint32(0)
	if len(body) >= 8 {
		petID = binary.BigEndian.Uint32(body[0:4])
		region = binary.BigEndian.Uint32(body[4:8])
	} else if len(body) >= 4 {
		petID = binary.BigEndian.Uint32(body[0:4])
	}
	if petID == 0 {
		petID = 10
	}
	lv := uint32(20 + region%30)
	capsule := uint32(5)
	if s.cfg.Store != nil {
		if n, err := s.cfg.Store.GetItemCount(int64(uid), huntCapsuleItem); err == nil && n > 0 {
			capsule = uint32(n)
		}
	}
	s.huntSet(uid, &huntSession{petID: petID, lv: lv, capsules: capsule})
	out := make([]byte, 12)
	binary.BigEndian.PutUint32(out[0:4], petID)
	binary.BigEndian.PutUint32(out[4:8], lv)
	binary.BigEndian.PutUint32(out[8:12], capsule)
	s.send(c, 80013, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d pet=%d lv=%d cap=%d", cmdname.Format(80013), uid, petID, lv, capsule)
}

// handleHuntFightAction CMD 80014：应答 type+id+catchResult+catchTime+capsuleCnt。
// 请求布局未知（HuntFight SWF 不在仓）；有胶囊则尝试捕捉。
func (s *Server) handleHuntFightAction(c *Client, uid uint32, body []byte) {
	actType, actID := uint32(1), uint32(huntCapsuleItem)
	if len(body) >= 8 {
		actType = binary.BigEndian.Uint32(body[0:4])
		actID = binary.BigEndian.Uint32(body[4:8])
	} else if len(body) >= 4 {
		actType = binary.BigEndian.Uint32(body[0:4])
	}
	st := s.huntOf(uid)
	if st == nil {
		st = &huntSession{petID: 10, lv: 20, capsules: 5}
	}
	catchResult, catchTime := uint32(0), uint32(0)
	// type==0 视为普通攻击回包；其它视为捕捉尝试
	if actType != 0 && s.cfg.Store != nil {
		capID := int(actID)
		if capID < 300001 || capID > 300010 {
			capID = huntCapsuleItem
		}
		if err := s.cfg.Store.ConsumeItem(int64(uid), capID, 1); err == nil {
			if st.capsules > 0 {
				st.capsules--
			}
			// 约 60% 成功
			if rand.Float64() < 0.6 {
				name := ""
				if s.cfg.Catalog != nil {
					name = s.cfg.Catalog.PetNameOf(int(st.petID))
				}
				ct, err := s.cfg.Store.GrantPet(int64(uid), int(st.petID), name, int(st.lv), rand.Intn(32), rand.Intn(25), nil)
				if err == nil {
					catchResult = 1
					catchTime = uint32(ct)
				}
			}
		} else if n, e := s.cfg.Store.GetItemCount(int64(uid), huntCapsuleItem); e == nil {
			st.capsules = uint32(n)
		}
		s.huntSet(uid, st)
	}
	out := make([]byte, 20)
	binary.BigEndian.PutUint32(out[0:4], actType)
	binary.BigEndian.PutUint32(out[4:8], actID)
	binary.BigEndian.PutUint32(out[8:12], catchResult)
	binary.BigEndian.PutUint32(out[12:16], catchTime)
	binary.BigEndian.PutUint32(out[16:20], st.capsules)
	s.send(c, 80014, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d type=%d catch=%d ct=%d cap=%d",
		cmdname.Format(80014), uid, actType, catchResult, catchTime, st.capsules)
}

func (s *Server) handleHuntFightOver(c *Client, uid uint32, body []byte) {
	s.huntSet(uid, nil)
	out := make([]byte, 4)
	s.send(c, 80015, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(80015), uid)
}

func (s *Server) handleQueryDreamPet(c *Client, uid uint32, body []byte) {
	out := make([]byte, 4)
	s.send(c, 80016, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(80016), uid)
}
