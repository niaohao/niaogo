package gameserver

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/conslog"
	"niaohao/server/internal/defaults"
	"niaohao/server/internal/packet"
	"niaohao/server/internal/store"
	"niaohao/server/internal/tableloader"
)

const (
	flashPolicyRequest = "<policy-file-request/>"
	maxPacketBytes     = 1 << 20
	maxAccumBytes      = 4 << 20
	loginTaskListSize  = 1000 // 对齐本客户端 UserInfo.setForLoginInfo

	// 地图 1 传送舱出生点（MapConfig / 210.xml）
	defaultMapID = 1
	defaultPosX  = 475
	defaultPosY  = 395
)

type Client struct {
	Conn           net.Conn
	UserID         int64
	LoggedIn       bool
	IsRoom         bool // 房间服第二 TCP（与主连并存）
	MapID          int
	PosX           int
	PosY           int
	ActionType     uint32 // 0=走 / 1–4=NoNo 飞行形态（CMD 2112）
	ClothIDs       []uint32 // 2604 / user_worn_clothes
	LastMiniGameID uint32   // 5001 JOIN_GAME 记住，供 5002 结算
	mu             sync.Mutex
}

type Config struct {
	Port          int
	Store         *store.MySQL
	Catalog       *tableloader.Catalog
	DataDir       string // server/data：属性表、野怪配置
	AdvertiseHost string // 10002 回给客户端的房间 IP（与登录 105 一致）
}

// defaultSkillCatalog 供 buildPetInfo 等无 Server 指针的编解码取 MaxPP。
var defaultSkillCatalog *tableloader.Catalog

type Server struct {
	cfg        Config
	listener   net.Listener
	mu         sync.Mutex
	byUID      map[int64]*Client
	roomByUID  map[int64]*Client       // 房间第二连接；勿写入 byUID
	mapUsers   map[int]map[int64]*Client // mapID -> uid -> client
	battles    battleHub
	ogres      ogreHub
	petHP      petHPHub
	modes      modeHub
	pvpInvites pvpInviteHub
	bossShield bossShieldHub
	teams      *teamHub
	melee      grandMeleeHub
	arena      arenaHub
	wheel      wheelState
	groupInv   groupInviteHub
	hunt       huntHub
	teamPK     teamPKHub
}

func New(cfg Config) *Server {
	if cfg.Port == 0 {
		cfg.Port = defaults.GameTCP
	}
	if cfg.Catalog != nil {
		defaultSkillCatalog = cfg.Catalog
	}
	if cfg.DataDir != "" {
		_ = LoadBattleData(cfg.DataDir)
		tryLoadFusionFormulas(filepath.Join(cfg.DataDir, "..", "tables", "xml"))
	} else {
		tryLoadFusionFormulas(filepath.Join("tables", "xml"))
	}
	log.Printf("[fusion] formulas=%d transformSec=%d successRate=%d%%", fusionFormulaCount(), soulBeadTransformSec, fusionSuccessRate)
	s := &Server{
		cfg:       cfg,
		byUID:     make(map[int64]*Client),
		roomByUID: make(map[int64]*Client),
		mapUsers:  make(map[int]map[int64]*Client),
	}
	s.initTeamHub()
	return s
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", s.cfg.Port))
	if err != nil {
		return err
	}
	s.listener = ln
	log.Printf("[game] TCP :%d", s.cfg.Port)
	s.startOgreRefreshLoop()
	go s.accept()
	return nil
}

func (s *Server) ForceDisconnect(uid int64) {
	s.mu.Lock()
	c := s.byUID[uid]
	leaveMapID := 0
	if c != nil {
		leaveMapID = c.MapID
		delete(s.byUID, uid)
		s.removeFromMapLocked(c)
	}
	rc := s.roomByUID[uid]
	if rc != nil {
		delete(s.roomByUID, uid)
		s.removeFromMapLocked(rc)
	}
	s.mu.Unlock()
	if leaveMapID > 0 {
		s.notifyMapLeave(leaveMapID, uid)
	}
	s.battles.clear(uid)
	s.clearGrandMeleeSession(uid)
	if c != nil {
		_ = c.Conn.Close()
	}
	if rc != nil {
		_ = rc.Conn.Close()
	}
}

func (s *Server) accept() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			log.Printf("[game] accept: %v", err)
			return
		}
		log.Printf("[game] conn + %s", conn.RemoteAddr())
		go s.handle(&Client{Conn: conn})
	}
}

func (s *Server) handle(c *Client) {
	defer func() {
		leaveMapID := 0
		leaveUID := int64(0)
		s.mu.Lock()
		isRoom := c.IsRoom
		if isRoom {
			if c.UserID > 0 && s.roomByUID[c.UserID] == c {
				delete(s.roomByUID, c.UserID)
			}
		} else if c.UserID > 0 && s.byUID[c.UserID] == c {
			delete(s.byUID, c.UserID)
		}
		if !isRoom && c.UserID > 0 && c.MapID > 0 {
			leaveMapID, leaveUID = c.MapID, c.UserID
		}
		s.removeFromMapLocked(c)
		s.mu.Unlock()
		if leaveMapID > 0 {
			s.notifyMapLeave(leaveMapID, leaveUID)
		}
		// 房间断线不触发主号下线（PvP/邀请等）
		if c.UserID > 0 && !isRoom {
			s.onUserDisconnect(c.UserID)
		}
		log.Printf("[game] conn - %s UID=%d room=%v", c.Conn.RemoteAddr(), c.UserID, isRoom)
		_ = c.Conn.Close()
	}()
	buf := make([]byte, 8192)
	var acc []byte
	for {
		n, err := c.Conn.Read(buf)
		if err != nil {
			return
		}
		acc = append(acc, buf[:n]...)
		if len(acc) > maxAccumBytes {
			return
		}
		for s.consumePolicy(c, &acc) {
		}
		s.process(c, &acc)
	}
}

func (s *Server) consumePolicy(c *Client, acc *[]byte) bool {
	b := *acc
	if len(b) < len(flashPolicyRequest) {
		return false
	}
	if string(b[:len(flashPolicyRequest)]) != flashPolicyRequest {
		return false
	}
	n := len(flashPolicyRequest)
	if len(b) > n && b[n] == 0 {
		n++
	}
	*acc = b[n:]
	policy := `<?xml version="1.0"?>
<!DOCTYPE cross-domain-policy SYSTEM "/xml/dtds/cross-domain-policy.dtd">
<cross-domain-policy>
	<allow-access-from domain="*" to-ports="*" />
</cross-domain-policy>` + "\x00"
	_, _ = c.Conn.Write([]byte(policy))
	return true
}

func (s *Server) process(c *Client, acc *[]byte) {
	b := *acc
	for len(b) >= packet.HeaderLen {
		pktLen := int(binary.BigEndian.Uint32(b[0:4]))
		if pktLen < packet.HeaderLen || pktLen > maxPacketBytes {
			*acc = nil
			return
		}
		if len(b) < pktLen {
			break
		}
		pkt := b[:pktLen]
		b = b[pktLen:]
		s.dispatch(c, pkt)
	}
	*acc = b
}

