package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// TemplateSubspaceSuffix is appended to space_id for template storage
const TemplateSubspaceSuffix = ":templates"

// ObservationTemplate defines a structured observation schema
type ObservationTemplate struct {
	TemplateID   string                 `json:"template_id"`
	SpaceID      string                 `json:"space_id"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description,omitempty"`
	ObsType      ObservationType        `json:"obs_type"`
	Schema       map[string]interface{} `json:"schema"` // JSON Schema
	AutoCapture  *AutoCaptureConfig     `json:"auto_capture,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// AutoCaptureConfig defines when to auto-capture using this template
type AutoCaptureConfig struct {
	OnSessionEnd bool `json:"on_session_end"`
	OnCompaction bool `json:"on_compaction"`
	OnError      bool `json:"on_error"`
}

// TemplateService handles observation template operations
type TemplateService struct {
	driver neo4j.DriverWithContext
}

// NewTemplateService creates a new template service
func NewTemplateService(driver neo4j.DriverWithContext) *TemplateService {
	return &TemplateService{driver: driver}
}

// GetTemplateSubspace returns the sub-space ID for template storage
func GetTemplateSubspace(spaceID string) string {
	return spaceID + TemplateSubspaceSuffix
}

// CreateTemplate creates a new observation template
func (s *TemplateService) CreateTemplate(ctx context.Context, template *ObservationTemplate) error {
	if template.TemplateID == "" {
		template.TemplateID = uuid.New().String()
	}
	if template.SpaceID == "" {
		return fmt.Errorf("space_id is required")
	}
	if template.Name == "" {
		return fmt.Errorf("name is required")
	}
	if template.ObsType == "" {
		template.ObsType = ObsTypeContext
	}

	now := time.Now()
	template.CreatedAt = now
	template.UpdatedAt = now

	// Serialize schema and auto_capture to JSON
	schemaJSON, err := json.Marshal(template.Schema)
	if err != nil {
		return fmt.Errorf("failed to serialize schema: %w", err)
	}

	var autoCaptureJSON []byte
	if template.AutoCapture != nil {
		autoCaptureJSON, err = json.Marshal(template.AutoCapture)
		if err != nil {
			return fmt.Errorf("failed to serialize auto_capture: %w", err)
		}
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			CREATE (t:ObservationTemplate {
				template_id: $templateId,
				space_id: $spaceId,
				name: $name,
				description: $description,
				obs_type: $obsType,
				schema: $schema,
				auto_capture: $autoCapture,
				created_at: datetime($createdAt),
				updated_at: datetime($updatedAt)
			})
			RETURN t.template_id AS id
		`
		params := map[string]interface{}{
			"templateId":  template.TemplateID,
			"spaceId":     GetTemplateSubspace(template.SpaceID),
			"name":        template.Name,
			"description": template.Description,
			"obsType":     string(template.ObsType),
			"schema":      string(schemaJSON),
			"autoCapture": string(autoCaptureJSON),
			"createdAt":   template.CreatedAt.Format(time.RFC3339),
			"updatedAt":   template.UpdatedAt.Format(time.RFC3339),
		}
		_, err := tx.Run(ctx, query, params)
		return nil, err
	})

	return err
}

// GetTemplate retrieves a template by ID
func (s *TemplateService) GetTemplate(ctx context.Context, spaceID, templateID string) (*ObservationTemplate, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (t:ObservationTemplate {space_id: $spaceId, template_id: $templateId})
			RETURN t
		`
		params := map[string]interface{}{
			"spaceId":    GetTemplateSubspace(spaceID),
			"templateId": templateID,
		}
		res, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}

		if res.Next(ctx) {
			return res.Record().Values[0], nil
		}
		return nil, nil
	})

	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	return parseTemplateNode(result.(neo4j.Node), spaceID)
}

// ListTemplates lists all templates for a space
func (s *TemplateService) ListTemplates(ctx context.Context, spaceID string) ([]*ObservationTemplate, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (t:ObservationTemplate {space_id: $spaceId})
			RETURN t
			ORDER BY t.name
		`
		params := map[string]interface{}{
			"spaceId": GetTemplateSubspace(spaceID),
		}
		res, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}

		var templates []*ObservationTemplate
		for res.Next(ctx) {
			node := res.Record().Values[0].(neo4j.Node)
			template, err := parseTemplateNode(node, spaceID)
			if err != nil {
				continue
			}
			templates = append(templates, template)
		}
		return templates, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]*ObservationTemplate), nil
}

