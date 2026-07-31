package gameserver

import (
	"encoding/binary"
	"log"
	"sort"

	"niaohao/server/internal/cmdname"
)

const (
	gaiyaPetID           = 261
	skillIDShiPoTianJing = 10715
	gaiyaTrainMapID      = 61
	gaiyaTrainBossRegion = 7 // 地图61 第二特训；出战宠为厄尔塞拉 421（无 5019 对战资源）
)

// isGaiyaTrainElsera 盖亚第二关特训（地图61 region7），不依赖特设 petID。
func isGaiyaTrainElsera(st *BattleState) bool {
	return st != nil && st.MapID == gaiyaTrainMapID && st.BossRegion == gaiyaTrainBossRegion
}

// handleGaiyaEffectSet CMD 2148：设置默认魂印；请求 effectID(4)；应答 old(4)+new(4)。
func (s *Server) handleGaiyaEffectSet(c *Client, uid uint32, body []byte) {
	if len(body) < 4 || s.cfg.Store == nil {
		s.send(c, 2148, uid, 0, nil)
		return
	}
	want := int(binary.BigEndian.Uint32(body[0:4]))
	defID, mask, _ := s.cfg.Store.GetGaiyaEffect(int64(uid))
	old := defID
	if old == 0 {
		for i := 0; i < 3; i++ {
			if mask&(1<<i) != 0 {
				old = i + 1
				break
			}
		}
	}
	newDef := old
	if want >= 1 && want <= 3 && mask&(1<<(want-1)) != 0 {
		newDef = want
		_ = s.cfg.Store.SetGaiyaEffect(int64(uid), newDef, mask)
	}
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], uint32(old))
	binary.BigEndian.PutUint32(out[4:8], uint32(newDef))
	s.send(c, 2148, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d old=%d new=%d", cmdname.Format(2148), uid, old, newDef)
}

// handleGaiyaEffect CMD 2149：查询；带 body 时仅切换已解锁默认魂印（不因设置而解锁）。
func (s *Server) handleGaiyaEffect(c *Client, uid uint32, body []byte) {
	defID, mask := 0, 0
	if s.cfg.Store != nil {
		defID, mask, _ = s.cfg.Store.GetGaiyaEffect(int64(uid))
		if len(body) >= 4 {
			want := int(binary.BigEndian.Uint32(body[0:4]))
			if want >= 1 && want <= 3 && mask&(1<<(want-1)) != 0 {
				defID = want
				_ = s.cfg.Store.SetGaiyaEffect(int64(uid), defID, mask)
				if want == 1 {
					s.syncGaiyaTrainTask(int64(uid), c, uid, 1)
				}
			}
		}
	}
	s.send(c, 2149, uid, 0, buildGaiyaEffectInfoBody(defID, mask))
	log.Printf("[CMD] OK     %s UID=%d def=%d mask=%d", cmdname.Format(2149), uid, defID, mask)
}

func buildGaiyaEffectInfoBody(defID, mask int) []byte {
	effects := make([]int, 0, 3)
	for i := 0; i < 3; i++ {
		if mask&(1<<i) != 0 {
			effects = append(effects, i+1)
		}
	}
	sort.Ints(effects)
	if defID == 0 && len(effects) > 0 {
		defID = effects[0]
	}
	if len(effects) == 0 {
		out := make([]byte, 8)
		binary.BigEndian.PutUint32(out[0:4], uint32(defID))
		return out
	}
	// 兼容旧解析：def + (count+1) + count + ids
	compat := len(effects) + 1
	out := make([]byte, 8+4*compat)
	binary.BigEndian.PutUint32(out[0:4], uint32(defID))
	binary.BigEndian.PutUint32(out[4:8], uint32(compat))
	binary.BigEndian.PutUint32(out[8:12], uint32(len(effects)))
	for i, id := range effects {
		binary.BigEndian.PutUint32(out[12+4*i:16+4*i], uint32(id))
	}
	return out
}

// unlockGaiyaEffectOnWin 盖亚魂印三阶解锁。
// 1 嗜血：423+雷伊(70) 满血结束；2 邪气：61 region7 厄尔塞拉特训 回合≥10；3 石破：423+哈莫(216) 末击 10715 且暴击。
func (s *Server) unlockGaiyaEffectOnWin(c *Client, uid uint32, st *BattleState) {
	if s.cfg.Store == nil || st == nil {
		return
	}
	effectID := 0
	if isGaiyaTrainElsera(st) && st.Round >= 10 {
		effectID = 2
	} else {
		switch st.EnemyID {
		case 70:
			if st.MapID == 423 && st.PlayerHP == st.PlayerMaxHP && st.PlayerHP > 0 {
				effectID = 1
			}
		case 216:
			if st.MapID == 423 && st.LastPlayerSkillID == skillIDShiPoTianJing && st.LastHitWasCrit {
				effectID = 3
			}
		}
	}
	if effectID == 0 {
		return
	}
	defID, mask, _ := s.cfg.Store.GetGaiyaEffect(int64(uid))
	bit := 1 << (effectID - 1)
	already := mask&bit != 0
	if !already {
		mask |= bit
		// 本客户端 GaiyaEffectInfo 反编译未填 effects[]，战后 hasGaiyaEffect 只认 defEffectID；
		// 故新解锁时必须把默认设为该魂印，否则 MapProcess_61/423 会判失败。
		defID = effectID
		_ = s.cfg.Store.SetGaiyaEffect(int64(uid), defID, mask)
		log.Printf("[盖亚特训] 解锁 effect=%d UID=%d map=%d enemy=%d round=%d crit=%v skill=%d",
			effectID, uid, st.MapID, st.EnemyID, st.Round, st.LastHitWasCrit, st.LastPlayerSkillID)
	}
	s.syncGaiyaTrainTask(int64(uid), c, uid, effectID)
}

func (s *Server) syncGaiyaTrainTask(storeUID int64, c *Client, uid uint32, effectID int) {
	taskID, step := 0, 0
	switch effectID {
	case 1:
		taskID, step = 622, 1
	case 2:
		taskID, step = 627, 2
	case 3:
		taskID, step = 634, 2
	}
	if taskID == 0 || s.cfg.Store == nil {
		return
	}
	t, _ := s.cfg.Store.GetTask(storeUID, taskID)
	already := t != nil && t.Status >= taskStatusComplete
	buf := make([]byte, 20)
	if t != nil && len(t.Buf) > 0 {
		copy(buf, t.Buf)
	}
	if step < 20 {
		buf[step] = 1
	}
	if taskID == 622 {
		buf[0] = 1
	}
	_ = s.cfg.Store.UpsertTaskBuf(storeUID, taskID, buf)
	s.setTaskStatus(storeUID, taskID, taskStatusComplete)
	if !already && c != nil {
		out := make([]byte, 16)
		binary.BigEndian.PutUint32(out[0:4], uint32(taskID))
		s.send(c, 2202, uid, 0, out)
	}
}
