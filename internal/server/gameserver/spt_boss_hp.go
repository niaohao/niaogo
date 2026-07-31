package gameserver

// SPT / 地图 Boss 固定血量（对照参考服 applyBossHPOverride + bossHPOverrides）。
// 优先级：地图/region 特例 → petID 表 → 种族公式原值。

var bossHPByPetID = map[int]int{
	47:   100,   // 蘑菇怪
	34:   200,   // 钢牙鲨
	42:   338,   // 里奥斯
	69:   500,   // 提亚斯
	50:   1000,  // 阿克希亚
	88:   1400,  // 纳多雷
	113:  1500,  // 雷纳多
	70:   2000,  // 雷伊
	5005: 1200,  // 雷伊倒影等
	132:  2800,  // 尤纳斯
	187:  3000000, // 魔狮迪露
	216:  10000, // 哈莫雷特
	264:  2500,  // 奈尼芬多
	421:  3000,  // 厄尔塞拉（含地图61特训，无独立 5019 资源）
	5019: 3000,  // 兼容旧配置
	166:  2000,  // 闪光波克尔（参考服 map10）
	261:  2000,  // 盖亚
	875:  2000,  // 布莱克
	274:  13000, // 塔克林
	1337: 10000, // 机械塔克林
	391:  10000, // 塔西亚
	393:  10000, // 上古炎兽
	347:  8000,  // 远古鱼龙
	798:  2000,  // 卡修斯
	804:  10000, // 迪符特
	715:  10000, // 德拉萨
	490:  5000,  // 劳克蒙德
	538:  10000, // 克拉尼特
	587:  1200,  // 墨杜萨
	617:  10000, // 肯佩德
	672:  5000,  // 亚伦斯
	5012: 5000,  // 亚伦斯（本服白虎表）
	589:  8000,  // 克瑞斯
	169:  450,   // 卡特斯 / 暗黑门
	171:  600,
	174:  800,
	177:  1000,
	195:  1000,
	222:  900,
	356:  1600,
	438:  1800,
	656:  2000,
	779:  1500,
	1182: 2000,
	1187: 2000,
	1403: 2000,
	183:  1100,
	192:  1200,
	224:  1300,
	227:  1400,
	297:  1500,
	359:  2000,
	441:  2000,
	435:  2000,
	659:  2100,
	661:  2200,
	784:  1200,
	782:  2500,
	1185: 2000,
	1397: 2000,
	1400: 3000,
	925:  4000, // 古尔扎迪
	957:  4000, // 米诺斯
	1723: 3000, // 安奈美
	153:  200,  // 小莹蜂（无参考表时给偏弱固定血）
	386:  5000, // 基维奥拉
	299:  8000, // 猛虎王
	413:  10000, // 塞维尔
	454:  400,  // 霹雳兽
	474:  400,  // 该伊
	462:  8000, // 阿尔达拉
	91:   500,  // 悠悠
	102:  800,  // 奇塔
	59:   400,  // 西塔
	74:   300,  // 果冻鸭
	144:  2000, // 赫卡特
	547:  500,  // 紫炎虫
	527:  8000, // 赫尔托克
}

const (
	mapIDXuanWu  = 401
	mapIDQingLong = 403
	mapIDBaiHu   = 483
	mapIDZhuQue  = 318
	mapIDYiNeng  = 677

	xuanWuGuardianHP   = 5000
	xuanWuBossHP       = 50000
	qingLongGuardianHP = 10000
	qingLongBossHP     = 50000
	baiHuGuardianHP    = 5000
	baiHuSecondStageHP = 1500
	baiHuBossHP        = 50000
)

// resolveBossFixedHP 返回应使用的固定血量；无覆盖时返回 original。
func resolveBossFixedHP(petID, original, mapID int, region uint32) int {
	if alias, ok := bossMapAlias[mapID]; ok {
		mapID = alias
	}
	// 谱尼封印
	if petID == petIDPuni && (mapID == 514 || mapID == 500 || mapID == 108) {
		if hp := puniSealMaxHP(region); hp > 0 {
			return hp
		}
	}
	// 玄武：p0 守护 / p1 真身（本前端 MapProcess_401）
	if mapID == mapIDXuanWu {
		if region == 0 {
			return xuanWuGuardianHP
		}
		if petID == 501 && region == 1 {
			return xuanWuBossHP
		}
	}
	// 青龙：p0 守护 / p1 真身
	if mapID == mapIDQingLong {
		if region == 0 {
			return qingLongGuardianHP
		}
		if petID == 502 && region == 1 {
			return qingLongBossHP
		}
	}
	// 白虎：p0 守护；p1–4 战虎/电虎；p5 泰格尔
	if mapID == mapIDBaiHu {
		if region == 0 {
			return baiHuGuardianHP
		}
		if region >= 1 && region <= 4 {
			return baiHuSecondStageHP
		}
		if petID == 503 && region == 5 {
			return baiHuBossHP
		}
	}
	// 朱雀
	if mapID == mapIDZhuQue {
		if hp := zhuQueHPByRegion(region); hp > 0 {
			return hp
		}
	}
	// 异能王空间
	if mapID == mapIDYiNeng && petID == 1000 {
		if region <= 5 {
			if region == 0 {
				return 2000
			}
			return 10000
		}
		if region == 6 {
			return 2000 // 终极首命基线（多命未完整移植）
		}
	}
	// 特例
	if mapID == 423 && region <= 2 && petID == 490 {
		return 2500 // 劳克蒙德三领域（参考服 XML）
	}
	if mapID == 423 && region == 5 && petID == 216 {
		return 1900
	}
	if mapID == 495 && region == 1 && petID == 755 {
		return 10000
	}
	if hp, ok := bossHPByPetID[petID]; ok && hp > 0 {
		return hp
	}
	return original
}

func zhuQueHPByRegion(region uint32) int {
	switch region {
	case 10:
		return 5000
	case 11:
		return 6000
	case 12:
		return 8000
	case 20, 21, 22:
		return 1000
	case 30:
		return 50000
	default:
		return 0
	}
}

// applyBossFixedHP 开战后按地图/region/petID 覆盖敌方血量。
func applyBossFixedHP(st *BattleState) {
	if st == nil || st.EnemyID <= 0 {
		return
	}
	orig := int(st.EnemyMaxHP)
	if orig <= 0 {
		orig = int(st.EnemyHP)
	}
	hp := resolveBossFixedHP(st.EnemyID, orig, st.MapID, st.BossRegion)
	if hp <= 0 {
		return
	}
	st.EnemyHP = uint32(hp)
	st.EnemyMaxHP = uint32(hp)
}
