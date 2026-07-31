package gameserver

import (
	"encoding/binary"
	"log"
	"math/rand"
	"time"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/store"
)

// defaultWildDropByEnemy 常见野怪材料（无表时兜底）；能量吸收器翻倍概率用。
var defaultWildDropByEnemy = map[int]int{
	10: 400004, 13: 400008, 16: 400005, 22: 400004, 25: 400023,
	27: 400005, 30: 400006, 33: 400029, 35: 400030, 38: 400007,
	43: 400008, 51: 400003, 52: 400003, 53: 400031, 105: 400031,
	106: 400031, 119: 400032, 120: 400032, 121: 400032, 198: 400029,
	205: 400033, 206: 400033, 208: 400027, 210: 400027, 211: 400034,
	249: 400023, 318: 400030,
}

// grantWildBattleDrops 野外可捕捉胜场材料掉落；EnergyTimes>0 时概率×2 并扣 1 次。
func (s *Server) grantWildBattleDrops(c *Client, uid uint32, st *BattleState) {
	if s.cfg.Store == nil || st == nil || c == nil {
		return
	}
	if st.isPvP() || !st.EnemyCatchable {
		return
	}
	if st.MapID == 500 || st.MapID == 600 || st.FightKind != fightKindNormal {
		return
	}
	itemID, ok := defaultWildDropByEnemy[st.EnemyID]
	if !ok || itemID <= 0 {
		itemID = 300001
	}
	prob := 70
	bt := s.boostTimesOf(int64(uid))
	usedEnergy := false
	if bt.EnergyTimes > 0 {
		prob *= 2
		if prob > 100 {
			prob = 100
		}
		usedEnergy = true
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	if r.Intn(100) >= prob {
		if usedEnergy {
			_, _ = s.cfg.Store.AddEnergyTimes(int64(uid), -1)
		}
		return
	}
	cnt := 1 + r.Intn(2)
	if err := s.cfg.Store.AddItem(int64(uid), itemID, cnt); err != nil {
		log.Printf("[wild-drop] AddItem UID=%d item=%d: %v", uid, itemID, err)
		return
	}
	if usedEnergy {
		_, _ = s.cfg.Store.AddEnergyTimes(int64(uid), -1)
	}
	s.send(c, 8004, uid, 0, buildBossMonster8004Body(0, 0, 0, uint32(itemID), uint32(cnt)))
	log.Printf("[CMD] OK     8004 wild-drop UID=%d enemy=%d item=%d x%d energy=%v",
		uid, st.EnemyID, itemID, cnt, usedEnergy)
}

// handleFastHatch CMD 2375：极速孵化剂 400082 — 立即完成分子转化仪孵化计时。
func (s *Server) handleFastHatch(c *Client, uid uint32, body []byte) {
	s.applyFastHatchNow(c, uid, 2375)
}

// handleFastHatchQuick CMD 9755：无参快捷极速孵化。
func (s *Server) handleFastHatchQuick(c *Client, uid uint32) {
	s.applyFastHatchNow(c, uid, 9755)
}

func (s *Server) applyFastHatchNow(c *Client, uid uint32, cmd int32) {
	out := make([]byte, 8)
	fail := func() {
		s.send(c, cmd, uid, 0, out)
	}
	if s.cfg.Store == nil {
		fail()
		return
	}
	h, err := s.cfg.Store.GetHatchState(int64(uid))
	if err != nil || h.PetID == 0 {
		fail()
		return
	}
	if err := s.cfg.Store.ConsumeItem(int64(uid), 400082, 1); err != nil {
		fail()
		log.Printf("[CMD] OK     %s UID=%d no 400082", cmdname.Format(cmd), uid)
		return
	}
	// 拨回开始时间使 remain<=0，客户端可立刻 2316 领取
	h.StartUnix = time.Now().Unix() - int64(h.Duration) - 1
	_ = s.cfg.Store.SetHatchState(int64(uid), h)
	binary.BigEndian.PutUint32(out[0:4], uint32(h.PetID))
	binary.BigEndian.PutUint32(out[4:8], 0)
	s.send(c, cmd, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d fast hatch pet=%d", cmdname.Format(cmd), uid, h.PetID)
}

// handleSoulBeadFastHatch CMD 80010：60000 赛尔豆立刻完成元神赋形计时。
// 请求 obtainTime(4)+itemID(4)；应答空即可触发面板刷新。
func (s *Server) handleSoulBeadFastHatch(c *Client, uid uint32, body []byte) {
	fail := func() {
		s.send(c, 80010, uid, 0, nil)
	}
	if s.cfg.Store == nil || len(body) < 4 {
		fail()
		return
	}
	obtain := int64(binary.BigEndian.Uint32(body[0:4]))
	const cost = 60000
	bal, ok, err := s.cfg.Store.TrySpendCoins(int64(uid), cost)
	if err != nil || !ok {
		fail()
		log.Printf("[CMD] OK     %s UID=%d no coins need=%d bal=%d", cmdname.Format(80010), uid, cost, bal)
		return
	}
	list, _ := s.cfg.Store.ListSoulBeads(int64(uid))
	var bead *store.SoulBead
	for i := range list {
		if int64(list[i].ObtainTime) == obtain {
			bead = &list[i]
			break
		}
	}
	if bead == nil || bead.TransformStart == 0 {
		_ = s.cfg.Store.AddCoins(int64(uid), cost)
		fail()
		return
	}
	bead.TransformStart = time.Now().Unix() - s.soulBeadTransformDuration(bead.ItemID) - 1
	if err := s.cfg.Store.UpsertSoulBead(int64(uid), *bead); err != nil {
		_ = s.cfg.Store.AddCoins(int64(uid), cost)
		fail()
		return
	}
	s.send(c, 80010, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d obtain=%d", cmdname.Format(80010), uid, obtain)
}
