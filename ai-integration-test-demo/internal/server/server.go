package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"

	"github.com/example/ai-integration-test-demo/internal/breakpoint"
	"github.com/example/ai-integration-test-demo/internal/equipment"
	"github.com/example/ai-integration-test-demo/internal/event"
	"github.com/example/ai-integration-test-demo/internal/player"
	"github.com/gorilla/websocket"
)

type Request struct {
	Cmd      string `json:"cmd"`
	PlayerID int    `json:"playerId,omitempty"`
	Sub      string `json:"sub,omitempty"`
	ItemID   int    `json:"itemId,omitempty"`
	Count    int    `json:"count,omitempty"`
	TaskID   int    `json:"taskId,omitempty"`
	AchID    int    `json:"achId,omitempty"`
	Day      int    `json:"day,omitempty"`
	MailID   int    `json:"mailId,omitempty"`
	Slot     string `json:"slot,omitempty"`
	Name     string `json:"name,omitempty"`
	Target   string `json:"target,omitempty"`
	Action   string `json:"action,omitempty"`
	Desc     string `json:"description,omitempty"`
}

type Response struct {
	Ok   bool     `json:"ok"`
	Data any      `json:"data,omitempty"`
	Log  []string `json:"log,omitempty"`
	Err  string   `json:"err,omitempty"`
}

type Server struct {
	pm       *player.Manager
	bus      *event.Bus
	bp       *breakpoint.Controller
	profile  string
	upgrader websocket.Upgrader
}

type RegisteredCommand struct {
	Name        string `json:"name"`
	Target      string `json:"target"`
	Action      string `json:"action"`
	Description string `json:"description,omitempty"`
}

type sessionState struct {
	customCmds map[string]RegisteredCommand
}

func New(pm *player.Manager, bus *event.Bus, bp *breakpoint.Controller, profile string) *Server {
	return &Server{
		pm:      pm,
		bus:     bus,
		bp:      bp,
		profile: profile,
		upgrader: websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
			return allowLocalOrigin(r)
		}},
	}
}

func allowLocalOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	switch u.Hostname() {
	case "127.0.0.1", "localhost":
		return true
	default:
		return false
	}
}

func allowLocalRemote(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) HandleWS(w http.ResponseWriter, r *http.Request) {
	if !allowLocalRemote(r.RemoteAddr) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade error: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("client connected: %s", conn.RemoteAddr())
	state := &sessionState{customCmds: make(map[string]RegisteredCommand)}

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("read error: %v", err)
			return
		}

		var req Request
		if err := json.Unmarshal(msg, &req); err != nil {
			s.send(conn, Response{Ok: false, Err: "invalid json"})
			continue
		}

		resp := s.dispatch(req, state)
		s.send(conn, resp)
	}
}

func (s *Server) send(conn *websocket.Conn, resp Response) {
	data, _ := json.Marshal(resp)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("write error: %v", err)
	}
}

func (s *Server) dispatch(req Request, state *sessionState) Response {
	if cfg, ok := state.customCmds[req.Cmd]; ok {
		return s.handleCustomCommand(req, cfg)
	}
	if !s.isBuiltinAllowed(req.Cmd) {
		return Response{Ok: false, Err: fmt.Sprintf("cmd not allowed in profile %s: %s", s.profile, req.Cmd)}
	}
	switch req.Cmd {
	case "playermgr":
		return s.handlePlayerMgr(req)
	case "additem":
		return s.handleAddItem(req)
	case "removeitem":
		return s.handleRemoveItem(req)
	case "checkin":
		return s.handleCheckIn(req)
	case "claimreward":
		return s.handleClaimReward(req)
	case "equip":
		return s.handleEquip(req)
	case "unequip":
		return s.handleUnequip(req)
	case "claimmail":
		return s.handleClaimMail(req)
	case "next":
		return s.handleNext(req)
	case "batch":
		return s.handleBatch()
	case "help":
		return s.handleHelp(state)
	case "register_cmd":
		return s.handleRegisterCmd(req, state)
	case "listcmd":
		return s.handleListCmd(state)
	default:
		return Response{Ok: false, Err: fmt.Sprintf("unknown cmd: %s", req.Cmd)}
	}
}