func (s *Server) dispatch(c *Client, pkt []byte) {
	_, cmd, uid, seq, body, err := packet.ParseHeader(pkt)
	if err != nil {
		return
	}
	userID := int64(uid)
	switch cmd {
	case 1001:
		log.Printf("[CMD] OK     %s UID=%d SEQ=%d body=%d (进服)", cmdname.Format(cmd), userID, seq, len(body))
		s.handleLoginIn(c, userID, body)
	case 1002:
		log.Printf("[CMD] OK     %s UID=%d SEQ=%d body=%d", cmdname.Format(cmd), userID, seq, len(body))
		s.handleSystemTime(c, uid)
	case 1004: // MAP_HOT
		s.handleMapHot(c, uid, body)
	case 1020: // 客户端行为上报
		s.handleClientReport(c, uid, body)
	case 1022: // 图鉴/脚本校验 stub
		s.send(c, 1022, uid, 0, nil)
		log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(1022), uid)
	case 1005: // GET_IMAGE_ADDRES
		s.handleGetImageAddress(c, uid, body)
	case 1007: // READ_COUNT
		s.handleReadCount(c, uid, body)
	case 1108, 1110, 1111, 1112, 2065: // 节日礼包/铭牌
		s.handleFestivalGift(c, uid, cmd, body)
	case 2023: // OFF_LINE_EXP
		s.handleOfflineExpQuery(c, uid, body)
	case 2053: // REQUEST_COUNT
		s.handleRequestCount(c, uid, body)
	case 2061: // CHANG_NICK_NAME
		s.handleChangeNickName(c, uid, body)
	case 2062: // CHANGE_DOODLE
		s.handleChangeDoodle(c, uid, body)
	case 2063: // CHANGE_COLOR
		s.handleChangeColor(c, uid, body)
	case 2064: // GET_REQUEST_AWARD
		s.handleGetRequestAward(c, uid, body)
	case 2101:
		s.handlePeopleWalk(c, uid, body)
	case 2102: // CHAT
		s.handleChat(c, uid, body)
	case 2103: // DANCE_ACTION
		s.handleDanceAction(c, uid, body)
	case 2104: // AIMAT 头部射击
		s.handleAimat(c, uid, body)
	case 2105: // HIT_STONE
		s.handleHitStone(c, uid, body)
	case 2106: // PRIZE_OF_ATRESIASPACE 阿特莱斯奖励
		s.handlePrizeOfAtresiaSpace(c, uid, body)
	case 2107: // TRANSFORM_USER
		s.handleNoteTransformUser(c, uid, body)
	case 2108: // NOTE_TRANSFORM_USER（多推送；请求则空 ACK）
		s.handleRemainEmptyAck(c, uid, 2108)
	case 2109: // ATTACK_BAILUEN
		s.handleAttackBailuen(c, uid, body)
	case 2110: // GET_TIMEPOKE
		s.handleGetTimePoke(c, uid, body)
	case 2111: // PEOPLE_TRANSFROM
		s.handlePeopleTransform(c, uid, body)
	case 2112: // ON_OR_OFF_FLYING NoNo 飞行
		s.handleOnOrOffFlying(c, uid, body)
	case 2113: // REMOVE_COINS
		s.handleRemoveCoins(c, uid, body)
	case 2120, 2121: // IcicleGame
		s.handleIcicleGame(c, uid, cmd, body)
	case 2148: // M_2148 盖亚魂印设置
		s.handleGaiyaEffectSet(c, uid, body)
	case 2149: // M_2149 盖亚魂印
		s.handleGaiyaEffect(c, uid, body)
	case 2150:
		log.Printf("[CMD] OK     %s UID=%d SEQ=%d body=%d", cmdname.Format(cmd), userID, seq, len(body))
		s.handleRelationList(c, uid)
	case 2151:
		s.handleFriendAdd(c, uid, body)
	case 2152:
		s.handleFriendAnswer(c, uid, body)
	case 2153:
		s.handleFriendRemove(c, uid, body)
	case 2154:
		s.handleBlackAdd(c, uid, body)
	case 2155:
		s.handleBlackRemove(c, uid, body)
	case 2157:
		s.handleSeeOnline(c, uid, body)
	case 2158:
		s.handleRequestOut(c, uid, body)
	case 2159:
		s.handleRequestAnswer(c, uid, body)
	case 2201, 2231: // ACCEPT_TASK / ACCEPT_DAILY_TASK
		s.handleAcceptTask(c, uid, body)
	case 2202, 2233: // COMPLETE_TASK / COMPLETE_DAILY_TASK → NoviceFinishInfo
		s.handleCompleteTask(c, uid, cmd, body)
	case 2203, 2234: // GET_TASK_BUF / GET_DAILY_TASK_BUF
		log.Printf("[CMD] OK     %s UID=%d SEQ=%d body=%d", cmdname.Format(cmd), userID, seq, len(body))
		s.handleGetTaskBuf(c, uid, body)
	case 2204, 2235: // ADD_TASK_BUF / ADD_DAILY_TASK_BUF
		s.handleAddTaskBuf(c, uid, body)
	case 2205, 2232: // DELETE_TASK / DELETE_DAILY_TASK
		s.handleDeleteTask(c, uid, body, cmd)
	case 2206: // CHANGE_TASK_STATUES
		s.handleChangeTaskStatues(c, uid, body)
	case 2251: // EXCHANGE_ORE
		s.handleExchangeOre(c, uid, body)
	case 2301: // GET_PET_INFO
		log.Printf("[CMD] OK     %s UID=%d SEQ=%d body=%d", cmdname.Format(cmd), userID, seq, len(body))
		s.handleGetPetInfo(c, uid, body)
	case 2302: // MODIFY_PET_NAME
		s.handleModifyPetName(c, uid, body)
	case 2303: // GET_PET_LIST 仓库列表
		s.handleGetPetList(c, uid, body)
	case 2304: // PET_RELEASE 背包/仓库互转
		log.Printf("[CMD] OK     %s UID=%d SEQ=%d body=%d", cmdname.Format(cmd), userID, seq, len(body))
		s.handlePetRelease(c, uid, body)
	case 2305: // PET_SHOW 跟随展示
		s.handlePetShow(c, uid, body)
	case 2306: // PET_CURE 全员治疗
		s.handlePetCureAll(c, uid)
	case 2307: // PET_STUDY_SKILL 学习/替换技能
		s.handlePetStudySkill(c, uid, body)
	case 2308: // PET_DEFAULT 设首发
		s.handleSetDefaultPet(c, uid, body)
	case 2309: // PET_BARGE_LIST 形态/击杀进度（光之迷城等）
		s.handlePetBargeList(c, uid, body)
	case 2310: // PET_ONE_CURE 单宠治疗
		s.handlePetOneCure(c, uid, body)
	case 2311: // PET_COLLECT 精灵收集计划
		s.handlePetCollect(c, uid, body)
	case 2312: // PET_SKILL_SWICTH 技能唤醒仪替换
		s.handlePetSkillSwitch(c, uid, body)
	case 2313: // IS_COLLECT
		s.handleISCollect(c, uid, body)
	case 2314: // PET_EVOLVTION 进化仓
		s.handlePetEvolve(c, uid, body)
	case 2315: // PET_HATCH
		s.handlePetHatch(c, uid, body)
	case 2316: // PET_HATCH_GET
		s.handlePetHatchGet(c, uid, body)
	case 2317: // PRIZE_OF_PETKING
		s.handlePrizeOfPetKing(c, uid, body)
	case 2318: // PET_SET_EXP 经验池分配
		s.handlePetSetExp(c, uid, body)
	case 2319: // PET_GET_EXP 读经验池
		s.handlePetGetExp(c, uid)
	case 2364: // GET_BREED_PET 培育仓选雌
		s.handleBreedPet(c, uid, body)
	case 2365: // GET_BREED_INFO
		s.handleBreedInfo(c, uid)
	case 2367: // GET_EGG_LIST
		s.handleEggList(c, uid)
	case 2368: // START_HATCH
		s.handleStartHatch(c, uid, body)
	case 2369: // EFFECT_HATCH
		s.handleEffectHatch(c, uid, body)
	case 2370: // GET_HATCH_PET
		s.handleGetHatchPet(c, uid)
	case 2374: // START_BREED
		s.handleStartBreed(c, uid, body)
	case 2320: // PET_ROWEI_LIST 放生仓
		s.handlePetRoweiList(c, uid)
	case 2321: // PET_ROWEI
		s.handlePetRowei(c, uid, body)
	case 2322: // PET_RETRIEVE
		s.handlePetRetrieve(c, uid, body)
	case 2323: // PET_ROOM_SHOW 基地展示精灵
		s.handlePetRoomShow(c, uid, body)
	case 2324: // PET_ROOM_LIST 基地展示列表
		s.handlePetRoomList(c, uid, body)
	case 2325: // PET_ROOM_INFO 基地精灵简略信息
		s.handlePetRoomInfo(c, uid, body)
	case 2326: // USE_PET_ITEM_OUT_OF_FIGHT
		s.handleUsePetItemOutOfFight(c, uid, body)
	case 2327: // USE_SPEEDUP_ITEM 双倍/三倍经验
		s.handleUseSpeedupItem(c, uid, body)
	case 2328: // Skill_Sort / 加速器次数查询
		s.handleSkillSort(c, uid, body)
	case 2329: // USE_AUTO_FIGHT_ITEM
		s.handleUseAutoFightItem(c, uid, body)
	case 2330: // ON_OFF_AUTO_FIGHT
		s.handleOnOffAutoFight(c, uid, body)
	case 2331: // USE_ENERGY_XISHOU
		s.handleUseEnergyXishou(c, uid, body)
	case 2332: // USE_STUDY_ITEM 学习力双倍仪
		s.handleUseStudyItem(c, uid, body)
	case 2333: // PET_ELITE_COLLECT
		s.handlePetEliteCollect(c, uid, body)
	case 2334: // PET_ELITE_UNCOLLECT
		s.handlePetEliteUncollect(c, uid, body)
	case 2375: // START_USE_ITEM_HATCH 极速孵化剂
		s.handleFastHatch(c, uid, body)
	case 2373: // SEND_EGG_TOFRIEND
		s.handleSendEggToFriend(c, uid, body)
	case 2377: // UP_GRADE_MEDICINE
		s.handleUpgradeMedicine(c, uid, body)
	case 9755: // QUICKBREED_AFTER_STARTBREED
		s.handleFastHatchQuick(c, uid)
	case 3201: // EGG_GAME_PLAY 扭蛋机
		s.handleGacha(c, uid, body)
	case 5001: // JOIN_GAME 小游戏进入
		s.handleMiniGameEnter(c, uid, body)
	case 5002: // GAME_OVER 小游戏结算
		s.handleMiniGameOver(c, uid, body)
	case 5003: // LEAVE_GAME
		s.handleLeaveGame(c, uid, body)
	case 70004: // EXCHANGE_GOLD_NIEOBEAN
		s.handleExchangeGoldNeoBean(c, uid, body)
	case 80007: // GET_CURRENT_GOLD_NIEOBEAN
		s.handleGetCurrentGoldNeoBean(c, uid)
	case 80010: // USE_SOUL_BEAD_FAST_HATCH
		s.handleSoulBeadFastHatch(c, uid, body)
	case 9113: // SET_PET_CONST_FORM 形态固定/还原
		s.handleSetPetConstForm(c, uid, body)
	case 2343: // PET_RESET_NATURE / 学习力次数
		s.handlePetResetNature(c, uid, body)
	case 2342: // USE_FIGHT_HP_ITEM
		s.handleUseFightHPItem(c, uid, body)
	case 2336: // GET_PET_SKILL 可学技能增量（唤醒仪/背包）
		s.handleGetPetSkill(c, uid, body)
	case 2393: // LEIYI_TRAIN_GET_STATUS 雷伊体能特训
		s.handleLeiyiTrainGetStatus(c, uid, body)
	case 9278: // USE_PET_ITEM_FULL_ABILITY_OF_STUDY 学习力注入
		s.handleUsePetItemFullAbilityOfStudy(c, uid, body)
	case 2351:
		s.handlePetFusion(c, uid, body)
	case 2352:
		s.handleGetSoulBeadBuf(c, uid, body)
	case 2353:
		s.handleSetSoulBeadBuf(c, uid, body)
	case 2354:
		s.handleSoulBeadList(c, uid)
	case 2356:
		s.handleGetSoulBeadStatus(c, uid, body)
	case 2357:
		s.handleTransformSoulBead(c, uid, body)
	case 2358:
		s.handleSoulBeadToPet(c, uid, body)
	case 2411: // CHALLENGE_BOSS → 2503
		s.handleChallengeBoss(c, uid, body)
	case 2412: // ATTACK_BOSS 破 SPT 防护罩（蘑菇怪等）
		s.handleAttackBoss(c, uid, body)
	case 2413: // PET_KING_JOIN
		s.handlePetKingJoin(c, uid, body)
	case 2414: // CHOICE_FIGHT_LEVEL 勇者之塔选层
		s.handleChoiceFightLevel(c, uid, body)
	case 2415: // START_FIGHT_LEVEL
		s.handleStartFightLevel(c, uid)
	case 2416: // LEAVE_FIGHT_LEVEL
		s.handleLeaveFightLevel(c, uid)
	case 2417: // ARENA_SET_OWENR
		s.handleArenaSetOwenr(c, uid, body)
	case 2418: // ARENA_FIGHT_OWENR
		s.handleArenaFightOwenr(c, uid, body)
	case 2419: // ARENA_GET_INFO
		s.handleArenaGetInfo(c, uid, body)
	case 2420: // ARENA_UPFIGHT
		s.handleArenaUpfight(c, uid, body)
	case 2421: // FIGHT_SPECIAL_PET 盖亚的出现
		s.handleFightSpecialPet(c, uid)
	case 2422: // ARENA_OWENR_ACCE
		s.handleArenaOwenrAcce(c, uid, body)
	case 2423: // ARENA_OWENR_OUT
		s.handleArenaOwenrOut(c, uid, body)
	case 2424: // OPEN_DARKPORTAL
		s.handleOpenDarkPortal(c, uid, body)
	case 2425: // FIGHT_DARKPORTAL
		s.handleFightDarkPortal(c, uid)
	case 2426: // LEAVE_DARKPORTAL
		s.handleLeaveDarkPortal(c, uid)
	case 2427: // NPC_JOIN
		s.handleNPCJoin(c, uid, body)
	case 2428: // FRESH_CHOICE_FIGHT_LEVEL
		s.handleFreshChoiceFightLevel(c, uid, body)
	case 2429: // FRESH_START_FIGHT_LEVEL
		s.handleFreshStartFightLevel(c, uid)
	case 2430: // FRESH_LEAVE_FIGHT_LEVEL
		s.handleFreshLeaveFightLevel(c, uid)
	case 2431: // START_PET_WAR 精灵大乱斗
		s.handleGrandMeleeJoin(c, uid)
	case 2452: // TURN_FORTUNE_WHEEL
		s.handleTurnFortuneWheel(c, uid, body)
	case 2453: // START_FORTUNE_WHEEL（CommandID 名以 AS 为准）
		s.handleStartFortuneWheel(c, uid, body)
	case 2454: // LEAVE_FORTUNE_WHEEL
		s.handleLeaveFortuneWheel(c, uid, body)
	case 2458: // PET_TOPLEVEL_JOIN
		s.handlePetTopLevelJoin(c, uid, body)
	case 2493: // MYSTERYHOLE_JOIN
		s.handleMysteryHoleJoin(c, uid, body)
	case 2494: // MYSTERYHOLE_PK
		s.handleMysteryHolePK(c, uid, body)
	case 2495: // MYSTERYHOLE_EXIT
		s.handleMysteryHoleExit(c, uid, body)
	case 2496: // MYSTERYHOLE_BOX
		s.handleMysteryHoleBox(c, uid, body)
	case 2497: // MYSTERYHOLE_FRONT
		s.handleMysteryHoleFront(c, uid, body)
	case 2499: // MYSTERYHOLE_DATA
		s.handleMysteryHoleData(c, uid, body)
	case 2401: // INVITE_TO_FIGHT
		s.handleInviteToFight(c, uid, body)
	case 2402: // INVITE_FIGHT_CANCEL
		s.handleInviteFightCancel(c, uid)
	case 2403: // HANDLE_FIGHT_INVITE
		s.handleHandleFightInvite(c, uid, body)
	case 2404: // READY_TO_FIGHT → 2504
		s.handleReadyToFight(c, uid)
	case 2405: // USE_SKILL → 2505/2506
		s.handleUseSkill(c, uid, body)
	case 2406: // USE_PET_ITEM
		s.handleUsePetItem(c, uid, body)
	case 2407: // CHANGE_PET
		s.handleChangePet(c, uid, body)
	case 2408: // FIGHT_NPC_MONSTER → 2503
		s.handleFightNpcMonster(c, uid, body)
	case 2409: // CATCH_MONSTER
		s.handleCatchMonster(c, uid, body)
	case 2410: // ESCAPE_FIGHT → 2506
		s.handleEscapeFight(c, uid)
	case 2441: // LOAD_PERCENT
		s.handleFightLoadPercent(c, uid, body)
	case 2442: // ML_FIG_BOSS
		s.handleMlFigBoss(c, uid, body)
	case 2444: // ML_STATE_BOSS
		s.handleMlStateBoss(c, uid)
	case 2445: // ML_STEP_POS
		s.handleMlStepPos(c, uid, body)
	case 2446: // ML_GET_PRIZE
		s.handleMlGetPrize(c, uid)
	case 2567: // TOPFIGHT_BEYOND
		s.handleTopFightBeyond(c, uid, body)
	case 46001: // GET_TOP_WAR_RANK
		s.handleGetTopWarRank(c, uid)
	case 46002: // PVP_RANK_REWARD
		s.handlePvpRankReward(c, uid)
	case 70003: // GET_HONOR_VALUE
		s.handleGetHonorValue(c, uid)
	case 70006: // GET_RECRUITSTATES
		s.handleGetRecruitStates(c, uid)
	case 70007: // GET_RECRUITREWARD4S
		s.handleGetRecruitReward(c, uid, body)
	case 9297: // NONO_VIP_DAILY_SIGN 超No每日签到
		s.handleNonoVipDailySign(c, uid)
	case 9298: // NONO_VIP_DAILY_SIGN_INFO
		s.handleNonoVipDailySignInfo(c, uid)
	case 9299: // NONO_VIP_DAILY_SIGN_BEE 小蜜蜂奖
		s.handleNonoVipDailySignBee(c, uid)
	case 1006: // GET_SESSION_KEY
		s.handleGetSessionKey(c, uid)
	case 1101: // MONEY_CHECK_PSW
		s.handleMoneyCheckPsw(c, uid)
	case 1102: // MONEY_BUY_PRODUCT
		s.handleMoneyBuyProduct(c, uid, body)
	case 1103: // MONEY_CHECK_REMAIN
		s.handleMoneyCheckRemain(c, uid)
	case 1104: // GOLD_BUY_PRODUCT
		s.handleGoldBuyProduct(c, uid, body)
	case 1105: // GOLD_CHECK_REMAIN
		s.handleGoldCheckRemain(c, uid)
	case 2601: // ITEM_BUY
		s.handleItemBuy(c, uid, body)
	case 2602: // ITEM_SALE
		s.handleItemSale(c, uid, body)
	case 2603: // ITEM_REPAIR
		s.handleItemRepair(c, uid)
	case 2604: // CHANGE_CLOTH
		s.handleChangeCloth(c, uid, body)
	case 2605: // ITEM_LIST
		s.handleItemList(c, uid, body)
	case 2701: // TALK_COUNT 对话/日常领取次数
		s.handleTalkCount(c, uid, body)
	case 2702: // TALK_CATE 船长室/发明室/挖矿领取
		s.handleTalkCate(c, uid, body)
	case 2606: // MULTI_ITEM_BUY
		s.handleMultiItemBuy(c, uid, body)
	case 2607: // ITEM_EXPEND
		s.handleItemExpend(c, uid, body)
	case 2608: // GET_LAS_EGG 实验室里奥斯精元
		s.handleGetLasEgg(c, uid)
	case 2609: // EQUIP_UPDATA
		s.handleEquipUpdate(c, uid, body)
	case 2610: // EAT_SPECIAL_MEDICINE
		s.handleEatSpecialMedicine(c, uid, body)
	case 2611: // MASTER_REWARDS
		s.handleMasterRewards(c, uid, body)
	case 2612: // GET_PET_KING_REWARDS
		s.handleGetPetKingRewards(c, uid, body)
	case 2621: // EXCHANGEMASTER_CARDS
		s.handleExchangeMasterCards(c, uid, body)
	case 2821: // USER_TIME_PASSWORD
		s.handleUserTimePassword(c, uid, body)
	case 2851, 2852: // SET_DS_STATUS / PRICE_OF_DS
		s.handleDSStatus(c, uid, cmd, body)
	case 2910: // TEAM_CREATE
		s.handleTeamCreate(c, uid, body)
	case 2911: // TEAM_ADD
		s.handleTeamAdd(c, uid, body)
	case 2912: // TEAM_ANSWER
		s.handleTeamAnswer(c, uid, body)
	case 2913: // TEAM_INFORM
		s.handleTeamInformPull(c, uid, body)
	case 2914: // TEAM_QUIT
		s.handleTeamQuit(c, uid, body)
	case 2915: // TEAM_CHANGE_ADMIN
		s.handleTeamChangeAdmin(c, uid, body)
	case 2916: // TEAM_DELET_MEMBER
		s.handleTeamDeleteMember(c, uid, body)
	case 2917: // TEAM_GET_INFO
		s.handleTeamGetInfo(c, uid, body)
	case 2918: // TEAM_GET_MEMBER_LIST
		s.handleTeamGetMemberList(c, uid, body)
	case 2920: // TEAM_SET_JOIN_FLAG
		s.handleTeamSetJoinFlag(c, uid, body)
	case 2921: // TEAM_SET_SLOGAN
		s.handleTeamSetSlogan(c, uid, body)
	case 2922: // TEAM_MODIFY_LOGO
		s.handleTeamModifyLogo(c, uid, body)
	case 2923: // TEAM_GIVE_SUPER_CORE
		s.handleTeamGiveSuperCore(c, uid, body)
	case 2924: // TEAM_GET_SUPER_CORE
		s.handleTeamGetSuperCore(c, uid, body)
	case 2925: // TEAM_SELECT_SUPER_CORE
		s.handleTeamSelectSuperCore(c, uid, body)
	case 2926: // TEAM_CREAT_ITEM
		s.handleTeamCreatItem(c, uid, body)
	case 2927: // TEAM_SHOW_LOGO
		s.handleTeamShowLogo(c, uid, body)
	case 2928: // TEAM_GET_LOGO_INFO
		s.handleTeamGetLogoInfo(c, uid, body)
	case 2929: // TEAM_CHAT
		s.handleTeamChat(c, uid, body)
	case 2930: // TEAM_INVITE_TO_JOIN
		s.handleTeamInviteToJoin(c, uid, body)
	case 2931: // TEAM_SET_NOTICE
		s.handleTeamSetNotice(c, uid, body)
	case 2932: // Get_CONTRIBUTE_BOUNDS
		s.handleContributeBounds(c, uid, body)
	case 2941:
		s.handleTeamArmGetUsedInfo(c, uid, body)
	case 2942:
		s.handleTeamArmGetAllInfo(c, uid, body)
	case 2943:
		s.handleTeamArmPlace2943(c, uid, body)
	case 2944:
		s.handleTeamArmSetInfo2944(c, uid, body)
	case 2945:
		s.handleTeamArmTakeBack2945(c, uid, body)
	case 2951:
		s.handleTeamArm2951(c, uid, body)
	case 2952:
		s.handleTeamArm2952(c, uid, body)
	case 2953:
		s.handleTeamArm2953(c, uid, body)
	case 2954:
		s.handleTeamArm2954(c, uid, body)
	case 2961:
		s.handleTeamArmUpBuy2961(c, uid, body)
	case 2962:
		s.handleTeamArmUpWork2962(c, uid, body)
	case 2963:
		s.handleTeamArmUpDonate2963(c, uid, body)
	case 2964:
		s.handleTeamArmUpSetInfo2964(c, uid, body)
	case 2965:
		s.handleTeamArmUpGetUsedInfo2965(c, uid, body)
	case 2966:
		s.handleTeamArmUpGetAllInfo2966(c, uid, body)
	case 2967:
		s.handleTeamArmUpGetOneInfo2967(c, uid, body)
	case 2968:
		s.handleTeamArmBuy2968(c, uid, body)
	case 2969:
		s.handleTeamArmUpOpenUpdate2969(c, uid, body)
	case 2970:
		s.handleTeamArmUpGetUpdate2970(c, uid, body)
	case 3001: // REQUEST_ADD_TEACHER
		s.handleRequestAddTeacher(c, uid, body)
	case 3002: // ANSWER_ADD_TEACHER
		s.handleAnswerAddTeacher(c, uid, body)
	case 3003: // REQUEST_ADD_STUDENT
		s.handleRequestAddStudent(c, uid, body)
	case 3004: // ANSWER_ADD_STUDENT
		s.handleAnswerAddStudent(c, uid, body)
	case 3005: // DELETE_TEACHER
		s.handleDeleteTeacher(c, uid, body)
	case 3006: // DELETE_STUDENT
		s.handleDeleteStudent(c, uid, body)
	case 3007: // EXPERIENCESHARED_COMPLETE
		s.handleExperienceShared(c, uid, body)
	case 3008: // TEACHERREWARD_COMPLETE
		s.handleTeacherReward(c, uid, body)
	case 3009: // MYEXPERIENCEPOND_COMPLETE
		s.handleMyExperiencePond(c, uid, body)
	case 3010: // SEVENNOLOGIN_COMPLETE
		s.handleSevenNoLogin(c, uid, body)
	case 3011: // GETMYEXPERIENCE_COMPLETE
		s.handleGetMyExperience(c, uid, body)
	case 3403: // ACHIEVETITLELIST
		s.handleAchieveTitleList(c, uid)
	case 3404: // SETTITLE
		s.handleSetTitle(c, uid, body)
	case 3301: // AWARD_CODE
		s.handleAwardCode(c, uid, body)
	case 4140: // BUERSIGUANG_ATTRIBUTES_FIX
		s.handleBuersiguangFix(c, uid, body)
	case 5052: // FB_GAME_OVER
		s.handleFBGameOver(c, uid, body)
	case 6001, 6003: // WORK_CONNECTION / ALL_CONNECTION
		s.handleWorkConnection(c, uid, cmd, body)
	case 7001, 7002, 7003: // COMPLAIN / CONTRIBUTE / INDAGATE
		s.handleComplainUser(c, uid, cmd, body)
	case 7501: // INVITE_JOIN_GROUP
		s.handleInviteJoinGroup(c, uid, body)
	case 7502: // REPLY_JOIN_GROUP
		s.handleReplyJoinGroup(c, uid, body)
	case 4001: // TEAM_PK_SIGN
		s.handleTeamPKSign(c, uid, body)
	case 4002: // TEAM_PK_REGISTER
		s.handleTeamPKRegister(c, uid, body)
	case 4003: // TEAM_PK_JOIN
		s.handleTeamPKJoin(c, uid, body)
	case 4011: // TEAM_PK_BUILDING
		s.handleTeamPKBuilding(c, uid, body)
	case 4012: // TEAM_PK_SITUATION
		s.handleTeamPKSituation(c, uid, body)
	case 4004, 4005, 4006, 4007, 4008, 4009, 4010, 4013, 4014, 4020, 4023, 4024, 4025, 4101, 4102, 2481:
		s.handleTeamPKEmptyStub(c, uid, cmd)
	case 4022: // TEAM_PK_ACTIVE
		s.handleTeamPKActive(c, uid, body)
	case 4017: // TEAM_PK_WEEKY_SCORE
		s.handleTeamPKWeekyScore(c, uid, body)
	case 4018: // TEAM_PK_HISTORY
		s.handleTeamPKHistory(c, uid, body)
	case 4019: // TEAM_PK_SOMEONE_JOIN_INFO
		s.handleTeamPKSomeoneJoin(c, uid, body)
	case 2751:
		s.handleMailGetList(c, uid, body)
	case 2752:
		s.handleMailSend(c, uid, body)
	case 2753:
		s.handleMailGetContent(c, uid, body)
	case 2754:
		s.handleMailSetReaded(c, uid, body)
	case 2755:
		s.handleMailDelete(c, uid, body)
	case 2756:
		s.handleMailDelAll(c, uid)
	case 2757:
		log.Printf("[CMD] OK     %s UID=%d SEQ=%d body=%d", cmdname.Format(cmd), userID, seq, len(body))
		s.handleMailUnread(c, uid)
	case 70000: // PET_GENE_RECAST 实验室基因重组
		s.handlePetGeneRecast(c, uid, body)
	case 70001: // GET_EXCHANGE_INFO 荣誉兑换剩余次数
		s.handleExchangeInfo(c, uid)
	case 70002: // EXCHANGE_ITEM 荣誉兑换
		s.handleExchangeItem(c, uid, body)
	case 80003: // ACTIVEACHIEVE
		s.handleActiveAchieve(c, uid, body)
	case 80004: // ACHIEVELIST
		s.handleAchieveList(c, uid)
	case 80005: // ACHIEVE_CURRENT
		s.handleAchieveCurrent(c, uid)
	case 80006: // ACHIEVEINFO
		s.handleAchieveInfo(c, uid, body)
	case 9001: // NONO_OPEN
		s.handleNonoOpen(c, uid)
	case 9002: // NONO_CHANGE_NAME
		s.handleNonoChangeName(c, uid, body)
	case 9003:
		log.Printf("[CMD] OK     %s UID=%d SEQ=%d body=%d", cmdname.Format(cmd), userID, seq, len(body))
		s.handleNonoInfo(c, uid)
	case 9004: // NONO_CHIP_MIXTURE
		s.handleNonoChipMixture(c, uid, body)
	case 9008: // NONO_EXPADM
		s.handleNonoExpadm(c, uid)
	case 9010: // NONO_IMPLEMENT_TOOL
		s.handleNonoImplementTool(c, uid, body)
	case 9012: // NONO_CHANGE_COLOR
		s.handleNonoChangeColor(c, uid, body)
	case 9013: // NONO_PLAY
		s.handleNonoPlay(c, uid, body)
	case 9023: // NONO_GET_CHIP
		s.handleNonoGetChip(c, uid, body)
	case 9014: // NONO_CLOSE_OPEN
		s.handleNonoCloseOpen(c, uid, body)
	case 9007: // NONO_CURE
		s.handleNonoCure(c, uid)
	case 9015: // NONO_EXE_LIST
		s.handleNonoExeList(c, uid)
	case 9016: // NONO_CHARGE
		s.handleNonoCharge(c, uid, body)
	case 9017: // NONO_START_EXE
		s.handleNonoStartExe(c, uid, body)
	case 9018: // NONO_END_EXE
		s.handleNonoEndExe(c, uid, body)
	case 9019: // NONO_FOLLOW_OR_HOOM
		s.handleNonoFollowOrHoom(c, uid, body)
	case 9020: // NONO_OPEN_SUPER
		s.handleNonoOpenSuper(c, uid)
	case 9021: // NONO_HELP_EXP → 经验池
		s.handleNonoHelpExp(c, uid)
	case 9022: // NONO_MATE_CHANGE
		s.handleNonoMateChange(c, uid)
	case 9024: // NONO_ADD_ENERGY_MATE
		s.handleNonoAddEnergyMate(c, uid)
	case 9025: // GET_DIAMOND
		s.handleGetDiamond(c, uid, body)
	case 9026, 9027: // NONO_ADD_EXP / NONO_IS_INFO（可空包体）
		s.handleNonoMapPetExp(c, uid, cmd, body)
	case 9032: // GET_NONOPARTY_EXP
		s.handleNonoPartyGetExp(c, uid, body)
	case 9033: // GET_NONOPARTY_ITEM
		s.handleNonoPartyGetItem(c, uid, body)
	case 9088: // FIRE_EDGE_REBORN_CHECK_EAT_MEDICINE 背包特效查询
		s.handleFireEdgeCheckEatMedicine(c, uid, body)
	case 9145: // FIRE_EDGE_REBORN_USE_BREED_CONVERT_ITEM
		s.handleFireEdgeBreedConvert(c, uid, body)
	case 9222: // GET_BREED_INTRO_MOVIE_GIFT
		s.handleBreedIntroMovieGift(c, uid, body)
	case 9388, 9394: // ARESUNIONCHALLENGE_*
		s.handleAresUnionTrain(c, uid, cmd, body)
	case 30000: // TEST
		s.handleTestCMD(c, uid, body)
	case 70008: // PVP_DELEVELING
		s.handlePvpDeleveling(c, uid, body)
	case 70009: // GET_WHEELCHOICE_DATA
		s.handleGetWheelChoiceData(c, uid, body)
	case 70010: // ENTER_WHEELCHOICE
		s.handleEnterWheelChoice(c, uid, body)
	case 80009: // USE_BAG_ITEM
		s.handleUseBagItem(c, uid, body)
	case 80011: // PLAY_VIDEO
		s.handlePlayVideo(c, uid, body)
	case 80012: // SUKE_EXCHANGE
		s.handleSukeExchange(c, uid, body)
	case 80013: // HUNT_FIGHT_START
		s.handleHuntFightStart(c, uid, body)
	case 80014: // HUNT_FIGHT_ACTION
		s.handleHuntFightAction(c, uid, body)
	case 80015: // HUNT_FIGHT_OVER
		s.handleHuntFightOver(c, uid, body)
	case 80016: // QUERY_DREAMPET
		s.handleQueryDreamPet(c, uid, body)
	case 80001: // OPEN_SUPER_NONO
		s.handleOpenSuperNono(c, uid)
	case 1106: // GOLD_ONLINE_CHECK_REMAIN
		s.handleGoldOnlineCheckRemain(c, uid)
	case 10001: // ROOM_LOGIN（房间第二 TCP）
		s.handleRoomLogin(c, uid, body)
	case 10002: // GET_ROOM_ADDRES
		s.handleGetRoomAddress(c, uid, body)
	case 10003: // LEAVE_ROOM
		s.handleLeaveRoom(c, uid, body)
	case 10004: // BUY_FITMENT
		s.handleBuyFitment(c, uid, body)
	case 10005: // BETRAY_FITMENT
		s.handleBetrayFitment(c, uid)
	case 10006: // FITMENT_USERING
		s.handleFitmentUsering(c, uid, body)
	case 10007: // FITMENT_ALL
		s.handleFitmentAll(c, uid)
	case 10008: // SET_FITMENT
		s.handleSetFitment(c, uid, body)
	case 10009: // ADD_ENERGY
		s.handleAddEnergy(c, uid)
	case 2001:
		log.Printf("[CMD] OK     %s UID=%d SEQ=%d body=%d", cmdname.Format(cmd), userID, seq, len(body))
		s.handleEnterMap(c, body)
	case 2002:
		log.Printf("[CMD] OK     %s UID=%d SEQ=%d body=%d", cmdname.Format(cmd), userID, seq, len(body))
		s.handleLeaveMap(c)
	case 2003:
		log.Printf("[CMD] OK     %s UID=%d SEQ=%d body=%d", cmdname.Format(cmd), userID, seq, len(body))
		s.handleListMapPlayer(c)
	case 2004:
		log.Printf("[CMD] OK     %s UID=%d SEQ=%d body=%d", cmdname.Format(cmd), userID, seq, len(body))
		s.handleMapOgreList(c)
	case 2051: // GET_SIM_USERINFO 个人信息简要
		s.handleGetSimUserInfo(c, uid, body)
	case 2052: // GET_MORE_USERINFO 个人信息详情面板
		s.handleGetMoreUserInfo(c, uid, body)
	case 80008:
		s.send(c, cmd, uid, 0, nil)
	case 8008: // MAIL_NEW_NOTE 推送/心跳
		s.send(c, 8008, uid, 0, nil)
	// 推送向 CMD：客户端偶发回打时防 UNIMPL；真逻辑由业务 s.send 主动推
	case 2000, 2021, 2022, 2501, 2502, 2503, 2504, 2505, 2506, 2507, 2508, 2509, 2510,
		2801, 2901, 2902, 2933, 2934, 2935, 2936, 3451,
		8001, 8002, 8004, 8005, 8006, 8007, 8009, 8010, 70005, 80002:
		s.handleRemainEmptyAck(c, uid, cmd)
	default:
		stage := "已登录"
		if !c.LoggedIn {
			stage = "未登录"
		}
		// 黄字：未实现命令，便于扫日志
		log.Print(conslog.Yellowf("[CMD] UNIMPL %s UID=%d SEQ=%d body=%d stage=%s -> 空回包",
			cmdname.Format(cmd), userID, seq, len(body), stage))
		s.send(c, cmd, uid, 0, nil)
	}
}