// UpdateTemplate updates an existing template
func (s *TemplateService) UpdateTemplate(ctx context.Context, template *ObservationTemplate) error {
	if template.TemplateID == "" || template.SpaceID == "" {
		return fmt.Errorf("template_id and space_id are required")
	}

	template.UpdatedAt = time.Now()

	schemaJSON, err := json.Marshal(template.Schema)
	if err != nil {
		return fmt.Errorf("failed to serialize schema: %w", err)
	}

	var autoCaptureJSON []byte
	if template.AutoCapture != nil {
		autoCaptureJSON, err = json.Marshal(template.AutoCapture)
		if err != nil {
			return fmt.Errorf("failed to serialize auto_capture: %w", err)
		}
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (t:ObservationTemplate {space_id: $spaceId, template_id: $templateId})
			SET t.name = $name,
			    t.description = $description,
			    t.obs_type = $obsType,
			    t.schema = $schema,
			    t.auto_capture = $autoCapture,
			    t.updated_at = datetime($updatedAt)
			RETURN t.template_id AS id
		`
		params := map[string]interface{}{
			"templateId":  template.TemplateID,
			"spaceId":     GetTemplateSubspace(template.SpaceID),
			"name":        template.Name,
			"description": template.Description,
			"obsType":     string(template.ObsType),
			"schema":      string(schemaJSON),
			"autoCapture": string(autoCaptureJSON),
			"updatedAt":   template.UpdatedAt.Format(time.RFC3339),
		}
		res, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return nil, fmt.Errorf("template not found")
		}
		return nil, nil
	})

	return err
}

// DeleteTemplate deletes a template
func (s *TemplateService) DeleteTemplate(ctx context.Context, spaceID, templateID string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (t:ObservationTemplate {space_id: $spaceId, template_id: $templateId})
			DELETE t
			RETURN count(t) AS deleted
		`
		params := map[string]interface{}{
			"spaceId":    GetTemplateSubspace(spaceID),
			"templateId": templateID,
		}
		res, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			deleted, _ := res.Record().Get("deleted")
			if deleted.(int64) == 0 {
				return nil, fmt.Errorf("template not found")
			}
		}
		return nil, nil
	})

	return err
}

// GetTemplatesForAutoCapture returns templates with matching auto-capture trigger
func (s *TemplateService) GetTemplatesForAutoCapture(ctx context.Context, spaceID, trigger string) ([]*ObservationTemplate, error) {
	templates, err := s.ListTemplates(ctx, spaceID)
	if err != nil {
		return nil, err
	}

	var matching []*ObservationTemplate
	for _, t := range templates {
		if t.AutoCapture == nil {
			continue
		}
		switch trigger {
		case "session_end":
			if t.AutoCapture.OnSessionEnd {
				matching = append(matching, t)
			}
		case "compaction":
			if t.AutoCapture.OnCompaction {
				matching = append(matching, t)
			}
		case "error":
			if t.AutoCapture.OnError {
				matching = append(matching, t)
			}
		}
	}
	return matching, nil
}

// parseTemplateNode converts a Neo4j node to ObservationTemplate
func parseTemplateNode(node neo4j.Node, originalSpaceID string) (*ObservationTemplate, error) {
	props := node.Props

	template := &ObservationTemplate{
		TemplateID:  props["template_id"].(string),
		SpaceID:     originalSpaceID, // Return original space_id, not sub-space
		Name:        props["name"].(string),
		Description: getStringProp(props, "description"),
		ObsType:     ObservationType(getStringProp(props, "obs_type")),
	}

	// Parse schema JSON
	if schemaStr, ok := props["schema"].(string); ok && schemaStr != "" {
		var schema map[string]interface{}
		if err := json.Unmarshal([]byte(schemaStr), &schema); err == nil {
			template.Schema = schema
		}
	}

	// Parse auto_capture JSON
	if autoCaptureStr, ok := props["auto_capture"].(string); ok && autoCaptureStr != "" {
		var autoCapture AutoCaptureConfig
		if err := json.Unmarshal([]byte(autoCaptureStr), &autoCapture); err == nil {
			template.AutoCapture = &autoCapture
		}
	}

	// Parse timestamps
	if createdAt, ok := props["created_at"].(time.Time); ok {
		template.CreatedAt = createdAt
	}
	if updatedAt, ok := props["updated_at"].(time.Time); ok {
		template.UpdatedAt = updatedAt
	}

	return template, nil
}

func getStringProp(props map[string]interface{}, key string) string {
	if v, ok := props[key].(string); ok {
		return v
	}
	return ""
}

// =============================================================================
// Default Templates
// =============================================================================

