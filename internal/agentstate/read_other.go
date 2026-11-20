//go:build !windows

package agentstate

import "os"

func readStateFile(path string) ([]byte, error) { return os.ReadFile(path) }