func (s *Server) send(c *Client, cmd int32, uid uint32, result int32, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, _ = c.Conn.Write(packet.BuildResponse(cmd, uid, result, body))
}

func (s *Server) handleSystemTime(c *Client, uid uint32) {
	// SystemTimeInfo 只读 1×u32；多 4 字节对齐参考服，无害
	buf := make([]byte, 8)
	binary.BigEndian.PutUint32(buf[0:4], uint32(time.Now().Unix()))
	binary.BigEndian.PutUint32(buf[4:8], 0)
	s.send(c, 1002, uid, 0, buf)
}

func (s *Server) handleLoginIn(c *Client, userID int64, body []byte) {
	user, err := s.cfg.Store.FindByUserID(userID)
	if err != nil || user == nil {
		log.Printf("[game-1001] user missing uid=%d err=%v", userID, err)
		s.send(c, 1001, uint32(userID), 1, nil)
		return
	}
	if len(body) >= 16 && user.SessionHex != "" {
		got := hex.EncodeToString(body[:16])
		if !strings.EqualFold(got, user.SessionHex) {
			log.Printf("[game-1001] session mismatch uid=%d got=%s expect=%s", userID, got, user.SessionHex)
			s.send(c, 1001, uint32(userID), 1, nil)
			return
		}
	} else if len(body) >= 16 {
		// 库中无 session 时，以客户端带来的为准并落库（兼容异常存档）
		user.SessionHex = hex.EncodeToString(body[:16])
		_ = s.cfg.Store.SaveUser(user)
		log.Printf("[game-1001] accept client session uid=%d sess=%s", userID, user.SessionHex)
	}

	s.ForceDisconnect(userID)
	c.UserID = userID
	c.LoggedIn = true
	s.mu.Lock()
	s.byUID[userID] = c
	s.mu.Unlock()

	user.MapID = defaultMapID
	user.PosX = defaultPosX
	user.PosY = defaultPosY
	user.LoginCnt++
	user.LastOnline = time.Now().Unix()
	_ = s.cfg.Store.SaveUser(user)

	c.MapID = user.MapID
	c.PosX = user.PosX
	c.PosY = user.PosY
	c.ClothIDs = s.wornClothIDs(userID)
	s.loadUserProgress(userID)

	bodyOut := s.buildLoginResponse(user)
	s.send(c, 1001, uint32(userID), 0, bodyOut)
	log.Printf("[game-1001] ok uid=%d nick=%s bodyLen=%d map=%d(%d,%d)",
		userID, user.Nickname, len(bodyOut), user.MapID, user.PosX, user.PosY)

	s.pushInitialMapEnter(c, user)
}

