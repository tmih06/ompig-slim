package subagents

// Ports src/cross-extension-rpc.ts: protocol 2 reply envelopes and channels.
import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type EventBus interface {
	On(string, func(any)) func()
	Emit(string, any)
}

func (m *Manager) BindRPC(bus EventBus) func() {
	var unsubscribe []func()
	for _, method := range []string{"ping", "spawn", "stop", "consume", "steer"} {
		method := method
		channel := "subagents:rpc:" + method
		unsubscribe = append(unsubscribe, bus.On(channel, func(value any) {
			b, err := json.Marshal(value)
			if err != nil {
				return
			}
			var p struct {
				RequestID                      string `json:"requestId"`
				Type, Prompt, AgentID, Message string
				Options                        map[string]json.RawMessage `json:"options"`
			}
			if json.Unmarshal(b, &p) != nil || p.RequestID == "" {
				return
			}
			reply := func(data any, err error) {
				envelope := map[string]any{"success": err == nil}
				if err != nil {
					envelope["error"] = err.Error()
				} else if data != nil {
					envelope["data"] = data
				}
				bus.Emit(channel+":reply:"+p.RequestID, envelope)
			}
			if method == "ping" {
				reply(map[string]int{"version": 2}, nil)
				return
			}
			if !m.Settings().Enabled {
				reply(nil, fmt.Errorf("subagents are disabled"))
				return
			}
			switch method {
			case "spawn":
				options := map[string]json.RawMessage{}
				for k, v := range p.Options {
					options[k] = v
				}
				for camel, snake := range map[string]string{"maxTurns": "max_turns", "runInBackground": "run_in_background", "inheritContext": "inherit_context"} {
					if v, ok := options[camel]; ok {
						options[snake] = v
					}
				}
				options["subagent_type"], _ = json.Marshal(p.Type)
				options["prompt"], _ = json.Marshal(p.Prompt)
				raw, _ := json.Marshal(options)
				var params Params
				if err = json.Unmarshal(raw, &params); err != nil {
					reply(nil, err)
					return
				}
				params.Detached = true
				r, err := m.Spawn(context.Background(), params, "", "")
				if err != nil {
					reply(nil, err)
					return
				}
				// Startup can need a worktree or authentication. This owned waiter
				// never blocks the event bus and is drained when the session closes.
				m.mu.Lock()
				if m.closed {
					m.mu.Unlock()
					reply(nil, fmt.Errorf("No active session"))
					return
				}
				m.wg.Add(1)
				m.mu.Unlock()
				go func() {
					defer m.wg.Done()
					ticker := time.NewTicker(10 * time.Millisecond)
					defer ticker.Stop()
					for {
						record, ok := m.Find(r.ID)
						if !ok {
							reply(nil, fmt.Errorf("agent not found"))
							return
						}
						if record.SessionFile != "" || !record.Active() {
							if record.Status == "error" {
								reply(nil, fmt.Errorf("%s", record.Error))
							} else {
								reply(map[string]string{"id": r.ID}, nil)
							}
							return
						}
						select {
						case <-m.ctx.Done():
							reply(nil, fmt.Errorf("No active session"))
							return
						case <-ticker.C:
						}
					}
				}()
			case "stop":
				err = m.Stop(p.AgentID, "")
				reply(nil, err)
			case "consume":
				r, e := m.Consume(p.AgentID, "")
				if e == nil && r.Active() {
					e = fmt.Errorf("Agent %s is not finished", p.AgentID)
				}
				reply(nil, e)
			case "steer":
				reply(nil, m.Steer(p.AgentID, p.Message, ""))
			}
		}))
	}
	bus.Emit("subagents:ready", map[string]int{"version": 2})
	return func() {
		for _, fn := range unsubscribe {
			fn()
		}
	}
}
