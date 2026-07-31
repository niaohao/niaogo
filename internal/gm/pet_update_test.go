package gm

import (
	"encoding/json"
	"testing"
)

func TestParsePetEV_Array(t *testing.T) {
	req := petUpdateReq{EV: json.RawMessage(`[255,0,0,255,0,0]`)}
	ev, ok, err := parsePetEV(req, [6]int{})
	if err != "" || !ok {
		t.Fatalf("err=%q ok=%v", err, ok)
	}
	if ev != [6]int{255, 0, 0, 255, 0, 0} {
		t.Fatalf("got %v", ev)
	}
}

func TestParsePetEV_Object(t *testing.T) {
	req := petUpdateReq{EV: json.RawMessage(`{"hp":10,"atk":20,"def":30,"sa":40,"sd":50,"sp":60}`)}
	ev, ok, err := parsePetEV(req, [6]int{})
	if err != "" || !ok {
		t.Fatalf("err=%q ok=%v", err, ok)
	}
	if ev != [6]int{10, 20, 30, 40, 50, 60} {
		t.Fatalf("got %v", ev)
	}
}

func TestParsePetEV_FlatAndCap(t *testing.T) {
	hp, atk := 255, 256
	req := petUpdateReq{EvHP: &hp, EvAtk: &atk}
	_, _, err := parsePetEV(req, [6]int{})
	if err == "" {
		t.Fatal("expect over-255 error")
	}
	atk = 255
	sp := 1
	req = petUpdateReq{EvHP: &hp, EvAtk: &atk, EvSP: &sp}
	_, _, err = parsePetEV(req, [6]int{})
	if err == "" {
		t.Fatal("expect sum>510 error")
	}
}

func TestParsePetEV_None(t *testing.T) {
	_, ok, err := parsePetEV(petUpdateReq{}, [6]int{1, 2, 3, 4, 5, 6})
	if ok || err != "" {
		t.Fatalf("ok=%v err=%q", ok, err)
	}
}
