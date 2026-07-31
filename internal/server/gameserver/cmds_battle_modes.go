package gameserver

import (
	"encoding/binary"
	"fmt"
	"log"
	"strconv"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/store"
)

// handleChoiceFightLevel CMD 2414：勇者之塔选层。
// 请求 level(4)；应答 level+count+bossIDs。
func (s *Server) handleChoiceFightLevel(c *Client, uid uint32, body []byte) {
	level := uint32(1)
	if len(body) >= 4 {
		level = binary.BigEndian.Uint32(body[0:4])
	}
	// 分页入口 1..8 → 真实层 1/11/21...
	if level >= 1 && level <= 8 {
		real := 1 + (level-1)*10
		if real <= braveTowerMaxLevel {
			level = real
		}
	}
	if level == 0 {
		level = 1
	}
	if level > braveTowerMaxLevel {
		level = braveTowerMaxLevel
	}
	bosses := braveTowerBosses(int(level))
	s.modes.setBrave(int64(uid), int(level), bosses)
	if s.cfg.Store != nil {
		_ = s.cfg.Store.SetBraveProgress(int64(uid), int(level))
	}
	out := buildLevelBossBody(level, bosses)
	s.send(c, 2414, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d level=%d bosses=%v", cmdname.Format(2414), uid, level, bosses)
}

// handleStartFightLevel CMD 2415：勇者之塔开战。
// 应答下一层 Boss 列表，再推 2503。
func (s *Server) handleStartFightLevel(c *Client, uid uint32) {
	cur, bosses := s.modes.getBrave(int64(uid))
	if cur < 1 {
		cur = 1
		bosses = braveTowerBosses(cur)
		s.modes.setBrave(int64(uid), cur, bosses)
	}
	next := cur + 1
	if next > braveTowerMaxLevel {
		next = braveTowerMaxLevel
	}
	nextBosses := braveTowerBosses(next)
	s.send(c, 2415, uid, 0, buildLevelBossBody(uint32(next), nextBosses))

	enemyID := uint32(15)
	if len(bosses) > 0 {
		enemyID = bosses[0]
	}
	enemyLv := 10 + cur*2
	if enemyLv > 100 {
		enemyLv = 100
	}
	s.beginFightVsEnemy(c, uid, int(enemyID), enemyLv, false, fightKindBrave)
	log.Printf("[CMD] OK     %s UID=%d cur=%d enemy=%d -> 2503", cmdname.Format(2415), uid, cur, enemyID)
}

// handleLeaveFightLevel CMD 2416：离开勇者之塔；客户端不解析包体。
// 本前端 MapProcess_500.onLeaveFightHandler 只恢复工具栏、不 changeMap；
// 但 MapController.onEnterMap 在 _newMapID==TOWER_MAP(500) 时收到任意 2001 会 changeMap(108)。
// 故 ACK 后推一条自角色 2001，触发客户端自行出塔（随后会发 LEAVE_MAP + ENTER_MAP 108）。
func (s *Server) handleLeaveFightLevel(c *Client, uid uint32) {
	s.modes.clearBrave(int64(uid))
	s.send(c, 2416, uid, 0, nil)
	s.pushTowerLeaveEnterMap(c)
	log.Printf("[CMD] OK     %s UID=%d -> push 2001 (kick tower→108)", cmdname.Format(2416), uid)
}

// handleFreshChoiceFightLevel CMD 2428：试炼之塔选层。
func (s *Server) handleFreshChoiceFightLevel(c *Client, uid uint32, body []byte) {
	level := uint32(1)
	if len(body) >= 4 {
		level = binary.BigEndian.Uint32(body[0:4])
	}
	if level == 0 {
		level = 1
	}
	if level > freshTowerMaxLevel {
		level = freshTowerMaxLevel
	}
	s.modes.setFresh(int64(uid), int(level))
	if s.cfg.Store != nil {
		_ = s.cfg.Store.SetFreshProgress(int64(uid), int(level))
	}
	boss := freshTowerBoss(int(level))
	out := buildLevelBossBody(level, []uint32{boss})
	s.send(c, 2428, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d level=%d boss=%d", cmdname.Format(2428), uid, level, boss)
}

// handleFreshStartFightLevel CMD 2429：试炼之塔开战。
func (s *Server) handleFreshStartFightLevel(c *Client, uid uint32) {
	cur := s.modes.getFresh(int64(uid))
	if cur < 1 {
		cur = 1
		s.modes.setFresh(int64(uid), cur)
	}
	next := cur + 1
	if next > freshTowerMaxLevel {
		next = freshTowerMaxLevel
	}
	s.send(c, 2429, uid, 0, buildLevelBossBody(uint32(next), []uint32{freshTowerBoss(next)}))
	enemyID := int(freshTowerBoss(cur))
	enemyLv := 5 + cur
	if enemyLv > 80 {
		enemyLv = 80
	}
	s.beginFightVsEnemy(c, uid, enemyID, enemyLv, false, fightKindFresh)
	log.Printf("[CMD] OK     %s UID=%d cur=%d enemy=%d -> 2503", cmdname.Format(2429), uid, cur, enemyID)
}

// handleFreshLeaveFightLevel CMD 2430：离开试炼之塔。
// 同 2416：本前端 MapProcess_600 离开后不 changeMap；onEnterMap 在 _newMapID==600 时会 changeMap(101)。
func (s *Server) handleFreshLeaveFightLevel(c *Client, uid uint32) {
	s.modes.clearFresh(int64(uid))
	s.send(c, 2430, uid, 0, nil)
	s.pushTowerLeaveEnterMap(c)
	log.Printf("[CMD] OK     %s UID=%d -> push 2001 (kick fresh→101)", cmdname.Format(2430), uid)
}

// pushTowerLeaveEnterMap 推送自角色 2001，借 MapController.onEnterMap 塔图分支触发 changeMap。
// 须带完整 PeopleInfo：若 _newMapID 不是 500/600，会走正常解析，空包体会 #1009。
func (s *Server) pushTowerLeaveEnterMap(c *Client) {
	if c == nil || !c.LoggedIn || c.UserID == 0 {
		return
	}
	user, _ := s.cfg.Store.FindByUserID(c.UserID)
	if user == nil {
		user = &store.User{UserID: c.UserID, Nickname: fmt.Sprintf("%d", c.UserID)}
	}
	x, y := c.PosX, c.PosY
	if x == 0 && y == 0 {
		x, y = defaultPosX, defaultPosY
	}
	people := s.buildPeopleInfo(user, x, y, c.ClothIDs, c.actionTypeLocked())
	s.send(c, 2001, uint32(c.UserID), 0, people)
	log.Printf("[game-2001] uid=%d body=%d (tower-leave kick)", c.UserID, len(people))
}

// handleOpenDarkPortal CMD 2424：开门；应答 bossPetID(4)。
// 请求 curDoor 为槽位 ID（门序号=curDoor/6，子门=curDoor%6）；
// 第2门起需超能等级 doorIndex+1；第1门可用超能或暗黑之钥。
func (s *Server) handleOpenDarkPortal(c *Client, uid uint32, body []byte) {
	curDoor := uint32(0)
	if len(body) >= 4 {
		curDoor = binary.BigEndian.Uint32(body[0:4])
	}
	doorIndex, subIndex := parseDarkPortalCurDoor(curDoor)
	ok, need, lv := s.darkPortalAccessOK(int64(uid), doorIndex)
	if !ok {
		msg := "超能NoNo等级不足（需" + strconv.Itoa(need) + "级，当前" + strconv.Itoa(lv) + "级）才能进入该暗黑之门"
		if doorIndex == 0 {
			msg = "需要超能NoNo或暗黑之钥才能进入暗黑第一门"
		}
		s.sendAlert(int64(uid), msg)
		out := make([]byte, 4)
		s.send(c, 2424, uid, 0, out)
		log.Printf("[CMD] OK     %s UID=%d curDoor=%d door=%d sub=%d deny need=%d have=%d",
			cmdname.Format(2424), uid, curDoor, doorIndex, subIndex, need, lv)
		return
	}
	s.modes.setDarkDoor(int64(uid), doorIndex, subIndex)
	boss, _ := darkPortalBossEntry(doorIndex, subIndex)
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, uint32(boss))
	s.send(c, 2424, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d curDoor=%d door=%d sub=%d boss=%d map=%d",
		cmdname.Format(2424), uid, curDoor, doorIndex, subIndex, boss, darkPortalMapID(doorIndex))
}

// handleFightDarkPortal CMD 2425：暗黑道场开战；应答可空，再推 2503。
// 首胜精元走 grantSPTFirstKillReward（按 EnemyID）；固定血量 + 天生能力走 applyBossOpenBattleRules。
func (s *Server) handleFightDarkPortal(c *Client, uid uint32) {
	info := s.modes.getDarkDoor(int64(uid))
	ok, _, _ := s.darkPortalAccessOK(int64(uid), info.DoorIndex)
	if !ok {
		s.send(c, 2425, uid, 0, nil)
		s.sendAlert(int64(uid), "超能NoNo等级不足，无法挑战暗黑武斗场")
		log.Printf("[CMD] OK     %s UID=%d door=%d sub=%d deny", cmdname.Format(2425), uid, info.DoorIndex, info.SubIndex)
		return
	}
	boss, lv := darkPortalBossEntry(info.DoorIndex, info.SubIndex)
	mapID := darkPortalMapID(info.DoorIndex)
	s.send(c, 2425, uid, 0, nil)
	s.beginFightVsEnemy(c, uid, boss, lv, false, fightKindNormal)
	if st := s.battles.get(int64(uid)); st != nil {
		st.MapID = mapID
		st.BossRegion = info.SubIndex
		applyBossOpenBattleRules(st)
		s.battles.set(int64(uid), st)
	}
	log.Printf("[CMD] OK     %s UID=%d door=%d sub=%d boss=%d lv=%d map=%d -> 2503",
		cmdname.Format(2425), uid, info.DoorIndex, info.SubIndex, boss, lv, mapID)
}

// handleLeaveDarkPortal CMD 2426。
func (s *Server) handleLeaveDarkPortal(c *Client, uid uint32) {
	s.modes.clearDarkDoor(int64(uid))
	s.send(c, 2426, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(2426), uid)
}

// handleMlFigBoss CMD 2442：师徒试炼开战信号；客户端不读包体，服端推 2503。
func (s *Server) handleMlFigBoss(c *Client, uid uint32, body []byte) {
	s.send(c, 2442, uid, 0, nil)
	s.beginFightVsEnemy(c, uid, 15, 20, false, fightKindNormal)
	log.Printf("[CMD] OK     %s UID=%d -> 2503", cmdname.Format(2442), uid)
}

// handleMlStateBoss CMD 2444：空 ACK（本客户端无独立解析类）。
func (s *Server) handleMlStateBoss(c *Client, uid uint32) {
	s.send(c, 2444, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(2444), uid)
}

// handleMlStepPos CMD 2445：站位同步。应答 2×(uid+posIndex+x+y)=32B。
func (s *Server) handleMlStepPos(c *Client, uid uint32, body []byte) {
	flag, posIdx := uint32(0), uint32(0)
	if len(body) >= 4 {
		flag = binary.BigEndian.Uint32(body[0:4])
	}
	if len(body) >= 8 {
		posIdx = binary.BigEndian.Uint32(body[4:8])
	}
	x, y := uint32(c.PosX), uint32(c.PosY)
	if x == 0 && y == 0 {
		x, y = 480, 280
	}
	out := make([]byte, 32)
	if flag != 0 {
		binary.BigEndian.PutUint32(out[0:4], uid)
		binary.BigEndian.PutUint32(out[4:8], posIdx)
		binary.BigEndian.PutUint32(out[8:12], x)
		binary.BigEndian.PutUint32(out[12:16], y)
	}
	// 第二人槽留空（单人 stub）
	s.send(c, 2445, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d flag=%d pos=%d", cmdname.Format(2445), uid, flag, posIdx)
}

// handleMlGetPrize CMD 2446：领奖。应答 3×u32（占位+itemID+占位）。
func (s *Server) handleMlGetPrize(c *Client, uid uint32) {
	itemID := uint32(300011) // 常见药剂类，面板能认
	out := make([]byte, 12)
	binary.BigEndian.PutUint32(out[0:4], 1)
	binary.BigEndian.PutUint32(out[4:8], itemID)
	binary.BigEndian.PutUint32(out[8:12], 1)
	if s.cfg.Store != nil {
		_ = s.cfg.Store.AddItem(int64(uid), int(itemID), 1)
	}
	s.send(c, 2446, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d item=%d", cmdname.Format(2446), uid, itemID)
}

// handleGetSessionKey CMD 1006：空 ACK stub。
func (s *Server) handleGetSessionKey(c *Client, uid uint32) {
	s.send(c, 1006, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(1006), uid)
}
