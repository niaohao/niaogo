package gameserver

import (
	"encoding/binary"
	"log"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/store"
)

const (
	taskStatusAccepted = 1
	taskStatusComplete = 3

	taskSelectPet = 86
	taskWinBattle = 87
	taskUseItem   = 88
	taskParent4   = 4 // 训练营每日任务 parent；完成 88 后需同步完成
)

func (s *Server) setTaskStatus(uid int64, taskID, status int) {
	if s.cfg.Store == nil || taskID <= 0 {
		return
	}
	if err := s.cfg.Store.UpsertTaskStatus(uid, taskID, status); err != nil {
		log.Printf("[task] UpsertTaskStatus uid=%d task=%d status=%d err=%v", uid, taskID, status, err)
	}
}

func (s *Server) handleAcceptTask(c *Client, uid uint32, body []byte) {
	// 2201: 回显 taskID；落库 status=1，供下次登录 taskList 恢复
	taskID := uint32(0)
	if len(body) >= 4 {
		taskID = binary.BigEndian.Uint32(body[0:4])
	}
	s.setTaskStatus(int64(uid), int(taskID), taskStatusAccepted)
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, taskID)
	s.send(c, 2201, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d taskID=%d status=1", cmdname.Format(2201), uid, taskID)
}

func (s *Server) handleGetTaskBuf(c *Client, uid uint32, body []byte) {
	// 2203 GET_TASK_BUF → TaskBufInfo: taskId(4)+flag(4)+buf(20)
	taskID := uint32(0)
	if len(body) >= 4 {
		taskID = binary.BigEndian.Uint32(body[0:4])
	}
	flag := uint32(0)
	bufBytes := make([]byte, 20)
	if s.cfg.Store != nil {
		if t, _ := s.cfg.Store.GetTask(int64(uid), int(taskID)); t != nil {
			if t.Status >= taskStatusAccepted {
				flag = 1
			}
			copy(bufBytes, t.Buf)
		}
	}
	out := make([]byte, 28)
	binary.BigEndian.PutUint32(out[0:4], taskID)
	binary.BigEndian.PutUint32(out[4:8], flag)
	copy(out[8:28], bufBytes)
	s.send(c, 2203, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d taskID=%d flag=%d", cmdname.Format(2203), uid, taskID, flag)
}

func (s *Server) handleAddTaskBuf(c *Client, uid uint32, body []byte) {
	// 2204 ADD_TASK_BUF：请求 taskId(4)+buf(20)；应答 4 字节占位
	taskID := uint32(0)
	if len(body) >= 4 {
		taskID = binary.BigEndian.Uint32(body[0:4])
	}
	if s.cfg.Store != nil && taskID > 0 {
		var buf []byte
		if len(body) >= 24 {
			buf = body[4:24]
		} else if len(body) > 4 {
			buf = body[4:]
		}
		if err := s.cfg.Store.UpsertTaskBuf(int64(uid), int(taskID), buf); err != nil {
			log.Printf("[CMD] WARN  %s UID=%d UpsertTaskBuf: %v", cmdname.Format(2204), uid, err)
		}
	}
	out := make([]byte, 4)
	s.send(c, 2204, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d taskID=%d body=%d", cmdname.Format(2204), uid, taskID, len(body))
}

func (s *Server) handleCompleteTask(c *Client, uid uint32, cmd int32, body []byte) {
	// 2202/2233 → NoviceFinishInfo；落库 status=3，避免每次登录重跑新手
	taskID := uint32(0)
	param := uint32(0)
	if len(body) >= 4 {
		taskID = binary.BigEndian.Uint32(body[0:4])
	}
	if len(body) >= 8 {
		param = binary.BigEndian.Uint32(body[4:8])
	}
	isDaily := cmd == 2233
	alreadyDone := s.taskAlreadyComplete(int64(uid), int(taskID))

	petID := uint32(0)
	captureTm := uint32(0)
	itemPairs := make([][2]uint32, 0)

	if !alreadyDone {
		switch taskID {
		case taskSelectPet:
			if s.cfg.Store != nil {
				p := newStarterPet(int64(uid), resolveNovicePetID(param))
				if existing, _ := s.cfg.Store.GetPetByCatchTime(int64(uid), p.CatchTime); existing != nil {
					p = existing
				} else {
					n, _ := s.cfg.Store.CountBagPets(int64(uid))
					if n >= store.MaxBagPets {
						p.InBag = false
						p.BagPos = -1
					}
					if err := s.cfg.Store.UpsertPet(p); err != nil {
						log.Printf("[CMD] WARN  %s UID=%d task86 UpsertPet: %v", cmdname.Format(cmd), uid, err)
					}
				}
				petID = uint32(p.PetID)
				captureTm = uint32(p.CatchTime)
			}
		case taskWinBattle:
			// TaskClass_87 客户端写死弹窗；服务端落库胶囊/药剂供 2605 查询
			s.grantNoviceBattleItems(int64(uid))
			itemPairs = append(itemPairs, [2]uint32{300001, 5}, [2]uint32{300011, 5})
		case taskUseItem:
			// TaskClass_88 客户端 +5000 豆；服务端同步 coins，并完成 parent 任务 4
			if s.cfg.Store != nil {
				if err := s.cfg.Store.AddCoins(int64(uid), 5000); err != nil {
					log.Printf("[CMD] WARN  %s UID=%d AddCoins: %v", cmdname.Format(cmd), uid, err)
				}
			}
			s.setTaskStatus(int64(uid), taskParent4, taskStatusComplete)
		default:
			itemPairs = resolveTaskRewards(taskID, isDaily)
			s.applyTaskItemRewards(int64(uid), itemPairs)
			s.grantTrainCampSetBonus(int64(uid), int(taskID))
		}
	} else {
		// 已领过：仍回同样 monBallList，避免 TaskClass_401 等读 [0] 崩；不再入账
		switch taskID {
		case taskSelectPet:
			if s.cfg.Store != nil {
				pid := resolveNovicePetID(param)
				if p, _ := s.cfg.Store.GetPetByCatchTime(int64(uid), task86CatchTm(pid)); p != nil {
					petID = uint32(p.PetID)
					captureTm = uint32(p.CatchTime)
				} else if bag, _ := s.cfg.Store.ListBagPets(int64(uid)); len(bag) > 0 {
					petID = uint32(bag[0].PetID)
					captureTm = uint32(bag[0].CatchTime)
				}
			}
		case taskWinBattle:
			itemPairs = append(itemPairs, [2]uint32{300001, 5}, [2]uint32{300011, 5})
		default:
			itemPairs = resolveTaskRewards(taskID, isDaily)
		}
	}

	s.setTaskStatus(int64(uid), int(taskID), taskStatusComplete)

	// 雷伊特训 121/122：多步任务末步前保持 accepted，并写入技能银行
	if taskID == 121 || taskID == 122 {
		if s.onLeiyiTrainTaskComplete(int64(uid), taskID, param) {
			// 122 非末步：已在 onLeiyiTrainTaskComplete 写回 status=1
		}
	}

	out := make([]byte, 16+len(itemPairs)*8)
	binary.BigEndian.PutUint32(out[0:4], taskID)
	binary.BigEndian.PutUint32(out[4:8], petID)
	binary.BigEndian.PutUint32(out[8:12], captureTm)
	binary.BigEndian.PutUint32(out[12:16], uint32(len(itemPairs)))
	off := 16
	for _, p := range itemPairs {
		binary.BigEndian.PutUint32(out[off:off+4], p[0])
		binary.BigEndian.PutUint32(out[off+4:off+8], p[1])
		off += 8
	}
	s.send(c, cmd, uid, 0, out)
	if taskID == taskSelectPet {
		log.Printf("[CMD] OK     %s UID=%d taskID=86 petID=%d catchTm=%d param=%d status=3 done=%v",
			cmdname.Format(cmd), uid, petID, captureTm, param, alreadyDone)
	} else {
		log.Printf("[CMD] OK     %s UID=%d taskID=%d items=%d daily=%v done=%v status=3",
			cmdname.Format(cmd), uid, taskID, len(itemPairs), isDaily, alreadyDone)
	}
}

func (s *Server) grantNoviceBattleItems(uid int64) {
	if s.cfg.Store == nil {
		return
	}
	// 对齐 TaskClass_87 弹窗：初级精灵胶囊×5、初级体力药剂×5
	for _, pair := range [][2]int{{300001, 5}, {300011, 5}} {
		if err := s.cfg.Store.AddItem(uid, pair[0], pair[1]); err != nil {
			log.Printf("[task87] AddItem uid=%d item=%d err=%v", uid, pair[0], err)
		}
	}
}
