package core

import (
	"errors"
	"strings"
	"sync"
	"time"

	ps "github.com/mitchellh/go-ps"

	"github.com/Wariie/go-woxy/com"
)

//Supervisor -
type Supervisor struct {
	listModule []string
	mux        sync.Mutex
}

//Remove -
func (s *Supervisor) Remove(m string) {
	for i := range s.listModule {
		if m == s.listModule[i] {
			s.listModule[i] = s.listModule[len(s.listModule)-1] // Copy last element to index i.
			s.listModule[len(s.listModule)-1] = ""              // Erase last element (write zero value).
			s.listModule = s.listModule[:len(s.listModule)-1]   // Truncate slice.
			break
		}
	}
}

//Add -
func (s *Supervisor) Add(m string) {
	s.mux.Lock()
	s.listModule = append(s.listModule, m)
	s.mux.Unlock()
}

//Supervise -
func (s *Supervisor) Supervise() {
	//ENDLESS LOOP
	for {
		//FOR EACH REGISTERED MODULE
		s.mux.Lock()

		for k := range s.listModule {
			//CHECK MODULE RUNNING
			m := GetManager().GetConfig().MODULES[s.listModule[k]]
			if checkModuleRunning(m) {
				if m.STATE != Online && m.STATE != Loading && m.STATE != Downloaded {
					m.STATE = Online
				}
				//ELSE SET STATE TO UNKNOWN
			} else if m.STATE != Loading && m.STATE != Downloaded {
				m.STATE = Unknown
				s.Remove(m.NAME)
			}
			GetManager().SaveModuleChanges(&m)
		}
		s.mux.Unlock()
		time.Sleep(time.Millisecond * 10)
	}
}

func checkModuleRunning(mc ModuleConfig) bool {
	try := 0
	b := false

	for b == false && try < 5 {
		if mc.pid != 0 && (mc.EXE != ModuleExecConfig{}) {
			b = checkPidRunning(&mc)
		}

		if !b {
			b = checkModulePing(&mc)
		}
		try++
	}
	return b
}

func checkModulePing(mc *ModuleConfig) bool {
	var cr com.CommandRequest
	cr.Generate("Ping", mc.PK, mc.NAME, GetManager().GetConfig().SECRET)
	resp, err := com.SendRequest(mc.GetServer("/cmd"), &cr, false)
	if err != nil {
		return false
	} else if strings.Contains(resp, mc.NAME+" ALIVE") {
		return true
	}
	return false
}

func findProcess(pid int) (int, string, error) {
	pname := ""
	err := errors.New("not found")
	ps, _ := ps.Processes()

	for i := range ps {
		if ps[i].Pid() == pid {
			pname = ps[i].Executable()
			err = nil
			break
		}
	}
	return pid, pname, err
}

func checkPidRunning(mc *ModuleConfig) bool {
	p, n, e := findProcess(mc.pid)
	if p != 0 && n != "" && e == nil {
		return true
	}
	return false
}
