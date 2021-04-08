package main

var Transitions = map[string]map[string]State{}

func initSM() {
	Transitions["login"] = map[string]State{
		"ok": &StateLogin{},
	}
}