// pushInitialMapEnter 1001 后主动推 2001/2003/2004，进传送舱。
func (s *Server) pushInitialMapEnter(c *Client, u *store.User) {
	s.putOnMap(c, u.MapID)
	people := s.buildPeopleInfo(u, c.PosX, c.PosY, c.ClothIDs, c.actionTypeLocked())
	s.send(c, 2001, uint32(u.UserID), 0, people)
	log.Printf("[CMD] OK     %s UID=%d body=%d (推送进图)", cmdname.Format(2001), u.UserID, len(people))
	s.broadcastToMap(c, 2001, people)

	list := s.buildMapPlayerList(c.MapID)
	s.send(c, 2003, uint32(u.UserID), 0, list)
	log.Printf("[CMD] OK     %s UID=%d body=%d (推送同图)", cmdname.Format(2003), u.UserID, len(list))

	s.onEnterMapOgres(c, c.MapID)
	s.pushGaiyaAppearNote(c, uint32(u.UserID), c.MapID)
	s.pushMapBossOnEnter(c, c.MapID)
	log.Printf("[CMD] OK     %s UID=%d (进图空野怪，定时刷新)", cmdname.Format(2004), u.UserID)
}

func (s *Server) handleEnterMap(c *Client, body []byte) {
	if !c.LoggedIn || c.UserID == 0 {
		return
	}
	mapID, x, y := defaultMapID, defaultPosX, defaultPosY
	if len(body) >= 16 {
		// mapType(4) + mapId(4) + x(4) + y(4)
		mapID = int(binary.BigEndian.Uint32(body[4:8]))
		x = int(binary.BigEndian.Uint32(body[8:12]))
		y = int(binary.BigEndian.Uint32(body[12:16]))
	}
	if mapID == 0 {
		mapID = defaultMapID
	}
	if x == 0 && y == 0 {
		x, y = defaultPosX, defaultPosY
	}
	s.enterMapForClient(c, mapID, x, y)
}

