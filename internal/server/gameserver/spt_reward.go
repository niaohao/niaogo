package gameserver

import (
	"log"

	"niaohao/server/internal/cmdname"
)

// sptFirstKillReward 经典 SPT 首杀（按 enemy PetID）。对照百科/参考表，仅本客户端 Boss 表内精灵。
// RewardPetID：直接发 1 级精灵；RewardItemID：精元等道具。二者可只填其一。
type sptFirstKillReward struct {
	RewardPetID  int
	RewardItemID int
}

// 谱尼(300)走裂片系统，不入本表；拂晓兔(4150)调试 Boss 无奖。
var sptFirstKillByPetID = map[int]sptFirstKillReward{
	47:   {RewardPetID: 46},             // 蘑菇怪 → 小蘑菇
	34:   {RewardItemID: 400051},        // 钢牙鲨 → 黑晶矿
	42:   {RewardItemID: 400107},        // 里奥斯精元
	50:   {RewardItemID: 400101},        // 阿克希亚精元
	69:   {RewardItemID: 400102},        // 提亚斯精元
	70:   {RewardItemID: 400103},        // 雷伊精元
	88:   {RewardItemID: 400104},        // 纳多雷精元
	113:  {RewardItemID: 400105},        // 雷纳多精元
	132:  {RewardItemID: 400108},        // 尤纳斯精元
	187:  {RewardItemID: 400114},        // 魔狮迪露精元
	216:  {RewardItemID: 400118},        // 哈莫雷特精元
	264:  {RewardItemID: 400125},        // 奈尼芬多精元
	421:  {RewardItemID: 400139},        // 厄尔塞拉精元
	261:  {RewardItemID: 400126},        // 盖亚精元（地图419 SPT；出现战走规则 2202/8010）
	274:  {RewardItemID: 400136},        // 塔克林精元
	391:  {RewardItemID: 400137},        // 塔西亚精元
	347:  {RewardItemID: 400127},        // 远古鱼龙精元（碧水精元）
	393:  {RewardItemID: 400133},        // 上古炎兽 → 烈火精元（孵化夏伏兽392）
	299:  {RewardItemID: 400119},        // 猛虎王 → 谜之精元（孵化迅牙虎298；官服虎碎片兑换同源）
	386:  {RewardPetID: 385},            // 基维奥拉：表内无专属精元，首杀发基摩（同蘑菇怪模式）
	462:  {RewardItemID: 400147},        // 阿尔达拉精元（与超No满3月同源；孵化阿尔克461）
	413:  {RewardItemID: 400138},        // 塞维尔精元
	1337: {RewardItemID: 400402},        // 机械塔克林精元
	169:  {RewardItemID: 400109},        // 卡特斯精元（试炼之门）
	490:  {RewardItemID: 400151},        // 劳克蒙德
	538:  {RewardItemID: 400153},        // 克拉尼特
	587:  {RewardItemID: 400156},        // 墨杜萨
	617:  {RewardItemID: 400161},        // 肯佩德
	715:  {RewardItemID: 400192},        // 德拉萨
	5012: {RewardItemID: 400187},        // 亚伦斯
	501:  {RewardItemID: 400140},        // 玄武巴斯特
	502:  {RewardItemID: 400145},        // 青龙朵拉格
	503:  {RewardItemID: 400190},        // 泰格尔
	527:  {RewardItemID: 400152},        // 赫尔托克 → 精元（孵化 526）

	// —— 暗黑武斗场主门 / 子门首胜精元 ——
	171:  {RewardItemID: 400110}, // 魔牙鲨
	174:  {RewardItemID: 400111}, // 贝鲁基德
	177:  {RewardItemID: 400112}, // 巴弗洛
	183:  {RewardItemID: 400113}, // 奇拉塔顿
	195:  {RewardItemID: 400115}, // 西萨拉斯
	192:  {RewardItemID: 400116}, // 克林卡修
	222:  {RewardItemID: 400120}, // 卡库
	224:  {RewardItemID: 400121}, // 赫德卡
	227:  {RewardItemID: 400122}, // 伊兰罗尼
	297:  {RewardItemID: 400128}, // 艾尔伊洛
	356:  {RewardItemID: 400129}, // 斯加尔卡
	359:  {RewardItemID: 400130}, // 布林克克
	438:  {RewardItemID: 400142}, // 魔花使者
	441:  {RewardItemID: 400143}, // 莫尔加斯
	435:  {RewardItemID: 400144}, // 萨诺拉斯
	656:  {RewardItemID: 400184}, // 帕多尼
	659:  {RewardItemID: 400185}, // 加洛德
	661:  {RewardItemID: 400186}, // 萨多拉尼
	779:  {RewardItemID: 400197}, // 迪普利德
	782:  {RewardItemID: 400198}, // 阿克诺亚
	784:  {RewardItemID: 400199}, // 拜洛亚斯
	1182: {RewardItemID: 400304}, // 鳞甲魔鱼
	1185: {RewardItemID: 400305}, // 艾斯德克
	1187: {RewardItemID: 400306}, // 狂狮迪卡
	1403: {RewardItemID: 400432}, // 萨洛奇斯
	1397: {RewardItemID: 400430}, // 查迪斯
	1400: {RewardItemID: 400431}, // 奈尼狄亚
}