func (s *Server) isBuiltinAllowed(cmd string) bool {
	switch s.profile {
	case "l0":
		return cmd == "playermgr" || cmd == "next" || cmd == "batch" || cmd == "help" || cmd == "register_cmd" || cmd == "listcmd"
	case "l1":
		return cmd == "playermgr" || cmd == "next" || cmd == "batch" || cmd == "help" || cmd == "register_cmd" || cmd == "listcmd" ||
			cmd == "additem" || cmd == "removeitem" || cmd == "checkin" || cmd == "claimreward" || cmd == "equip" || cmd == "unequip" || cmd == "claimmail"
	case "step-only", "dual":
		return cmd == "playermgr" || cmd == "next" || cmd == "help" ||
			cmd == "additem" || cmd == "removeitem" || cmd == "checkin" || cmd == "claimreward" || cmd == "equip" || cmd == "unequip" || cmd == "claimmail"
	case "batch-only", "code-batch":
		return cmd == "batch" || cmd == "help" ||
			cmd == "additem" || cmd == "removeitem" || cmd == "checkin" || cmd == "claimreward" || cmd == "equip" || cmd == "unequip" || cmd == "claimmail"
	default:
		return true
	}
}

func (s *Server) handlePlayerMgr(req Request) Response {
	p := s.pm.GetPlayer(req.PlayerID)
	if p == nil {
		return Response{Ok: false, Err: "player not found"}
	}

	switch req.Sub {
	case "bag":
		if req.ItemID != 0 {
			it := p.Bag.GetItem(req.ItemID)
			if it == nil {
				return Response{Ok: true, Data: nil}
			}
			return Response{Ok: true, Data: it}
		}
		return Response{Ok: true, Data: p.Bag.AllItems()}
	case "task":
		if req.TaskID != 0 {
			t := p.Tasks.GetTask(req.TaskID)
			if t == nil {
				return Response{Ok: true, Data: nil}
			}
			return Response{Ok: true, Data: t}
		}
		return Response{Ok: true, Data: p.Tasks.AllTasks()}
	case "achievement":
		if req.AchID != 0 {
			a := p.Achievements.GetAchievement(req.AchID)
			if a == nil {
				return Response{Ok: true, Data: nil}
			}
			return Response{Ok: true, Data: a}
		}
		return Response{Ok: true, Data: p.Achievements.AllAchievements()}
	case "equipment":
		return Response{Ok: true, Data: p.Equipment.All()}
	case "signin":
		if req.Day != 0 {
			return Response{Ok: true, Data: p.SignIn.GetDay(req.Day)}
		}
		return Response{Ok: true, Data: p.SignIn.AllDays()}
	case "mail":
		if req.MailID != 0 {
			return Response{Ok: true, Data: p.Mail.GetMail(req.MailID)}
		}
		return Response{Ok: true, Data: p.Mail.AllMails()}
	default:
		return Response{
			Ok: true,
			Data: map[string]any{
				"playerId": p.ID,
				"modules":  []string{"bag", "task", "achievement", "equipment", "signin", "mail"},
			},
		}
	}
}

func (s *Server) handleAddItem(req Request) Response {
	if req.Count <= 0 {
		return Response{Ok: false, Err: "invalid count: additem requires count > 0 at the command layer"}
	}
	p := s.pm.GetPlayer(req.PlayerID)
	if p == nil {
		return Response{Ok: false, Err: "player not found"}
	}
	s.bp.Enqueue(breakpoint.PendingOp{
		Execute: func() {
			p.Bag.AddItem(req.ItemID, req.Count)
		},
	})
	return Response{Ok: true, Data: map[string]any{"queued": true, "pendingOps": s.bp.PendingCount()}}
}

func (s *Server) handleRemoveItem(req Request) Response {
	if req.Count <= 0 {
		return Response{Ok: false, Err: "invalid count: removeitem requires count > 0 at the command layer"}
	}
	p := s.pm.GetPlayer(req.PlayerID)
	if p == nil {
		return Response{Ok: false, Err: "player not found"}
	}
	s.bp.Enqueue(breakpoint.PendingOp{
		Execute: func() {
			p.Bag.RemoveItem(req.ItemID, req.Count)
		},
	})
	return Response{Ok: true, Data: map[string]any{"queued": true, "pendingOps": s.bp.PendingCount()}}
}

func (s *Server) handleCheckIn(req Request) Response {
	p := s.pm.GetPlayer(req.PlayerID)
	if p == nil {
		return Response{Ok: false, Err: "player not found"}
	}
	s.bp.Enqueue(breakpoint.PendingOp{
		Execute: func() {
			p.SignIn.CheckIn(req.Day)
		},
	})
	return Response{Ok: true, Data: map[string]any{"queued": true, "pendingOps": s.bp.PendingCount()}}
}