// enterMapForClient 主连进图：落库 + 2001/2003/2004（离开基地 LEAVE_ROOM 后也会调）。
func (s *Server) enterMapForClient(c *Client, mapID, x, y int) {
	if c == nil || !c.LoggedIn || c.UserID == 0 {
		return
	}
	if mapID == 0 {
		mapID = defaultMapID
	}
	if x == 0 && y == 0 {
		x, y = defaultPosX, defaultPosY
	}
	// 未先发 LEAVE_MAP 的切图（如离开基地）：先通知旧图删人
	oldMapID := c.MapID
	if oldMapID > 0 && oldMapID != mapID {
		s.notifyMapLeave(oldMapID, c.UserID)
	}
	c.PosX = x
	c.PosY = y

	user, _ := s.cfg.Store.FindByUserID(c.UserID)
	if user == nil {
		user = &store.User{UserID: c.UserID, Nickname: fmt.Sprintf("%d", c.UserID)}
	}
	user.MapID = mapID
	user.PosX = x
	user.PosY = y
	_ = s.cfg.Store.SaveUser(user)

	// putOnMap 内会按当前 c.MapID 从旧表移除再登记新图，故勿提前改 MapID
	s.putOnMap(c, mapID)
	people := s.buildPeopleInfo(user, x, y, c.ClothIDs, c.actionTypeLocked())
	s.send(c, 2001, uint32(c.UserID), 0, people)
	// 同图其他人：2001 PeopleInfo → onEnterMap → addUser
	s.broadcastToMap(c, 2001, people)
	s.send(c, 2003, uint32(c.UserID), 0, s.buildMapPlayerList(mapID))
	s.onEnterMapOgres(c, mapID)
	s.pushGaiyaAppearNote(c, uint32(c.UserID), mapID)
	s.pushMapBossOnEnter(c, mapID)
	log.Printf("[game-2001] uid=%d map=%d (%d,%d) people=%d (ogre delayed)", c.UserID, mapID, x, y, len(people))
}