// isFourGodNonTrueForm 四神非真身战：不发巴斯特/朵拉格/泰格尔精元。
func isFourGodNonTrueForm(mapID int, region uint32) bool {
	switch mapID {
	case 401, 403:
		return region != 1
	case 483:
		return region != 5
	default:
		return false
	}
}

// grantSPTFirstKillReward 首次击败经典 SPT / 四神兽真身等发奖；key=petID。
func (s *Server) grantSPTFirstKillReward(c *Client, uid uint32, st *BattleState) {
	if s.cfg.Store == nil || st == nil || c == nil || st.EnemyID <= 0 {
		return
	}
	// 特训幻影/副本 Boss / 盖亚出现战不发 SPT 首杀（出现战走规则精元）
	if isLeiyiEnergyTrain(st.BossRegion) || isGaiyaTrainElsera(st) || st.EnemyID == leiyiTrainPhantomPetID {
		return
	}
	if st.IsGaiyaAppear {
		return
	}
	if st.MapID == 423 && (st.BossRegion == 4 || st.BossRegion == 5) {
		return
	}
	// 四神：守护/战虎电虎阶段不发真身精元（同 petID 时按 region 区分）
	if isFourGodNonTrueForm(st.MapID, st.BossRegion) {
		return
	}
	rew, ok := sptFirstKillByPetID[st.EnemyID]
	if !ok || (rew.RewardPetID <= 0 && rew.RewardItemID <= 0) {
		return
	}
	already, err := s.cfg.Store.HasDefeatedSPT(int64(uid), st.EnemyID)
	if err != nil || already {
		return
	}
	if err := s.cfg.Store.MarkDefeatedSPT(int64(uid), st.EnemyID); err != nil {
		log.Printf("[spt] mark defeated UID=%d pet=%d err=%v", uid, st.EnemyID, err)
		return
	}
	if rew.RewardItemID > 0 {
		if err := s.cfg.Store.AddItem(int64(uid), rew.RewardItemID, 1); err != nil {
			log.Printf("[spt] AddItem UID=%d item=%d err=%v", uid, rew.RewardItemID, err)
			return
		}
		s.send(c, 8004, uid, 0, buildBossMonster8004Body(0, 0, 0, uint32(rew.RewardItemID), 1))
		log.Printf("[CMD] OK     %s UID=%d spt pet=%d item=%d", cmdname.Format(8004), uid, st.EnemyID, rew.RewardItemID)
	}
	if rew.RewardPetID > 0 {
		catch, err := s.grantNewPet(int64(uid), rew.RewardPetID, 1)
		if err != nil {
			log.Printf("[spt] grant pet UID=%d pet=%d err=%v", uid, rew.RewardPetID, err)
			return
		}
		s.pushBossMonster8004(c, uid, uint32(rew.RewardPetID), uint32(catch))
		log.Printf("[CMD] OK     %s UID=%d spt pet=%d rewardPet=%d catch=%d", cmdname.Format(8004), uid, st.EnemyID, rew.RewardPetID, catch)
	}
}
