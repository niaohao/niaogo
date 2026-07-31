package tableloader

import (
	"encoding/xml"
	"math/rand"
	"os"
	"strconv"
	"strings"
)

// BreedEggDef EggsXMLInfo 一条配方。
type BreedEggDef struct {
	ID         int
	MaleMon    int
	FemaleMon  int
	OutputMons []int
	Probs      []int
}

func (c *Catalog) loadEggs(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	type eggNode struct {
		ID         string `xml:"Id,attr"`
		MaleMon    string `xml:"MaleMon,attr"`
		FemaleMon  string `xml:"FemaleMon,attr"`
		OutputMons string `xml:"OutputMons,attr"`
		Probs      string `xml:"Probs,attr"`
	}
	var root struct {
		Eggs []eggNode `xml:"Egg"`
	}
	if err := xml.Unmarshal(b, &root); err != nil {
		return err
	}
	if c.BreedEggs == nil {
		c.BreedEggs = make(map[int]BreedEggDef)
	}
	if c.BreedPairEgg == nil {
		c.BreedPairEgg = make(map[uint64]int)
	}
	atoi := func(s string) int {
		n, _ := strconv.Atoi(strings.TrimSpace(s))
		return n
	}
	parseInts := func(s string) []int {
		parts := strings.Fields(s)
		out := make([]int, 0, len(parts))
		for _, p := range parts {
			if n := atoi(p); n > 0 {
				out = append(out, n)
			}
		}
		return out
	}
	for _, e := range root.Eggs {
		id := atoi(e.ID)
		male := atoi(e.MaleMon)
		female := atoi(e.FemaleMon)
		if id <= 0 || male <= 0 || female <= 0 {
			continue
		}
		outs := parseInts(e.OutputMons)
		probs := parseInts(e.Probs)
		if len(outs) == 0 {
			continue
		}
		for len(probs) < len(outs) {
			probs = append(probs, 0)
		}
		if len(probs) > len(outs) {
			probs = probs[:len(outs)]
		}
		c.BreedEggs[id] = BreedEggDef{
			ID: id, MaleMon: male, FemaleMon: female,
			OutputMons: outs, Probs: probs,
		}
		c.BreedPairEgg[breedPairKey(male, female)] = id
	}
	return nil
}

func breedPairKey(male, female int) uint64 {
	return uint64(male)<<32 | uint64(uint32(female))
}

// BreedEggID 查雄雌配方蛋 ID；无则 0。
func (c *Catalog) BreedEggID(maleID, femaleID int) int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.BreedPairEgg == nil {
		return 0
	}
	return c.BreedPairEgg[breedPairKey(maleID, femaleID)]
}

// EggOutputPetID 按概率抽孵化产物精灵 ID。
func (c *Catalog) EggOutputPetID(eggID int) int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	def, ok := c.BreedEggs[eggID]
	c.mu.RUnlock()
	if !ok || len(def.OutputMons) == 0 {
		return 0
	}
	total := 0
	for _, p := range def.Probs {
		total += p
	}
	if total <= 0 {
		return def.OutputMons[0]
	}
	r := rand.Intn(total)
	acc := 0
	for i, p := range def.Probs {
		acc += p
		if r < acc {
			return def.OutputMons[i]
		}
	}
	return def.OutputMons[0]
}

// PetGender 种族性别：1 雄 2 雌 0 未知/无性别。
func (c *Catalog) PetGender(petID int) int {
	d := c.PetBase(petID)
	if d == nil {
		return 0
	}
	return d.Gender
}
