package cli

// Ports packages/coding-agent/src/package-manager-cli.ts:583-615.

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding"
	"github.com/MichaelKinsy/PiG/internal/codingagent"
)

type packageModelRuntime interface {
	Refresh(context.Context, ...ai.ModelsRefreshOptions) ai.ModelsRefreshResult
	Close()
}

var createPackageModelRuntime = func(ctx context.Context, options coding.CreateModelRuntimeOptions) (packageModelRuntime, error) {
	return coding.CreateModelRuntime(ctx, options)
}

func refreshModelCatalogs(agentDir string) error {
	// upstream: packages/coding-agent/src/package-manager-cli.ts:refreshModelCatalogs
	ctx, cancel := context.WithTimeout(context.Background(), 15000*time.Millisecond)
	defer cancel()
	modelsPath := codingagent.ModelsPath(agentDir)
	credentials, err := ai.OpenCredentialStore(agentDir)
	if err != nil {
		return err
	}
	defer func() { _ = credentials.Close() }()
	runtime, err := createPackageModelRuntime(ctx, coding.CreateModelRuntimeOptions{
		Credentials: credentials, ModelsPath: new(&modelsPath), AllowModelNetwork: false,
	})
	if err != nil {
		return err
	}
	defer runtime.Close()
	result := runtime.Refresh(ctx, ai.ModelsRefreshOptions{AllowNetwork: new(true), Force: new(true)})
	if result.Aborted {
		return fmt.Errorf("Model catalog refresh timed out.")
	}
	if len(result.Errors) > 0 {
		order := slices.Clone(result.ErrorOrder)
		for _, id := range slices.Sorted(maps.Keys(result.Errors)) {
			if !slices.Contains(order, id) {
				order = append(order, id)
			}
		}
		details := make([]string, 0, len(order))
		for _, id := range order {
			details = append(details, id+": "+result.Errors[id].Error())
		}
		return fmt.Errorf("Could not refresh model catalogs: %s", strings.Join(details, "; "))
	}
	fmt.Println(chalkGreen("Model catalogs refreshed"))
	return nil
}