func (s *Server) mapOgreBody(c *Client) []byte {
	if c == nil {
		return emptyMapOgreList()
	}
	slots := s.getOgreSlots(c.UserID, c.MapID)
	return buildMapOgreList(slots)
}

func (s *Server) handleLeaveMap(c *Client) {
	if !c.LoggedIn || c.UserID == 0 {
		return
	}
	var b bytes.Buffer
	packet.WriteU32(&b, uint32(c.UserID))
	body := b.Bytes()
	s.send(c, 2002, uint32(c.UserID), 0, body)
	// 必须先广播再从表移除，否则同图其他人看不到离图（幽灵人）
	s.broadcastToMap(c, 2002, body)
	s.mu.Lock()
	s.removeFromMapLocked(c)
	s.mu.Unlock()
}

func (s *Server) handleListMapPlayer(c *Client) {
	if !c.LoggedIn {
		return
	}
	body := s.buildMapPlayerList(c.MapID)
	s.send(c, 2003, uint32(c.UserID), 0, body)
}

func (s *Server) handleMapOgreList(c *Client) {
	if !c.LoggedIn {
		return
	}
	body := s.mapOgreBody(c)
	s.send(c, 2004, uint32(c.UserID), 0, body)
	log.Printf("[CMD] OK     2004 MAP_OGRE_LIST UID=%d map=%d body=%d", c.UserID, c.MapID, len(body))
}