// DefaultTemplates returns the built-in observation templates
func DefaultTemplates() []*ObservationTemplate {
	return []*ObservationTemplate{
		{
			TemplateID:  "task_handoff",
			Name:        "Task Handoff",
			Description: "Capture task state for session continuity",
			ObsType:     ObsTypeContext,
			Schema: map[string]interface{}{
				"type":     "object",
				"required": []string{"task_name", "status"},
				"properties": map[string]interface{}{
					"task_name":        map[string]interface{}{"type": "string", "description": "Current task name"},
					"status":           map[string]interface{}{"type": "string", "enum": []string{"in_progress", "blocked", "completed", "paused"}},
					"active_files":     map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
					"current_goal":     map[string]interface{}{"type": "string"},
					"blockers":         map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
					"next_steps":       map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
					"recent_decisions": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
					"context_notes":    map[string]interface{}{"type": "string"},
				},
			},
			AutoCapture: &AutoCaptureConfig{OnSessionEnd: true, OnCompaction: true},
		},
		{
			TemplateID:  "decision",
			Name:        "Decision Record",
			Description: "Record architectural or implementation decisions with rationale",
			ObsType:     ObsTypeDecision,
			Schema: map[string]interface{}{
				"type":     "object",
				"required": []string{"decision", "rationale"},
				"properties": map[string]interface{}{
					"decision":                map[string]interface{}{"type": "string", "description": "The decision made"},
					"rationale":               map[string]interface{}{"type": "string", "description": "Why this decision was made"},
					"alternatives_considered": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
					"impact":                  map[string]interface{}{"type": "string", "enum": []string{"low", "medium", "high"}},
					"reversible":              map[string]interface{}{"type": "boolean"},
				},
			},
		},
		{
			TemplateID:  "error",
			Name:        "Error Record",
			Description: "Document errors encountered and their resolutions",
			ObsType:     ObsTypeError,
			Schema: map[string]interface{}{
				"type":     "object",
				"required": []string{"error_type", "description"},
				"properties": map[string]interface{}{
					"error_type":  map[string]interface{}{"type": "string", "description": "Category of error"},
					"description": map[string]interface{}{"type": "string", "description": "What happened"},
					"file_path":   map[string]interface{}{"type": "string"},
					"resolution":  map[string]interface{}{"type": "string", "description": "How it was fixed"},
					"root_cause":  map[string]interface{}{"type": "string"},
					"prevention":  map[string]interface{}{"type": "string", "description": "How to prevent in future"},
				},
			},
			AutoCapture: &AutoCaptureConfig{OnError: true},
		},
		{
			TemplateID:  "learning",
			Name:        "Learning Record",
			Description: "Capture new insights and domain knowledge",
			ObsType:     ObsTypeLearning,
			Schema: map[string]interface{}{
				"type":     "object",
				"required": []string{"topic", "insight"},
				"properties": map[string]interface{}{
					"topic":         map[string]interface{}{"type": "string", "description": "Subject area"},
					"insight":       map[string]interface{}{"type": "string", "description": "What was learned"},
					"source":        map[string]interface{}{"type": "string", "description": "Where this came from"},
					"confidence":    map[string]interface{}{"type": "string", "enum": []string{"low", "medium", "high"}},
					"applicable_to": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				},
			},
		},
		{
			TemplateID:  "correction",
			Name:        "Correction Record",
			Description: "Document user corrections and updated understanding",
			ObsType:     ObsTypeCorrection,
			Schema: map[string]interface{}{
				"type":     "object",
				"required": []string{"incorrect", "correct"},
				"properties": map[string]interface{}{
					"incorrect":      map[string]interface{}{"type": "string", "description": "What was wrong"},
					"correct":        map[string]interface{}{"type": "string", "description": "The correct information"},
					"context":        map[string]interface{}{"type": "string"},
					"impact_on_task": map[string]interface{}{"type": "string"},
				},
			},
		},
	}
}

// EnsureDefaultTemplates creates default templates if they don't exist
func (s *TemplateService) EnsureDefaultTemplates(ctx context.Context, spaceID string) error {
	defaults := DefaultTemplates()
	for _, template := range defaults {
		template.SpaceID = spaceID
		existing, err := s.GetTemplate(ctx, spaceID, template.TemplateID)
		if err != nil {
			return err
		}
		if existing == nil {
			if err := s.CreateTemplate(ctx, template); err != nil {
				return fmt.Errorf("failed to create default template %s: %w", template.TemplateID, err)
			}
		}
	}
	return nil
}
