package cli

import (
	"context"
	"sync"
	"testing"

	"github.com/MichaelKinsy/PiG/coding"
	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/internal/codingagent"
)

func TestRPCBindPublishesEventsEmittedBySessionStart(t *testing.T) {
	t.Setenv("PI_OFFLINE", "1")
	cwd, agentDir := t.TempDir(), t.TempDir()
	services, err := coding.CreateAgentSessionServices(coding.CreateAgentSessionServicesOptions{
		CWD: cwd, AgentDir: agentDir, SettingsManager: coding.NewInMemorySettingsManager(coding.Settings{}),
	})
	if err != nil {
		t.Fatal(err)
	}

	var session *coding.Session
	var starts int
	ext := extension.Extension{
		Name: "fixture",
		Path: "fixture",
		Handlers: map[string][]extension.HandlerFn{
			codingagent.EventSessionStart: {func(...any) (any, error) {
				starts++
				return nil, session.SetSessionName("set during session_start")
			}},
		},
	}
	runner := coding.NewExtensionRunner([]extension.Extension{ext}, nil, cwd, nil, services.Registry())
	session, err = coding.NewSession(services, coding.SessionOptions{
		NoSession: true, NoTools: "all", ResourceLoader: coding.NoResources, Runner: runner,
	})
	if err != nil {
		services.Close()
		t.Fatal(err)
	}
	runtime, err := coding.NewAgentSessionRuntime(context.Background(), session, services, func(context.Context, coding.CreateAgentSessionRuntimeOptions) (coding.CreateAgentSessionRuntimeResult, error) {
		return coding.CreateAgentSessionRuntimeResult{}, nil
	}, nil, "")
	if err != nil {
		session.Close()
		t.Fatal(err)
	}

	written := make(chan any, 16)
	host := &rpcHost{
		ctx:           context.Background(),
		ui:            newRPCUIContext(func(any) {}),
		writeRPC:      func(event any) { written <- event },
		responses:     &rpcResponseTurn{},
		promptWG:      &sync.WaitGroup{},
		tasks:         &rpcTaskGroup{},
		conversionErr: make(chan error, 1),
	}
	host.rt = runtime
	if err := host.bind(context.Background(), session, session.StartEvent(), false); err != nil {
		t.Fatal(err)
	}
	if starts != 1 {
		t.Fatalf("session_start handlers ran %d times, want once", starts)
	}
	if err := session.FlushEvents(context.Background()); err != nil {
		t.Fatal(err)
	}
	found := false
	for {
		select {
		case event := <-written:
			if value, ok := event.(RPCSessionInfoChangedEvent); ok && value.Name == "set during session_start" {
				found = true
			}
		default:
			if !found {
				t.Fatal("RPC did not publish session_info_changed emitted by session_start")
			}
			host.detached.Store(true)
			if err := runtime.Close(); err != nil {
				t.Fatal(err)
			}
			host.forwarders.Wait()
			host.closeHost()
			return
		}
	}
}