func (s *Server) putOnMap(c *Client, mapID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeFromMapLocked(c)
	c.MapID = mapID
	m := s.mapUsers[mapID]
	if m == nil {
		m = make(map[int64]*Client)
		s.mapUsers[mapID] = m
	}
	m[c.UserID] = c
}

func (s *Server) removeFromMapLocked(c *Client) {
	if c == nil || c.MapID == 0 {
		return
	}
	if m := s.mapUsers[c.MapID]; m != nil {
		delete(m, c.UserID)
		if len(m) == 0 {
			delete(s.mapUsers, c.MapID)
		}
	}
}

func (s *Server) buildMapPlayerList(mapID int) []byte {
	s.mu.Lock()
	clients := make([]*Client, 0)
	if m := s.mapUsers[mapID]; m != nil {
		for _, c := range m {
			clients = append(clients, c)
		}
	}
	s.mu.Unlock()

	var buf bytes.Buffer
	packet.WriteU32(&buf, uint32(len(clients)))
	for _, c := range clients {
		u, err := s.cfg.Store.FindByUserID(c.UserID)
		if err != nil || u == nil {
			u = &store.User{UserID: c.UserID, Nickname: fmt.Sprintf("%d", c.UserID)}
		}
		clothIDs := c.ClothIDs
		if len(clothIDs) == 0 {
			clothIDs = s.wornClothIDs(c.UserID)
		}
		buf.Write(s.buildPeopleInfo(u, c.PosX, c.PosY, clothIDs, c.actionTypeLocked()))
	}
	return buf.Bytes()
}

func (c *Client) actionTypeLocked() uint32 {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	v := c.ActionType
	c.mu.Unlock()
	return v
}

// buildPeopleInfo 对齐 nieocore UserInfo.setForPeoleInfo（无 petShiny / fireBuff）。
func (s *Server) buildPeopleInfo(u *store.User, posX, posY int, clothIDs []uint32, actionType uint32) []byte {
	var buf bytes.Buffer
	w32 := func(v uint32) { packet.WriteU32(&buf, v) }
	w16 := func(v uint16) { packet.WriteU16(&buf, v) }
	wStr := func(s string, n int) { packet.WriteFixedString(&buf, s, n) }

	nick := u.Nickname
	if nick == "" {
		nick = fmt.Sprintf("%d", u.UserID)
	}
	if posX == 0 && posY == 0 {
		posX, posY = defaultPosX, defaultPosY
	}

	w32(uint32(time.Now().Unix())) // sysTime
	w32(uint32(u.UserID))
	wStr(nick, 16)
	w32(uint32(u.Color))
	w32(0) // texture
	w32(0) // vip flags
	w32(1) // vipStage
	w32(actionType)
	w32(uint32(posX))
	w32(uint32(posY))
	w32(0) // action
	w32(2) // direction DOWN
	w32(0) // changeShape
	w32(0) // spiritTime
	w32(0) // spiritID
	w32(0) // petDV
	w32(0) // petSkin
	w32(0) // fightFlag
	teacherID, studentID, _ := s.teacherIDsForUser(u.UserID)
	w32(teacherID)
	w32(studentID)
	nonoState, nonoColor, superNono := uint32(0), uint32(0), uint32(0)
	if n := s.loadNonoForLogin(u.UserID); n != nil && n.HasNono != 0 {
		nonoState = uint32(n.State)
		nonoColor = uint32(n.Color)
		// 飞行全面开启：有 NoNo 即视为超能位，客户端飞行面板才能点选
		superNono = 1
	}
	w32(nonoState)
	w32(nonoColor)
	w32(superNono)
	w32(0) // playerForm
	w32(0) // transTime
	ts := s.userTeamSnapshot(u.UserID)
	w32(ts.ID)
	w32(ts.CoreCount)
	w32(ts.IsShow)
	w16(ts.LogoBg)
	w16(ts.LogoIcon)
	w16(ts.LogoColor)
	w16(ts.TxtColor)
	wStr(ts.LogoWord, 4)
	raw := buf.Bytes()
	appendClothesBlock(&raw, clothIDs)
	raw = append(raw, 0, 0, 0, 0) // curTitle
	return raw
}

