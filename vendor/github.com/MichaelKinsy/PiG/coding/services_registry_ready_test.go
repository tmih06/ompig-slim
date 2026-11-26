package coding

import (
	"testing"

	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
)

func TestRegistryReadyRunsBeforeServicesReturn(t *testing.T) {
	dir := t.TempDir()
	settings := NewInMemorySettingsManager(Settings{})
	ready := make(chan struct{})
	var readyRegistry *icodingagent.ModelRegistry
	var readySettings *SettingsManager

	services, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD:             dir,
		AgentDir:        dir,
		SettingsManager: settings,
		RegistryReady: func(registry *icodingagent.ModelRegistry, settingsManager *SettingsManager) {
			readyRegistry = registry
			readySettings = settingsManager
			close(ready)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer services.Close()

	select {
	case <-ready:
	default:
		t.Fatal("RegistryReady did not run before CreateAgentSessionServices returned")
	}
	if readyRegistry == nil || readySettings == nil {
		t.Fatalf("RegistryReady received nil inputs: registry=%v settings=%v", readyRegistry, readySettings)
	}
	if services.Registry().ModelRegistry != readyRegistry {
		t.Fatal("RegistryReady received a registry other than the services registry")
	}
	if services.SettingsManager() != readySettings {
		t.Fatal("RegistryReady received a settings manager other than the services settings manager")
	}
}
