package gameserver

import (
	"encoding/binary"
	"log"
	"strconv"
	"time"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/store"
)

const (
	superNonoMaxLevel         = 15
	superNonoActivateGoldCost = 10
	superNonoDurationDays     = 30
)

// handleNonoOpen CMD 9001：开通普通 NoNo；应答 endNum(4)，0=已有，≠0=成功；再推 9003。
func (s *Server) handleNonoOpen(c *Client, uid uint32) {
	endNum := uint32(1)
	if s.cfg.Store != nil {
		n, err := s.cfg.Store.GetOrInitNono(int64(uid))
		if err != nil {
			log.Printf("[CMD] WARN  %s UID=%d get: %v", cmdname.Format(9001), uid, err)
		} else if n.HasNono != 0 {
			endNum = 0
		} else {
			n.HasNono = 1
			n.Flag = 1
			if n.Nick == "" {
				n.Nick = "NoNo"
			}
			if n.Birth == 0 {
				n.Birth = time.Now().Unix()
			}
			if err := s.cfg.Store.UpsertNono(n); err != nil {
				log.Printf("[CMD] WARN  %s UID=%d upsert: %v", cmdname.Format(9001), uid, err)
			}
		}
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, endNum)
	s.send(c, 9001, uid, 0, out)
	s.pushNonoInfo(c, uid)
	log.Printf("[CMD] OK     %s UID=%d endNum=%d", cmdname.Format(9001), uid, endNum)
}

// handleNonoChangeName CMD 9002：请求 nick16；应答 userID+nick16。
func (s *Server) handleNonoChangeName(c *Client, uid uint32, body []byte) {
	nick := "NoNo"
	if len(body) >= 16 {
		nb := body[:16]
		for len(nb) > 0 && nb[len(nb)-1] == 0 {
			nb = nb[:len(nb)-1]
		}
		if len(nb) > 0 {
			nick = string(nb)
		}
	}
	if s.cfg.Store != nil {
		n, _ := s.cfg.Store.GetOrInitNono(int64(uid))
		if n.HasNono == 0 {
			n.HasNono = 1
			n.Flag = 1
		}
		n.Nick = nick
		_ = s.cfg.Store.UpsertNono(n)
	}
	out := make([]byte, 20)
	binary.BigEndian.PutUint32(out[0:4], uid)
	putFixedNick(out, 4, nick)
	s.send(c, 9002, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d nick=%q", cmdname.Format(9002), uid, nick)
}

// handleNonoInfo CMD 9003：90 字节 NonoInfo。
func (s *Server) handleNonoInfo(c *Client, uid uint32) {
	s.pushNonoInfo(c, uid)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(9003), uid)
}

func (s *Server) pushNonoInfo(c *Client, uid uint32) {
	s.send(c, 9003, uid, 0, s.buildNonoInfoBody(int64(uid)))
}

func (s *Server) buildNonoInfoBody(uid int64) []byte {
	var n *store.Nono
	if s.cfg.Store != nil {
		n, _ = s.cfg.Store.GetOrInitNono(uid)
	}
	if n == nil {
		n = store.DefaultNono(uid)
	}
	s.applySuperNonoExpiry(n)
	s.syncSuperNonoStage(n)
	return buildNonoInfo90From(n)
}

func buildNonoInfo90From(n *store.Nono) []byte {
	buf := make([]byte, 0, 90)
	w32 := func(v uint32) {
		tmp := make([]byte, 4)
		binary.BigEndian.PutUint32(tmp, v)
		buf = append(buf, tmp...)
	}
	w16 := func(v uint16) {
		tmp := make([]byte, 2)
		binary.BigEndian.PutUint16(tmp, v)
		buf = append(buf, tmp...)
	}
	wStr := func(s string, size int) {
		b := []byte(s)
		if len(b) > size {
			b = b[:size]
		}
		buf = append(buf, b...)
		if len(b) < size {
			buf = append(buf, make([]byte, size-len(b))...)
		}
	}
	uid := uint32(0)
	if n != nil {
		uid = uint32(n.UserID)
	}
	w32(uid)
	flag, state := uint32(0), uint32(0)
	nick := ""
	superNono, color := uint32(0), uint32(0)
	power, mate, iq := uint32(0), uint32(0), uint32(0)
	ai := uint16(0)
	birth := uint32(time.Now().Unix())
	charge := uint32(0)
	funcBits := make([]byte, 20)
	superEnergy, superLevel, superStage := uint32(0), uint32(0), uint32(0)
	if n != nil && n.HasNono != 0 {
		flag = uint32(n.Flag)
		if flag == 0 {
			flag = 1
		}
		state = uint32(n.State)
		nick = n.Nick
		superNono = uint32(n.SuperNono)
		// 飞行全面开启：包内至少视为超能，解锁飞行面板与快捷芯片位
		if superNono == 0 {
			superNono = 1
		}
		color = uint32(n.Color)
		power = uint32(n.Power * 1000)
		mate = uint32(n.Mate * 1000)
		iq = uint32(n.IQ)
		ai = uint16(n.AI)
		if n.Birth > 0 {
			birth = uint32(n.Birth)
		}
		charge = uint32(n.ChargeTime)
		for i := range funcBits {
			funcBits[i] = 0xFF
		}
		superEnergy = uint32(n.SuperEnergy)
		superLevel = uint32(n.SuperLevel)
		superStage = uint32(n.SuperStage)
		if superStage < 5 {
			superStage = 5
		}
	}
	w32(flag)
	w32(state)
	wStr(nick, 16)
	w32(superNono)
	w32(color)
	w32(power)
	w32(mate)
	w32(iq)
	w16(ai)
	w32(birth)
	w32(charge)
	buf = append(buf, funcBits...)
	w32(superEnergy)
	w32(superLevel)
	w32(superStage)
	return buf
}