// buildLoginResponse 严格按本仓库反编译 UserInfo.setForLoginInfo。
func (s *Server) buildLoginResponse(u *store.User) []byte {
	var buf bytes.Buffer
	w32 := func(v uint32) { packet.WriteU32(&buf, v) }
	w8 := func(v byte) { buf.WriteByte(v) }
	wStr := func(s string, n int) { packet.WriteFixedString(&buf, s, n) }

	reg := u.RegisterTime
	if reg == 0 {
		reg = time.Now().Unix() - 86400
	}
	nick := u.Nickname
	if nick == "" {
		nick = fmt.Sprintf("%d", u.UserID)
	}
	energy := u.Energy
	if energy == 0 {
		energy = 100
	}
	mapID := u.MapID
	if mapID == 0 {
		mapID = defaultMapID
	}
	posX, posY := u.PosX, u.PosY
	if posX == 0 {
		posX = defaultPosX
	}
	if posY == 0 {
		posY = defaultPosY
	}

	nono := s.loadNonoForLogin(u.UserID)
	vipFlags := uint32(0)
	vipLevel, vipValue, vipStage := uint32(0), uint32(0), uint32(1)
	autoCharge, vipEnd, freshBonus := uint32(0), uint32(0), uint32(0)
	hasNono, superNono, nonoState, nonoColor := uint32(0), uint32(0), uint32(0), uint32(0)
	nonoNick := ""
	chips := make([]byte, 80)
	if nono != nil && nono.HasNono != 0 {
		hasNono = 1
		nonoState = uint32(nono.State)
		nonoColor = uint32(nono.Color)
		nonoNick = nono.Nick
		// 飞行全面开启：有 NoNo 即下发超能位，客户端飞行面板/快捷栏可进
		superNono = 1
		vipStage = uint32(nono.SuperStage)
		if vipStage < 5 {
			vipStage = 5 // 形态 1–4 均可选
		}
		if vipStage > 5 {
			vipStage = 5
		}
		for i := range chips {
			chips[i] = 1
		}
		if nono.SuperNono > 0 {
			vipFlags = 1 // bit0=vip
			vipLevel = uint32(nono.SuperLevel)
			if vipLevel == 0 {
				vipLevel = 1
			}
			vipValue = uint32(nono.VipValue)
			if nono.SuperEnergy > 0 {
				vipValue = uint32(nono.SuperEnergy)
			}
			autoCharge = uint32(nono.AutoCharge)
			if nono.VipEndTime > 0 && nono.VipEndTime <= 0x7fffffff {
				vipEnd = uint32(nono.VipEndTime)
			} else {
				vipEnd = 0x7fffffff
			}
		}
	}

	w32(uint32(u.UserID))
	w32(uint32(reg))
	wStr(nick, 16)
	w32(vipFlags)
	w32(0) // dsFlag
	w32(uint32(u.Color))
	w32(0) // texture
	w32(uint32(energy))
	w32(uint32(u.Coins))
	w32(0) // fightBadge
	w32(uint32(mapID))
	w32(uint32(posX))
	w32(uint32(posY))
	w32(0)
	w32(86400)
	w8(0)
	w8(0)
	w8(0)
	w8(0)
	w32(uint32(u.LoginCnt))
	w32(0) // inviter
	w32(0) // newInviteeCnt
	w32(vipLevel)
	w32(vipValue)
	w32(vipStage)
	w32(autoCharge)
	w32(vipEnd)
	w32(freshBonus)
	buf.Write(chips)
	dailyRes := make([]byte, 50)
	s.fillPuniDailyRes(u.UserID, dailyRes)
	buf.Write(dailyRes)
	teacherID, studentID, graduation := s.teacherIDsForUser(u.UserID)
	w32(teacherID)
	w32(studentID)
	w32(graduation)
	w32(s.maxPuniLvOf(u.UserID))
	petMaxLev, petAllNum := 0, 0
	if s.cfg.Store != nil {
		if bag, err := s.cfg.Store.ListBagPets(u.UserID); err == nil {
			petAllNum += len(bag)
			for _, p := range bag {
				if p.Level > petMaxLev {
					petMaxLev = p.Level
				}
			}
		}
		if st, err := s.cfg.Store.ListStoragePets(u.UserID); err == nil {
			petAllNum += len(st)
			for _, p := range st {
				if p.Level > petMaxLev {
					petMaxLev = p.Level
				}
			}
		}
	}
	achievePts, _ := s.achieveCurrentTotals(u.UserID)
	w32(uint32(petMaxLev))
	w32(uint32(petAllNum))
	w32(uint32(achievePts)) // totalAchieve
	w32(0)                  // monKingWin
	braveCur, braveMax, freshCur, freshMax := s.loginProgress(u.UserID)
	// 客户端 curStage = 包内值 + 1
	braveStagePkt := braveCur
	if braveStagePkt > 0 {
		braveStagePkt--
	}
	w32(braveStagePkt)
	w32(braveMax)
	w32(freshCur)
	w32(freshMax)
	w32(0) // maxArenaWins
	bt := s.boostTimesOf(u.UserID)
	w32(uint32(max0(bt.TwoTimes)))
	w32(uint32(max0(bt.ThreeTimes)))
	w32(uint32(max0(bt.AutoFight)))
	w32(uint32(max0(bt.AutoFightTimes)))
	w32(uint32(max0(bt.EnergyTimes)))
	w32(uint32(max0(bt.LearnTimes)))
	w32(0) // monBtlMedal
	w32(0) // recordCnt
	w32(0) // obtainTm
	w32(0) // soulBeadItemID
	w32(0) // expireTm
	w32(0) // fuseTimes
	w32(hasNono)
	w32(superNono)
	w32(nonoState)
	w32(nonoColor)
	wStr(nonoNick, 16)
	ts := s.userTeamSnapshot(u.UserID)
	w32(ts.ID)
	w32(ts.Priv)
	w32(ts.SuperCore)
	w32(ts.IsShow)
	w32(ts.AllContrib)
	w32(ts.CanExContrib)
	w32(0) // TeamPK group
	w32(0) // TeamPK home
	w8(0)
	w32(0) // badge
	buf.Write(make([]byte, 27))

	// 任务状态表：对齐 TasksManager.taskList；88 完成后不再进 515 重跑新手
	taskList := make([]byte, loginTaskListSize)
	if s.cfg.Store != nil {
		if encoded, err := s.cfg.Store.EncodeLoginTaskList(u.UserID, loginTaskListSize); err == nil {
			taskList = encoded
		} else {
			log.Printf("[game-1001] EncodeLoginTaskList uid=%d err=%v", u.UserID, err)
		}
	}
	buf.Write(taskList)

	// 背包精灵：petNum + PetInfo*n（PetManager.initData）；超限自动挪仓
	var bag []store.Pet
	if s.cfg.Store != nil {
		if n, err := s.cfg.Store.NormalizeBagOverflow(u.UserID); err == nil && n > 0 {
			log.Printf("[game-1001] bag overflow uid=%d moved=%d -> storage", u.UserID, n)
		}
		bag, _ = s.cfg.Store.ListBagPets(u.UserID)
	}
	w32(uint32(len(bag)))
	for i := range bag {
		if !debugFightNoSkills && s.fillPetSkillsUpToFour(&bag[i]) {
			_ = s.cfg.Store.UpsertPet(&bag[i])
		}
		petBody := buildPetInfo(&bag[i])
		buf.Write(petBody)
		ct := uint32(0)
		if len(petBody) >= 136 {
			ct = binary.BigEndian.Uint32(petBody[132:136])
		}
		log.Printf("[game-1001] bag[%d] petID=%d catch=%d bodyCatch=%d len=%d skills=%v",
			i, bag[i].PetID, bag[i].CatchTime, ct, len(petBody), bag[i].Skills)
	}

	clothIDs := s.wornClothIDs(u.UserID)
	appendClothesBlockFromBuf(&buf, clothIDs)
	w32(0) // title
	buf.Write(s.buildBossAchievementBytes(u.UserID))
	return buf.Bytes()
}