func (s *Server) handleClaimReward(req Request) Response {
	p := s.pm.GetPlayer(req.PlayerID)
	if p == nil {
		return Response{Ok: false, Err: "player not found"}
	}
	s.bp.Enqueue(breakpoint.PendingOp{
		Execute: func() {
			p.SignIn.ClaimReward(req.Day)
		},
	})
	return Response{Ok: true, Data: map[string]any{"queued": true, "pendingOps": s.bp.PendingCount()}}
}

func (s *Server) handleEquip(req Request) Response {
	p := s.pm.GetPlayer(req.PlayerID)
	if p == nil {
		return Response{Ok: false, Err: "player not found"}
	}
	s.bp.Enqueue(breakpoint.PendingOp{
		Execute: func() {
			p.Equipment.Equip(equipment.EquipSlot(req.Slot), req.ItemID)
		},
	})
	return Response{Ok: true, Data: map[string]any{"queued": true, "pendingOps": s.bp.PendingCount()}}
}

func (s *Server) handleUnequip(req Request) Response {
	p := s.pm.GetPlayer(req.PlayerID)
	if p == nil {
		return Response{Ok: false, Err: "player not found"}
	}
	s.bp.Enqueue(breakpoint.PendingOp{
		Execute: func() {
			p.Equipment.Unequip(equipment.EquipSlot(req.Slot))
		},
	})
	return Response{Ok: true, Data: map[string]any{"queued": true, "pendingOps": s.bp.PendingCount()}}
}

func (s *Server) handleClaimMail(req Request) Response {
	p := s.pm.GetPlayer(req.PlayerID)
	if p == nil {
		return Response{Ok: false, Err: "player not found"}
	}
	s.bp.Enqueue(breakpoint.PendingOp{
		Execute: func() {
			p.Mail.ClaimAttachment(req.MailID)
		},
	})
	return Response{Ok: true, Data: map[string]any{"queued": true, "pendingOps": s.bp.PendingCount()}}
}

func (s *Server) handleNext(req Request) Response {
	logs := s.bp.Next()
	return Response{Ok: true, Log: logs}
}

func (s *Server) handleBatch() Response {
	var allLogs []string
	for {
		logs := s.bp.Next()
		if len(logs) == 0 {
			break
		}
		allLogs = append(allLogs, logs...)
	}
	return Response{Ok: true, Log: allLogs}
}

func (s *Server) handleHelp(state *sessionState) Response {
	commands := []map[string]string{}
	add := func(cmd, desc string) {
		if s.isBuiltinAllowed(cmd) {
			commands = append(commands, map[string]string{"cmd": cmd, "desc": desc})
		}
	}
	add("playermgr", "query player data, sub: bag/task/achievement/equipment/signin/mail")
	add("additem", "enqueue add item (requires playerId, itemId, count>0)")
	add("removeitem", "enqueue remove item (requires playerId, itemId, count>0)")
	add("checkin", "enqueue sign-in check-in (requires playerId, day)")
	add("claimreward", "enqueue claim sign-in reward (requires playerId, day)")
	add("equip", "enqueue equip item (requires playerId, slot, itemId)")
	add("unequip", "enqueue unequip slot (requires playerId, slot)")
	add("claimmail", "enqueue claim mail attachment (requires playerId, mailId)")
	add("register_cmd", "register a named raw test command (requires name, target, action)")
	add("listcmd", "list registered custom commands")
	add("next", "execute one pending operation and return logs")
	add("batch", "execute all pending operations and return all logs")
	add("help", "show this help")

	custom := make([]RegisteredCommand, 0, len(state.customCmds))
	for _, cmd := range state.customCmds {
		custom = append(custom, cmd)
	}
	return Response{Ok: true, Data: map[string]any{"commands": commands, "customCommands": custom}}
}

