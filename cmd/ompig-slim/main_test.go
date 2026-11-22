package main

import "testing"

func TestBrokerStartupSkipsNonInteractiveFastPaths(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "version", args: []string{"--version"}},
		{name: "offline", args: []string{"--offline", "--no-session"}},
		{name: "diagnose", args: []string{"diagnose"}},
		{name: "help", args: []string{"--help"}},
		{name: "durable status", args: []string{"durable", "status", "--state", "/unused"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if brokerStartupNeeded(test.args) {
				t.Fatalf("broker startup enabled for %v", test.args)
			}
		})
	}
	if !brokerStartupNeeded([]string{"--no-session"}) {
		t.Fatal("normal session skipped broker startup")
	}
}
