//go:build !windows

package selfupdate

import "testing"

func fixturePayload(t *testing.T, script string) []byte {
	t.Helper()
	return []byte(script)
}
