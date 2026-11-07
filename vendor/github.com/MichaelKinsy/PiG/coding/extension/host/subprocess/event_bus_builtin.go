package subprocess

// ompig-slim additive: session-owned built-ins share pi.events with every
// extension realization. Values cross the same JSON boundary as native SDKs.
import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

var builtinListenerSequence atomic.Uint64

// On subscribes an in-process listener and returns an idempotent unsubscribe.
func (h *Host) On(channel string, handler func(any)) func() {
	h.routeEventBus(context.Background())
	listener := &busListener{realm: "builtin", handlerID: fmt.Sprint(builtinListenerSequence.Add(1)), channel: channel, value: true, builtin: handler}
	h.eventBus.add(listener)
	var once sync.Once
	return func() { once.Do(func() { h.eventBus.remove(listener) }) }
}

// Emit delivers a JSON value to the ordered registry, including Node and native
// subprocess listeners. No listener callback runs while the registry is locked.
func (h *Host) Emit(channel string, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		reportEventBusError(channel, err)
		return
	}
	h.dispatchEvent(context.Background(), channel, h.newEmitPayload(true, nil, data), nil, "builtin", "", nil)
}
