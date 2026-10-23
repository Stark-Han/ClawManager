package repository

import (
	"fmt"
	"time"

	"clawreef/internal/models"

	"github.com/upper/db/v4"
)

// LLMModelRepository defines repository operations for admin-managed models.
type LLMModelRepository interface {
	List() ([]models.LLMModel, error)
	ListActive() ([]models.LLMModel, error)
	GetByID(id int) (*models.LLMModel, error)
	GetByDisplayName(displayName string) (*models.LLMModel, error)
	Save(model *models.LLMModel) error
	Delete(id int) error
}

type llmModelRepository struct {
	sess db.Session
}

// NewLLMModelRepository creates a new model repository (schema is owned by embedded migrations).
func NewLLMModelRepository(sess db.Session) LLMModelRepository {
	repo := &llmModelRepository{sess: sess}
	return repo
}

func (r *llmModelRepository) List() ([]models.LLMModel, error) {
	var items []models.LLMModel
	if err := r.sess.Collection("llm_models").Find().OrderBy("-is_secure", "display_name").All(&items); err != nil {
		return nil, fmt.Errorf("failed to list llm models: %w", err)
	}
	populateProviderModels(items)
	return items, nil
}

func (r *llmModelRepository) ListActive() ([]models.LLMModel, error) {
	var items []models.LLMModel
	if err := r.sess.Collection("llm_models").Find(db.Cond{"is_active": true}).OrderBy("-is_secure", "display_name").All(&items); err != nil {
		return nil, fmt.Errorf("failed to list active llm models: %w", err)
	}
	populateProviderModels(items)
	return items, nil
}

func (r *llmModelRepository) GetByID(id int) (*models.LLMModel, error) {
	var item models.LLMModel
	err := r.sess.Collection("llm_models").Find(db.Cond{"id": id}).One(&item)
	if err != nil {
		if err == db.ErrNoMoreRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get llm model by id: %w", err)
	}
	models.PopulateLLMProviderModels(&item)
	return &item, nil
}

func (r *llmModelRepository) GetByDisplayName(displayName string) (*models.LLMModel, error) {
	var item models.LLMModel
	err := r.sess.Collection("llm_models").Find(db.Cond{"display_name": displayName}).One(&item)
	if err != nil {
		if err == db.ErrNoMoreRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get llm model by display name: %w", err)
	}
	models.PopulateLLMProviderModels(&item)
	return &item, nil
}

func (r *llmModelRepository) Save(model *models.LLMModel) error {
	now := time.Now()
	if model.ID == 0 {
		model.CreatedAt = now
		model.UpdatedAt = now
		res, err := r.sess.Collection("llm_models").Insert(model)
		if err != nil {
			return fmt.Errorf("failed to create llm model: %w", err)
		}
		model.ID = int(res.ID().(int64))
		return nil
	}

	model.UpdatedAt = now
	if err := r.sess.Collection("llm_models").Find(db.Cond{"id": model.ID}).Update(model); err != nil {
		return fmt.Errorf("failed to update llm model: %w", err)
	}
	return nil
}

func (r *llmModelRepository) Delete(id int) error {
	if err := r.sess.Collection("llm_models").Find(db.Cond{"id": id}).Delete(); err != nil {
		return fmt.Errorf("failed to delete llm model: %w", err)
	}
	return nil
}

func populateProviderModels(items []models.LLMModel) {
	for index := range items {
		models.PopulateLLMProviderModels(&items[index])
	}
}
