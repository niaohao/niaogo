package gameserver

import (
	"encoding/json"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
)

type fusionFormula struct {
	MainID     int `json:"MainID"`
	SubID      int `json:"SubID"`
	BeadItemID int `json:"BeadItemID"`
	ResultID   int `json:"ResultID"`
	Prob       int `json:"Prob"`
}

const wildBeigeBeadID = 1000007

// 米色珠默认池（items.xml TransmuteMon）；catalog 有配置时优先生效。
var wildBeigePets = []int{325, 328, 330, 528, 602, 730}

// 内嵌兜底：与 tables/json/fusion_formulas.json 同步的经典配方（含副宠多形态与扩展珠）。
var defaultFusionFormulas = []fusionFormula{
	{3, 27, 1000001, 301, 100}, {3, 28, 1000001, 301, 100}, {3, 29, 1000001, 301, 100},
	{6, 198, 1000002, 304, 100}, {6, 199, 1000002, 304, 100}, {6, 200, 1000002, 304, 100},
	{9, 35, 1000003, 307, 100}, {9, 36, 1000003, 307, 100}, {9, 37, 1000003, 307, 100},
	{210, 38, 1000003, 332, 100}, {210, 39, 1000003, 332, 100}, {210, 40, 1000003, 332, 100},
	{180, 53, 1000004, 316, 100}, {180, 54, 1000004, 316, 100}, {180, 55, 1000004, 316, 100},
	{118, 190, 1000005, 322, 100}, {118, 191, 1000005, 322, 100}, {118, 192, 1000005, 322, 100},
	{155, 25, 1000006, 319, 100}, {155, 26, 1000006, 319, 100},
	{79, 164, 1000008, 310, 95}, {79, 165, 1000008, 310, 95}, {79, 166, 1000008, 310, 95},
	{79, 164, 1000009, 313, 5}, {79, 165, 1000009, 313, 5}, {79, 166, 1000009, 313, 5},
	{61, 102, 1000010, 338, 100}, {61, 103, 1000010, 338, 100}, {61, 104, 1000010, 338, 100},
	{138, 128, 1000011, 341, 100}, {138, 129, 1000011, 341, 100}, {138, 130, 1000011, 341, 100},
	{15, 97, 1000012, 401, 100}, {15, 98, 1000012, 401, 100}, {15, 99, 1000012, 401, 100},
	{189, 228, 1000013, 404, 100}, {189, 229, 1000013, 404, 100},
	{251, 232, 1000014, 427, 100}, {251, 233, 1000014, 427, 100}, {251, 234, 1000014, 427, 100},
}

var (
	fusionFormulasMu sync.RWMutex
	fusionFormulas   = append([]fusionFormula(nil), defaultFusionFormulas...)
)

func loadFusionFormulasJSON(path string) {
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		return
	}
	var wrap struct {
		Formulas []fusionFormula `json:"formulas"`
	}
	if err := json.Unmarshal(b, &wrap); err != nil || len(wrap.Formulas) == 0 {
		return
	}
	fusionFormulasMu.Lock()
	fusionFormulas = wrap.Formulas
	fusionFormulasMu.Unlock()
	log.Printf("[fusion] loaded %d formulas from %s", len(wrap.Formulas), path)
}

func tryLoadFusionFormulas(xmlDir string) {
	cands := []string{
		filepath.Join(xmlDir, "..", "json", "fusion_formulas.json"),
		filepath.Join(xmlDir, "fusion_formulas.json"),
		filepath.Join("tables", "json", "fusion_formulas.json"),
	}
	for _, p := range cands {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			loadFusionFormulasJSON(p)
			return
		}
	}
}

func matchFusionFormula(mainID, subID int) (beadItemID, resultPetID int, ok bool) {
	fusionFormulasMu.RLock()
	defer fusionFormulasMu.RUnlock()
	cands := make([]fusionFormula, 0, 4)
	for _, f := range fusionFormulas {
		if (f.MainID == mainID && f.SubID == subID) || (f.MainID == subID && f.SubID == mainID) {
			cands = append(cands, f)
		}
	}
	if len(cands) == 0 {
		return 0, 0, false
	}
	total := 0
	for _, c := range cands {
		if c.Prob > 0 {
			total += c.Prob
		}
	}
	if total <= 0 {
		c := cands[rand.Intn(len(cands))]
		return c.BeadItemID, c.ResultID, true
	}
	roll := rand.Intn(total) + 1
	acc := 0
	for _, c := range cands {
		if c.Prob <= 0 {
			continue
		}
		acc += c.Prob
		if roll <= acc {
			return c.BeadItemID, c.ResultID, true
		}
	}
	last := cands[len(cands)-1]
	return last.BeadItemID, last.ResultID, true
}

func calcFusionDV(dvMain, dvSub int) int {
	// 尼尔号：宝石数 = ⌊(主+副)/2⌋，再按高位填宝石还原 DV（5→31）。
	g := (gemCountOfDV(dvMain) + gemCountOfDV(dvSub)) / 2
	return dvFromGemCount(g)
}

func gemCountOfDV(dv int) int {
	if dv < 0 {
		dv = 0
	}
	if dv > 31 {
		dv = 31
	}
	n := 0
	for i := 0; i < 5; i++ {
		if dv&(1<<i) != 0 {
			n++
		}
	}
	return n
}

func dvFromGemCount(n int) int {
	if n <= 0 {
		return 0
	}
	if n >= 5 {
		return 31
	}
	bits := []int{16, 8, 4, 2, 1}
	v := 0
	for i := 0; i < n; i++ {
		v |= bits[i]
	}
	return v
}

// chooseFusionNature：性格相同且非 0 则继承，否则随机；孤独(常见 ID=0 已排除) 不同则随机。
func chooseFusionNature(natureMain, natureSub int) int {
	if natureMain != 0 && natureMain == natureSub {
		return natureMain
	}
	return rand.Intn(25)
}

func fusionFormulaCount() int {
	fusionFormulasMu.RLock()
	defer fusionFormulasMu.RUnlock()
	return len(fusionFormulas)
}