func (s *Server) handleRegisterCmd(req Request, state *sessionState) Response {
	if req.Name == "" || req.Target == "" || req.Action == "" {
		return Response{Ok: false, Err: "register_cmd requires name, target, action"}
	}
	if s.isReservedBuiltin(req.Name) {
		return Response{Ok: false, Err: fmt.Sprintf("command name conflicts with builtin: %s", req.Name)}
	}
	if _, exists := state.customCmds[req.Name]; exists {
		return Response{Ok: false, Err: fmt.Sprintf("command already registered: %s", req.Name)}
	}
	if !s.isAllowedRawBinding(req.Target, req.Action) {
		return Response{Ok: false, Err: fmt.Sprintf("unsupported raw binding: %s.%s", req.Target, req.Action)}
	}
	state.customCmds[req.Name] = RegisteredCommand{
		Name:        req.Name,
		Target:      req.Target,
		Action:      req.Action,
		Description: req.Desc,
	}
	return Response{
		Ok: true,
		Data: map[string]any{
			"registered":  true,
			"name":        req.Name,
			"target":      req.Target,
			"action":      req.Action,
			"description": req.Desc,
		},
	}
}

func (s *Server) handleListCmd(state *sessionState) Response {
	commands := make([]RegisteredCommand, 0, len(state.customCmds))
	for _, cmd := range state.customCmds {
		commands = append(commands, cmd)
	}
	return Response{Ok: true, Data: commands}
}

func (s *Server) isReservedBuiltin(cmd string) bool {
	reserved := []string{
		"playermgr", "additem", "removeitem", "checkin", "claimreward", "equip",
		"unequip", "claimmail", "next", "batch", "help", "register_cmd", "listcmd",
	}
	for _, name := range reserved {
		if cmd == name {
			return true
		}
	}
	return false
}

func (s *Server) isAllowedRawBinding(target, action string) bool {
	switch target {
	case "bag":
		return action == "AddItem" || action == "RemoveItem"
	case "signin":
		return action == "CheckIn" || action == "ClaimReward"
	case "equipment":
		return action == "Equip" || action == "Unequip"
	case "mail":
		return action == "ClaimAttachment"
	default:
		return false
	}
}

func (s *Server) handleCustomCommand(req Request, cfg RegisteredCommand) Response {
	p := s.pm.GetPlayer(req.PlayerID)
	if p == nil {
		return Response{Ok: false, Err: "player not found"}
	}
	switch cfg.Target {
	case "bag":
		switch cfg.Action {
		case "AddItem":
			s.bp.Enqueue(breakpoint.PendingOp{Execute: func() { p.Bag.AddItem(req.ItemID, req.Count) }})
		case "RemoveItem":
			s.bp.Enqueue(breakpoint.PendingOp{Execute: func() { p.Bag.RemoveItem(req.ItemID, req.Count) }})
		default:
			return Response{Ok: false, Err: fmt.Sprintf("unsupported custom action: %s.%s", cfg.Target, cfg.Action)}
		}
	case "signin":
		switch cfg.Action {
		case "CheckIn":
			s.bp.Enqueue(breakpoint.PendingOp{Execute: func() { p.SignIn.CheckIn(req.Day) }})
		case "ClaimReward":
			s.bp.Enqueue(breakpoint.PendingOp{Execute: func() { p.SignIn.ClaimReward(req.Day) }})
		default:
			return Response{Ok: false, Err: fmt.Sprintf("unsupported custom action: %s.%s", cfg.Target, cfg.Action)}
		}
	case "equipment":
		switch cfg.Action {
		case "Equip":
			s.bp.Enqueue(breakpoint.PendingOp{Execute: func() { p.Equipment.Equip(equipment.EquipSlot(req.Slot), req.ItemID) }})
		case "Unequip":
			s.bp.Enqueue(breakpoint.PendingOp{Execute: func() { p.Equipment.Unequip(equipment.EquipSlot(req.Slot)) }})
		default:
			return Response{Ok: false, Err: fmt.Sprintf("unsupported custom action: %s.%s", cfg.Target, cfg.Action)}
		}
	case "mail":
		if cfg.Action != "ClaimAttachment" {
			return Response{Ok: false, Err: fmt.Sprintf("unsupported custom action: %s.%s", cfg.Target, cfg.Action)}
		}
		s.bp.Enqueue(breakpoint.PendingOp{Execute: func() { p.Mail.ClaimAttachment(req.MailID) }})
	default:
		return Response{Ok: false, Err: fmt.Sprintf("unsupported custom target: %s", cfg.Target)}
	}
	return Response{
		Ok: true,
		Data: map[string]any{
			"queued":     true,
			"pendingOps": s.bp.PendingCount(),
			"customCmd":  cfg.Name,
			"binding":    fmt.Sprintf("%s.%s", cfg.Target, cfg.Action),
		},
	}
}
