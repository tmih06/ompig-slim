package codingagent

import (
	"context"
	"slices"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
)

// TestProfileLoginSelector checks the native /login list, autocomplete and direct
// slash-command routing without registering a synthetic inference provider.
func TestProfileLoginSelector(t *testing.T) {
	previous := ProfileLogin
	t.Cleanup(func() { ProfileLogin = previous })
	ProfileLogin = &ProfileLoginOption{ID: "remote-ompig-slim-broker", Name: "Remote ompig-slim broker", Login: func(context.Context, string, ai.AuthInteraction) error { return nil }}
	m := &InteractiveMode{}
	m.opts.AgentDir = t.TempDir()
	options := m.getLoginProviderOptions(false)
	index := slices.IndexFunc(options, func(p tui.OAuthProvider) bool { return p.ID == ProfileLogin.ID })
	if index < 0 || options[index].AuthType != "profile" || options[index].Name != ProfileLogin.Name {
		t.Fatal("missing profile login entry")
	}
	if tui.FormatAuthSelectorProviderType(options[index].AuthType) != "profile sync" {
		t.Fatal("profile login mislabelled as model authentication")
	}
	if len(m.loginArgCompletions(ProfileLogin.ID)) != 1 {
		t.Fatal("missing slash-command completion")
	}
	var selected tui.OAuthProvider
	sc := &SlashContext{Args: ProfileLogin.ID, LoginProviders: func() []tui.OAuthProvider { return options }, StartProviderLogin: func(p tui.OAuthProvider) error { selected = p; return nil }}
	if err := handleLoginCommand(sc); err != nil {
		t.Fatal(err)
	}
	if selected.ID != ProfileLogin.ID || selected.AuthType != "profile" {
		t.Fatal("direct /login did not select storage setup")
	}
}