// handleNonoExeList CMD 9015：训练中精灵列表。
// 应答：count + N×(flag+capTm+petId+remainSec+course)，每条 20B。
func (s *Server) handleNonoExeList(c *Client, uid uint32) {
	var pets []store.Pet
	if s.cfg.Store != nil {
		pets, _ = s.cfg.Store.ListExePets(int64(uid))
	}
	out := make([]byte, 4+len(pets)*20)
	binary.BigEndian.PutUint32(out[0:4], uint32(len(pets)))
	now := time.Now().Unix()
	off := 4
	for i := range pets {
		p := &pets[i]
		course := p.ExeCourse
		if course < 1 {
			course = 1
		}
		totalSec := int64(course) * 24 * 3600
		remain := totalSec
		if p.ExeStart > 0 {
			elapsed := now - p.ExeStart
			if elapsed < 0 {
				elapsed = 0
			}
			remain = totalSec - elapsed
			if remain < 0 {
				remain = 0
			}
		}
		binary.BigEndian.PutUint32(out[off:off+4], 0) // flag
		binary.BigEndian.PutUint32(out[off+4:off+8], uint32(p.CatchTime))
		binary.BigEndian.PutUint32(out[off+8:off+12], uint32(p.PetID))
		binary.BigEndian.PutUint32(out[off+12:off+16], uint32(remain))
		binary.BigEndian.PutUint32(out[off+16:off+20], uint32(course))
		off += 20
	}
	s.send(c, 9015, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d count=%d", cmdname.Format(9015), uid, len(pets))
}

// handleNonoStartExe CMD 9017：开始模拟训练。请求 capTime+type；应答 cap+petId+0+course。
func (s *Server) handleNonoStartExe(c *Client, uid uint32, body []byte) {
	capTime, course := uint32(0), uint32(1)
	if len(body) >= 4 {
		capTime = binary.BigEndian.Uint32(body[0:4])
	}
	if len(body) >= 8 {
		course = binary.BigEndian.Uint32(body[4:8])
	}
	if course < 1 {
		course = 1
	}
	if course > 30 {
		course = 30
	}
	petID := uint32(0)
	if s.cfg.Store != nil && capTime != 0 {
		p, _ := s.cfg.Store.GetPetByCatchTime(int64(uid), int64(capTime))
		if p != nil {
			petID = uint32(p.PetID)
			_ = s.cfg.Store.MovePetToExe(int64(uid), int64(capTime), int(course), time.Now().Unix())
		}
	}
	out := make([]byte, 16)
	binary.BigEndian.PutUint32(out[0:4], capTime)
	binary.BigEndian.PutUint32(out[4:8], petID)
	binary.BigEndian.PutUint32(out[8:12], 0)
	binary.BigEndian.PutUint32(out[12:16], course)
	s.send(c, 9017, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d catch=%d pet=%d course=%d", cmdname.Format(9017), uid, capTime, petID, course)
}

// handleNonoEndExe CMD 9018：结束训练。请求 cap；应答 gainedExp(4)。
func (s *Server) handleNonoEndExe(c *Client, uid uint32, body []byte) {
	capTime := uint32(0)
	if len(body) >= 4 {
		capTime = binary.BigEndian.Uint32(body[0:4])
	}
	gained := uint32(0)
	if s.cfg.Store != nil && capTime != 0 {
		p, _ := s.cfg.Store.GetPetByCatchTime(int64(uid), int64(capTime))
		if p != nil && p.BagPos == store.ExeBagPos {
			now := time.Now().Unix()
			elapsedH := int64(0)
			if p.ExeStart > 0 && now > p.ExeStart {
				elapsedH = (now - p.ExeStart) / 3600
			}
			course := p.ExeCourse
			if course < 1 {
				course = 1
			}
			gain := int(elapsedH)*(10+p.Level)*course + 50*course
			if gain < 0 {
				gain = 0
			}
			if gain > 50000 {
				gain = 50000
			}
			oldLv := p.Level
			used := applyPetExpGain(p, gain)
			note := s.afterPetLevelChange(p, oldLv)
			ended, err := s.cfg.Store.EndPetExe(int64(uid), int64(capTime))
			if err == nil && ended != nil {
				ended.Level, ended.Exp, ended.Skills = p.Level, p.Exp, p.Skills
				_ = s.cfg.Store.UpsertPet(ended)
				if len(note) > 0 {
					s.send(c, 2507, uid, 0, note)
				}
				s.pushPetPropAndInfo(c, uid, ended)
			} else {
				_ = s.cfg.Store.UpsertPet(p)
			}
			gained = uint32(used)
		}
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, gained)
	s.send(c, 9018, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d catch=%d exp=%d", cmdname.Format(9018), uid, capTime, gained)
}

// handleNonoCloseOpen CMD 9014：开关 NoNo。应答 uid+isOpen。
func (s *Server) handleNonoCloseOpen(c *Client, uid uint32, body []byte) {
	on := uint32(0)
	if len(body) >= 4 {
		on = binary.BigEndian.Uint32(body[0:4])
	}
	if s.cfg.Store != nil {
		n, _ := s.cfg.Store.GetOrInitNono(int64(uid))
		if on != 0 {
			n.Flag = 1
			if n.HasNono == 0 {
				n.HasNono = 1
			}
		} else {
			n.Flag = 0
		}
		_ = s.cfg.Store.UpsertNono(n)
	}
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], uid)
	binary.BigEndian.PutUint32(out[4:8], on)
	s.send(c, 9014, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d on=%d", cmdname.Format(9014), uid, on)
}

// handleNonoHelpExp CMD 9021：发明室帮助经验 → 积累经验（攻略 10000，日限 1 次）。
func (s *Server) handleNonoHelpExp(c *Client, uid uint32) {
	const add = 10000
	granted := false
	if s.cfg.Store != nil && s.tryMarkDaily(int64(uid), "nonoHelpExp") {
		_, _ = s.cfg.Store.AddExpPool(int64(uid), add)
		granted = true
		s.sendAlert(int64(uid), "获得积累经验 "+strconv.Itoa(add))
	} else if s.cfg.Store != nil {
		s.sendAlert(int64(uid), "今日发明室经验已领取")
	}
	s.send(c, 9021, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d granted=%v pool+%d", cmdname.Format(9021), uid, granted, add)
}

// handleNonoMateChange CMD 9022：心情回满。
func (s *Server) handleNonoMateChange(c *Client, uid uint32) {
	if s.cfg.Store != nil {
		n, _ := s.cfg.Store.GetOrInitNono(int64(uid))
		n.Mate = 100
		_ = s.cfg.Store.UpsertNono(n)
	}
	s.send(c, 9022, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(9022), uid)
}

// handleNonoAddEnergyMate CMD 9024：加能量心情。
func (s *Server) handleNonoAddEnergyMate(c *Client, uid uint32) {
	if s.cfg.Store != nil {
		n, _ := s.cfg.Store.GetOrInitNono(int64(uid))
		n.Power = clampNonoStat(n.Power + 10)
		n.Mate = clampNonoStat(n.Mate + 10)
		_ = s.cfg.Store.UpsertNono(n)
		s.pushNonoInfo(c, uid)
	}
	s.send(c, 9024, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(9024), uid)
}

// handleNonoMapPetExp CMD 9026 / handleNonoIsInfo CMD 9027：地图给首发精灵经验。
// 客户端常发空包体（如 MapProcess_56 的 NONO_ADD_EXP）或仅 type(4)；必须回 gainedExp(4)，禁空 ACK。
func (s *Server) handleNonoMapPetExp(c *Client, uid uint32, cmd int32, body []byte) {
	typ := uint32(0)
	if len(body) >= 4 {
		typ = binary.BigEndian.Uint32(body[0:4])
	}
	gain := 800 + int(uid%7)*200 // 800–2000 档
	if typ >= 2 {
		gain += 1000 // 地图 boss/强化档略高
	}
	if gain > 3500 {
		gain = 3500
	}
	used := s.grantMapPetExp(c, uid, gain)
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, uint32(used))
	s.send(c, cmd, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d type=%d exp=%d", cmdname.Format(cmd), uid, typ, used)
}

// 发明室超能芯片领取：每页 3 个，共 4 页（本客户端面板顺序）。
var superNonoChipList = []uint32{
	700019, 700007, 700009,
	700001, 700002, 700003,
	700004, 700005, 700006,
	700008, 700010, 700011,
}

func setNonoFuncBit(bits []byte, itemID uint32) {
	if itemID < 700001 {
		return
	}
	idx := int(itemID - 700001)
	if idx >= 160 {
		return
	}
	if len(bits) < 20 {
		return
	}
	bits[idx/8] |= 1 << uint(idx%8)
}

func normalizeNonoFuncBits(b []byte) []byte {
	out := make([]byte, 20)
	copy(out, b)
	return out
}

func clampNonoStat(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func isNonoChipItemID(itemID uint32) bool {
	return (itemID >= 700001 && itemID <= 700399) || itemID == 700061
}

func (s *Server) applyNonoItemFx(n *store.Nono, itemID uint32) {
	if n == nil || s.cfg.Catalog == nil {
		return
	}
	fx, ok := s.cfg.Catalog.NonoItemOf(int(itemID))
	if !ok {
		return
	}
	if fx.AddPower != 0 {
		n.Power = clampNonoStat(n.Power + fx.AddPower)
	}
	if fx.AddCloseness != 0 {
		n.Mate = clampNonoStat(n.Mate + fx.AddCloseness)
	}
	if fx.AddIQ != 0 {
		n.IQ += fx.AddIQ
		if n.IQ < 0 {
			n.IQ = 0
		}
	}
}

func (s *Server) buildNonoToolBody(uid uint32, itemID uint32, n *store.Nono) []byte {
	power, mate, iq := uint32(0), uint32(0), uint32(0)
	ai := uint16(0)
	if n != nil {
		power = uint32(n.Power * 1000)
		mate = uint32(n.Mate * 1000)
		iq = uint32(n.IQ)
		ai = uint16(n.AI)
	}
	body := make([]byte, 22)
	binary.BigEndian.PutUint32(body[0:4], uid)
	binary.BigEndian.PutUint32(body[4:8], itemID)
	binary.BigEndian.PutUint32(body[8:12], power)
	binary.BigEndian.PutUint16(body[12:14], ai)
	binary.BigEndian.PutUint32(body[14:18], mate)
	binary.BigEndian.PutUint32(body[18:22], iq)
	return body
}

// handleNonoChipMixture CMD 9004：芯片合成；空 ACK。
func (s *Server) handleNonoChipMixture(c *Client, uid uint32, body []byte) {
	if s.cfg.Store == nil {
		s.send(c, 9004, uid, 0, nil)
		return
	}
	words := make([]uint32, 0, len(body)/4)
	for i := 0; i+4 <= len(body); i += 4 {
		words = append(words, binary.BigEndian.Uint32(body[i:i+4]))
	}
	if len(words) == 0 {
		s.pushAlert(c, uid, "芯片合成参数错误")
		s.send(c, 9004, uid, 0, nil)
		return
	}
	resultID := uint32(0)
	mats := words
	if isNonoChipItemID(words[0]) {
		resultID = words[0]
		mats = words[1:]
	} else if isNonoChipItemID(words[len(words)-1]) {
		resultID = words[len(words)-1]
		mats = words[:len(words)-1]
	}
	if !isNonoChipItemID(resultID) {
		s.pushAlert(c, uid, "未识别到目标芯片")
		s.send(c, 9004, uid, 0, nil)
		return
	}
	need := map[uint32]int{}
	for _, id := range mats {
		if id == 0 {
			continue
		}
		need[id]++
	}
	if len(need) == 0 {
		s.pushAlert(c, uid, "请先放入合成材料")
		s.send(c, 9004, uid, 0, nil)
		return
	}
	for itemID, cnt := range need {
		have, _ := s.cfg.Store.GetItemCount(int64(uid), int(itemID))
		if have < cnt {
			s.pushAlert(c, uid, "合成材料不足")
			s.send(c, 9004, uid, 0, nil)
			return
		}
	}
	for itemID, cnt := range need {
		if err := s.cfg.Store.ConsumeItem(int64(uid), int(itemID), cnt); err != nil {
			s.pushAlert(c, uid, "合成材料不足")
			s.send(c, 9004, uid, 0, nil)
			return
		}
	}
	_ = s.cfg.Store.AddItem(int64(uid), int(resultID), 1)
	s.pushAlert(c, uid, "芯片合成成功")
	s.send(c, 9004, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d result=%d", cmdname.Format(9004), uid, resultID)
}

// handleNonoExpadm CMD 9008：经验分配芯片管理；空 ACK。
func (s *Server) handleNonoExpadm(c *Client, uid uint32) {
	s.send(c, 9008, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(9008), uid)
}

// handleNonoImplementTool CMD 9010：安装/使用芯片。
// 应答：uid+itemId+power*1000+ai(u16)+mate*1000+iq；变色芯片另推 9012。
func (s *Server) handleNonoImplementTool(c *Client, uid uint32, body []byte) {
	itemID := uint32(0)
	if len(body) >= 4 {
		itemID = binary.BigEndian.Uint32(body[0:4])
	}
	n := store.DefaultNono(int64(uid))
	if s.cfg.Store != nil {
		if got, _ := s.cfg.Store.GetOrInitNono(int64(uid)); got != nil {
			n = got
		}
		if n.HasNono == 0 {
			n.HasNono = 1
			n.Flag = 1
		}
		_ = s.cfg.Store.ConsumeItem(int64(uid), int(itemID), 1)
	}
	n.FuncBits = normalizeNonoFuncBits(n.FuncBits)
	if itemID >= 700001 && itemID <= 700060 {
		setNonoFuncBit(n.FuncBits, itemID)
	}
	if itemID == 700061 {
		setNonoFuncBit(n.FuncBits, itemID)
	}
	s.applyNonoItemFx(n, itemID)
	changedColor := false
	if s.cfg.Catalog != nil {
		if fx, ok := s.cfg.Catalog.NonoItemOf(int(itemID)); ok && fx.HasColor {
			n.Color = fx.Color
			changedColor = true
		}
	}
	if s.cfg.Store != nil {
		_ = s.cfg.Store.UpsertNono(n)
	}
	if changedColor {
		colorBody := make([]byte, 8)
		binary.BigEndian.PutUint32(colorBody[0:4], uid)
		binary.BigEndian.PutUint32(colorBody[4:8], uint32(n.Color))
		s.send(c, 9012, uid, 0, colorBody)
		s.broadcastToMap(c, 9012, colorBody)
	}
	out := s.buildNonoToolBody(uid, itemID, n)
	s.send(c, 9010, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d item=%d color=%d", cmdname.Format(9010), uid, itemID, n.Color)
}

// handleNonoChangeColor CMD 9012：请求 color；应答 uid+color，并广播同图。
func (s *Server) handleNonoChangeColor(c *Client, uid uint32, body []byte) {
	color := uint32(0)
	if len(body) >= 4 {
		color = binary.BigEndian.Uint32(body[0:4])
	}
	if s.cfg.Store != nil {
		n, _ := s.cfg.Store.GetOrInitNono(int64(uid))
		if n.HasNono == 0 {
			n.HasNono = 1
			n.Flag = 1
		}
		n.Color = int(color)
		_ = s.cfg.Store.UpsertNono(n)
	}
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], uid)
	binary.BigEndian.PutUint32(out[4:8], color)
	s.send(c, 9012, uid, 0, out)
	s.broadcastToMap(c, 9012, out)
	log.Printf("[CMD] OK     %s UID=%d color=%d", cmdname.Format(9012), uid, color)
}

// handleNonoPlay CMD 9013：玩耍/玩具；应答与 9010 同布局（首字段为 uid）。
func (s *Server) handleNonoPlay(c *Client, uid uint32, body []byte) {
	itemID := uint32(0)
	if len(body) >= 4 {
		itemID = binary.BigEndian.Uint32(body[0:4])
	}
	n := store.DefaultNono(int64(uid))
	if s.cfg.Store != nil {
		if got, _ := s.cfg.Store.GetOrInitNono(int64(uid)); got != nil {
			n = got
		}
		if n.HasNono == 0 {
			n.HasNono = 1
			n.Flag = 1
		}
		_ = s.cfg.Store.ConsumeItem(int64(uid), int(itemID), 1)
	}
	s.applyNonoItemFx(n, itemID)
	if itemID == 0 {
		n.Mate = clampNonoStat(n.Mate + 5)
	}
	if s.cfg.Store != nil {
		_ = s.cfg.Store.UpsertNono(n)
	}
	out := s.buildNonoToolBody(uid, itemID, n)
	s.send(c, 9013, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d item=%d mate=%d", cmdname.Format(9013), uid, itemID, n.Mate)
}

// handleNonoGetChip CMD 9023：NPC/发明室领取芯片。
// 应答：0*3 + count + [itemId,count]...
func (s *Server) handleNonoGetChip(c *Client, uid uint32, body []byte) {
	chipItemID := uint32(700005)
	if len(body) >= 20 {
		page := binary.BigEndian.Uint32(body[12:16])
		slot := binary.BigEndian.Uint32(body[16:20])
		gi := (page-1)*3 + slot
		if gi < uint32(len(superNonoChipList)) {
			chipItemID = superNonoChipList[gi]
		} else if slot >= 1 && slot <= 60 {
			chipItemID = 700000 + slot
		}
	} else if len(body) >= 4 {
		chipType := binary.BigEndian.Uint32(body[0:4])
		// 任务 96 肖恩送礼：发包 13，文案为跟随模式芯片 → 700005
		if chipType == 13 {
			chipItemID = 700005
		} else if chipType >= 1 && chipType <= 60 {
			chipItemID = 700000 + chipType
		}
	}
	if s.cfg.Store != nil {
		_ = s.cfg.Store.AddItem(int64(uid), int(chipItemID), 1)
	}
	out := make([]byte, 24)
	binary.BigEndian.PutUint32(out[12:16], 1)
	binary.BigEndian.PutUint32(out[16:20], chipItemID)
	binary.BigEndian.PutUint32(out[20:24], 1)
	s.send(c, 9023, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d chip=%d", cmdname.Format(9023), uid, chipItemID)
}

// handleNonoCure CMD 9007：同 2306（Nono 快捷「精灵治疗」）；超能期内免费。
func (s *Server) handleNonoCure(c *Client, uid uint32) {
	cost := petCureAllCost
	if s.hasActiveSuperNono(int64(uid)) {
		cost = 0
	}
	n, spent := s.cureAllBagPets(int64(uid), cost)
	s.send(c, 9007, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d bag=%d spent=%v free=%v", cmdname.Format(9007), uid, n, spent, cost == 0)
}

// handleNonoCharge CMD 9016：请求 onOff；应答 userID+charging。
func (s *Server) handleNonoCharge(c *Client, uid uint32, body []byte) {
	onOff := uint32(0)
	if len(body) >= 4 {
		onOff = binary.BigEndian.Uint32(body[0:4])
	}
	if s.cfg.Store != nil {
		n, _ := s.cfg.Store.GetOrInitNono(int64(uid))
		if onOff != 0 {
			n.ChargeTime = int(time.Now().Unix())
			if n.Power < 100 {
				n.Power += 10
				if n.Power > 100 {
					n.Power = 100
				}
			}
		} else {
			n.ChargeTime = 0
		}
		_ = s.cfg.Store.UpsertNono(n)
	}
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], uid)
	binary.BigEndian.PutUint32(out[4:8], onOff)
	s.send(c, 9016, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d on=%d", cmdname.Format(9016), uid, onOff)
}

// handleNonoFollowOrHoom CMD 9019：action=1 跟随(36B) / 0 回家(12B)；再推 9003。
func (s *Server) handleNonoFollowOrHoom(c *Client, uid uint32, body []byte) {
	action := uint32(0)
	if len(body) >= 4 {
		action = binary.BigEndian.Uint32(body[0:4])
	}
	n := store.DefaultNono(int64(uid))
	if s.cfg.Store != nil {
		if got, _ := s.cfg.Store.GetOrInitNono(int64(uid)); got != nil {
			n = got
		}
	}
	if n.HasNono == 0 {
		n.HasNono = 1
		n.Flag = 1
	}
	s.syncSuperNonoStage(n)
	follow := action == 1
	n.SetFollowing(follow)
	if s.cfg.Store != nil {
		_ = s.cfg.Store.UpsertNono(n)
	}

	var out []byte
	if follow {
		out = make([]byte, 36)
		binary.BigEndian.PutUint32(out[0:4], uid)
		binary.BigEndian.PutUint32(out[4:8], uint32(n.SuperStage)) // 客户端作 superStage
		binary.BigEndian.PutUint32(out[8:12], 1)
		putFixedNick(out, 12, n.Nick)
		binary.BigEndian.PutUint32(out[28:32], uint32(n.Color))
		binary.BigEndian.PutUint32(out[32:36], uint32(n.Power*1000))
	} else {
		out = make([]byte, 12)
		binary.BigEndian.PutUint32(out[0:4], uid)
		binary.BigEndian.PutUint32(out[4:8], uint32(n.SuperStage))
		binary.BigEndian.PutUint32(out[8:12], 0)
	}
	s.send(c, 9019, uid, 0, out)
	s.broadcastToMap(c, 9019, out)
	s.pushNonoInfo(c, uid)
	log.Printf("[CMD] OK     %s UID=%d follow=%v stage=%d", cmdname.Format(9019), uid, follow, n.SuperStage)
}

// handleNonoOpenSuper CMD 9020：扣 10 金豆升一级超能；空 ACK + 9003/8006/1106。
func (s *Server) handleNonoOpenSuper(c *Client, uid uint32) {
	ok, n := s.tryActivateSuperNono(c, uid, true)
	s.send(c, 9020, uid, 0, nil)
	if ok {
		s.pushAfterSuperOpen(c, uid, n)
	}
	log.Printf("[CMD] OK     %s UID=%d ok=%v", cmdname.Format(9020), uid, ok)
}

// handleOpenSuperNono CMD 80001：KTool 开通；应答 success(0=ok)。
func (s *Server) handleOpenSuperNono(c *Client, uid uint32) {
	ok, n := s.tryActivateSuperNono(c, uid, true)
	out := make([]byte, 4)
	if !ok {
		binary.BigEndian.PutUint32(out, 1)
	}
	s.send(c, 80001, uid, 0, out)
	if ok {
		s.pushAfterSuperOpen(c, uid, n)
	}
	log.Printf("[CMD] OK     %s UID=%d ok=%v", cmdname.Format(80001), uid, ok)
}

func (s *Server) tryActivateSuperNono(c *Client, uid uint32, chargeGold bool) (bool, *store.Nono) {
	if s.cfg.Store == nil {
		s.pushAlert(c, uid, "存档未就绪")
		return false, nil
	}
	n, err := s.cfg.Store.GetOrInitNono(int64(uid))
	if err != nil {
		s.pushAlert(c, uid, "读取 NoNo 失败")
		return false, nil
	}
	s.applySuperNonoExpiry(n)
	if n.HasNono == 0 {
		n.HasNono = 1
		n.Flag = 1
		if n.Birth == 0 {
			n.Birth = time.Now().Unix()
		}
	}
	active := n.VipEndTime > time.Now().Unix()
	// 已激活且等级已满：不可再升
	if active && n.SuperLevel > 0 && n.SuperMonths >= superNonoMaxLevel && n.SuperLevel >= n.SuperMonths {
		s.pushAlert(c, uid, "超能NONO已达最高等级")
		return false, n
	}
	if chargeGold {
		u, err := s.cfg.Store.FindByUserID(int64(uid))
		if err != nil || u == nil || u.Gold < superNonoActivateGoldCost {
			s.pushAlert(c, uid, "你的金豆不够")
			return false, n
		}
		if err := s.cfg.Store.AddGold(int64(uid), -superNonoActivateGoldCost); err != nil {
			s.pushAlert(c, uid, "你的金豆不够")
			return false, n
		}
		n.OpenGoldCharged = true
	}
	now := time.Now().Unix()
	granted3Month := false
	switch {
	case !active:
		// 续费/首次开通：延长时长；已有月数则等级仍为 0，需再花金豆恢复
		n.VipEndTime = now + int64(superNonoDurationDays*24*60*60)
		if n.SuperMonths < 1 {
			n.SuperMonths = 1
			n.SuperLevel = 1
		}
		// SuperMonths>0 且 SuperLevel==0：仅续期，等下次开通恢复等级
	case n.SuperLevel == 0 && n.SuperMonths > 0:
		// 续费后再花金豆：恢复过期前等级
		n.SuperLevel = n.SuperMonths
		if n.SuperLevel > superNonoMaxLevel {
			n.SuperLevel = superNonoMaxLevel
		}
	default:
		// 有效期内升级：月数+1，等级跟上
		n.SuperMonths++
		if n.SuperMonths > superNonoMaxLevel {
			n.SuperMonths = superNonoMaxLevel
		}
		n.SuperLevel = n.SuperMonths
		granted3Month = n.SuperMonths == 3
	}
	s.syncSuperNonoStage(n)
	if err := s.cfg.Store.UpsertNono(n); err != nil {
		log.Printf("[nono] UpsertNono uid=%d: %v", uid, err)
	}
	// 开通满 3 个月赠阿尔达拉精元（攻略；仅首次升到 3 月时发）
	if granted3Month {
		if err := s.cfg.Store.AddItem(int64(uid), 400147, 1); err != nil {
			log.Printf("[nono] grant 400147 uid=%d: %v", uid, err)
		} else {
			s.pushAlert(c, uid, "开通满3个月，获得阿尔达拉的精元×1")
		}
	}
	return true, n
}

// applySuperNonoExpiry 超No到期：等级归零（保留 SuperMonths 供续费后恢复）。
func (s *Server) applySuperNonoExpiry(n *store.Nono) {
	if n == nil || n.VipEndTime <= 0 || n.VipEndTime >= time.Now().Unix() {
		return
	}
	if n.SuperLevel == 0 && (n.SuperNono == 0 || n.SuperNono == 6) {
		return
	}
	changed := n.SuperLevel > 0
	n.SuperLevel = 0
	if n.SuperNono != 6 { // 至尊外观保留码，过期仍清阶段
		n.SuperStage = 0
		n.SuperNono = 0
	} else {
		n.SuperStage = 0
	}
	if changed && s.cfg.Store != nil {
		if err := s.cfg.Store.UpsertNono(n); err != nil {
			log.Printf("[nono] expiry UpsertNono uid=%d: %v", n.UserID, err)
		}
	}
}

func (s *Server) hasActiveSuperNono(uid int64) bool {
	if s.cfg.Store == nil {
		return false
	}
	n, err := s.cfg.Store.GetOrInitNono(uid)
	if err != nil || n == nil {
		return false
	}
	s.applySuperNonoExpiry(n)
	if n.VipEndTime <= 0 || n.VipEndTime < time.Now().Unix() {
		return false
	}
	return n.SuperLevel > 0
}

func (s *Server) syncSuperNonoStage(n *store.Nono) {
	if n == nil {
		return
	}
	if n.SuperLevel <= 0 {
		if n.SuperNono != 6 {
			n.SuperStage = 0
			n.SuperNono = 0
		}
		return
	}
	stage := superStageByLevel(n.SuperLevel)
	n.SuperStage = stage
	if n.SuperNono != 6 { // 至尊保留 6
		n.SuperNono = stage
		if n.SuperNono < 1 {
			n.SuperNono = 1
		}
	}
}

func superStageByLevel(level int) int {
	if level < 1 {
		return 0
	}
	switch {
	case level >= 12:
		return 5
	case level >= 9:
		return 4
	case level >= 7:
		return 3
	case level >= 4:
		return 2
	default:
		return 1
	}
}

func (s *Server) pushAfterSuperOpen(c *Client, uid uint32, n *store.Nono) {
	s.pushNonoInfo(c, uid)
	s.pushVipCo(c, uid, n)
	s.pushGoldBalance1106(c, uid)
}

func (s *Server) pushVipCo(c *Client, uid uint32, n *store.Nono) {
	auto, end := uint32(0), uint32(0)
	if n != nil {
		auto = uint32(n.AutoCharge)
		if n.VipEndTime > 0 && n.VipEndTime <= 0x7fffffff {
			end = uint32(n.VipEndTime)
		}
	}
	body := make([]byte, 16)
	binary.BigEndian.PutUint32(body[0:4], uid)
	binary.BigEndian.PutUint32(body[4:8], 2) // vipFlag=2 → 客户端 superNono=true
	binary.BigEndian.PutUint32(body[8:12], auto)
	binary.BigEndian.PutUint32(body[12:16], end)
	s.send(c, 8006, uid, 0, body)
}

func (s *Server) pushGoldBalance1106(c *Client, uid uint32) {
	gold, coins := 0, 0
	if s.cfg.Store != nil {
		if u, err := s.cfg.Store.FindByUserID(int64(uid)); err == nil && u != nil {
			gold, coins = u.Gold, u.Coins
		}
	}
	body := make([]byte, 8)
	binary.BigEndian.PutUint32(body[0:4], uint32(gold*100))
	binary.BigEndian.PutUint32(body[4:8], uint32(coins))
	s.send(c, 1106, uid, 0, body)
}

func (s *Server) pushAlert(c *Client, uid uint32, msg string) {
	b := []byte(msg)
	body := make([]byte, 4+len(b))
	binary.BigEndian.PutUint32(body[0:4], uint32(len(b)))
	copy(body[4:], b)
	s.send(c, 80002, uid, 0, body)
}

// loadNonoForLogin 登录/同图展示用。
func (s *Server) loadNonoForLogin(uid int64) *store.Nono {
	if s.cfg.Store == nil {
		return nil
	}
	n, _ := s.cfg.Store.GetNono(uid)
	if n != nil {
		s.applySuperNonoExpiry(n)
		s.syncSuperNonoStage(n)
	}
	return n
}
