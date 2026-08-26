package aigateway

import (
	"testing"

	"clawreef/internal/models"
)

type catalogModelRepository struct {
	active []models.LLMModel
}

func (r *catalogModelRepository) List() ([]models.LLMModel, error) { return r.active, nil }
func (r *catalogModelRepository) ListActive() ([]models.LLMModel, error) {
	return r.active, nil
}
func (r *catalogModelRepository) GetByID(int) (*models.LLMModel, error) { return nil, nil }
func (r *catalogModelRepository) GetByDisplayName(string) (*models.LLMModel, error) {
	return nil, nil
}
func (r *catalogModelRepository) Save(*models.LLMModel) error { return nil }
func (r *catalogModelRepository) Delete(int) error            { return nil }

func TestAvailableAndRequestedModelsExpandProviderCatalog(t *testing.T) {
	configured := models.LLMModel{
		ID:                2,
		DisplayName:       "icompify",
		ProviderType:      models.ProviderTypeOpenAICompatible,
		ProtocolType:      models.ProtocolTypeOpenAICompatible,
		ProviderModelName: "qwen3.8",
		IsActive:          true,
	}
	if err := models.SetLLMProviderModels(&configured, []models.LLMProviderModel{
		{ID: "qwen3.8"},
		{ID: "glm-5.2"},
		{ID: "deepseek-v4-pro"},
	}); err != nil {
		t.Fatal(err)
	}
	service := &service{modelRepo: &catalogModelRepository{active: []models.LLMModel{configured}}}

	available, err := service.ListAvailableModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(available) != 3 || available[0].DisplayName != "qwen3.8" || available[2].DisplayName != "deepseek-v4-pro" {
		t.Fatalf("available models were not expanded: %#v", available)
	}

	selected, err := service.resolveRequestedModel("glm-5.2")
	if err != nil {
		t.Fatal(err)
	}
	if selected.DisplayName != "glm-5.2" || selected.ProviderModelName != "glm-5.2" || selected.ID != 2 {
		t.Fatalf("requested provider model was not resolved through its provider config: %#v", selected)
	}
}
