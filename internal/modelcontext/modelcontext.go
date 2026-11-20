package modelcontext

import (
	"context"
	"fmt"

	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/durable"
)

func description(provider, model string) string {
	return fmt.Sprintf("Selected provider: %s\nSelected model: %s", provider, model)
}

const customType = "model-identity"

// Extension adds an identity update only when the projected conversation needs one.
func Extension(api extension.API) error {
	api.OnBeforeAgentStart(func(ctx context.Context, _ extension.BeforeAgentStartEvent) (extension.BeforeAgentStartEventResult, error) {
		model, err := extension.FromContext(ctx).Model()
		if err != nil || model == nil {
			return extension.BeforeAgentStartEventResult{}, err
		}
		provider := model.ProviderMeta.ProviderID
		if provider == "" && model.Provider != nil {
			provider = model.Provider.ID()
		}
		value := description(provider, model.ID)
		manager, err := extension.FromContext(ctx).SessionManager()
		if err != nil {
			return extension.BeforeAgentStartEventResult{}, err
		}
		if manager != nil {
			messages := manager.BuildSessionProjection().Messages
			for i := len(messages) - 1; i >= 0; i-- {
				message := messages[i].Custom
				if message != nil && message["customType"] == customType {
					if message["content"] == value {
						return extension.BeforeAgentStartEventResult{}, nil
					}
					break
				}
			}
		}
		return extension.BeforeAgentStartEventResult{Message: &extension.CustomMessageRef{
			CustomType: customType, Content: value, Display: false,
		}}, nil
	})
	return nil
}

// DurableExtension supplies the corresponding section to the durable harness.
func DurableExtension() *durable.Extension {
	return &durable.Extension{Name: "model-context", Sections: []*durable.PromptSection{{
		Key: "model_identity",
		Render: func(_ context.Context, input durable.PromptInput) (*string, error) {
			if input.Agent.Model == nil {
				return nil, nil
			}
			value := description(input.Agent.Model.Provider, input.Agent.Model.ModelId)
			return &value, nil
		},
	}}}
}
