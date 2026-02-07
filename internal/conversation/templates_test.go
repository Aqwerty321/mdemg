package conversation

import (
	"testing"
)

func TestGetTemplateSubspace(t *testing.T) {
	tests := []struct {
		spaceID  string
		expected string
	}{
		{"mdemg-dev", "mdemg-dev:templates"},
		{"my-space", "my-space:templates"},
		{"", ":templates"},
	}

	for _, tt := range tests {
		result := GetTemplateSubspace(tt.spaceID)
		if result != tt.expected {
			t.Errorf("GetTemplateSubspace(%q) = %q, want %q", tt.spaceID, result, tt.expected)
		}
	}
}

func TestDefaultTemplates(t *testing.T) {
	templates := DefaultTemplates()

	if len(templates) != 5 {
		t.Errorf("Expected 5 default templates, got %d", len(templates))
	}

	// Check required templates exist
	expectedIDs := map[string]bool{
		"task_handoff": false,
		"decision":     false,
		"error":        false,
		"learning":     false,
		"correction":   false,
	}

	for _, tmpl := range templates {
		if _, ok := expectedIDs[tmpl.TemplateID]; ok {
			expectedIDs[tmpl.TemplateID] = true
		}

		// Validate required fields
		if tmpl.TemplateID == "" {
			t.Error("Template has empty TemplateID")
		}
		if tmpl.Name == "" {
			t.Error("Template has empty Name")
		}
		if tmpl.Schema == nil {
			t.Errorf("Template %s has nil Schema", tmpl.TemplateID)
		}
	}

	for id, found := range expectedIDs {
		if !found {
			t.Errorf("Expected template %q not found in defaults", id)
		}
	}
}

func TestDefaultTemplate_TaskHandoff(t *testing.T) {
	templates := DefaultTemplates()

	var taskHandoff *ObservationTemplate
	for _, tmpl := range templates {
		if tmpl.TemplateID == "task_handoff" {
			taskHandoff = tmpl
			break
		}
	}

	if taskHandoff == nil {
		t.Fatal("task_handoff template not found")
	}

	// Check auto-capture config
	if taskHandoff.AutoCapture == nil {
		t.Fatal("task_handoff should have auto_capture config")
	}
	if !taskHandoff.AutoCapture.OnSessionEnd {
		t.Error("task_handoff should have on_session_end = true")
	}
	if !taskHandoff.AutoCapture.OnCompaction {
		t.Error("task_handoff should have on_compaction = true")
	}

	// Check schema has required fields
	schema := taskHandoff.Schema
	if schema["type"] != "object" {
		t.Error("Schema type should be 'object'")
	}

	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("Schema should have 'required' array")
	}
	if len(required) != 2 {
		t.Errorf("Expected 2 required fields, got %d", len(required))
	}
}

func TestDefaultTemplate_Error(t *testing.T) {
	templates := DefaultTemplates()

	var errorTmpl *ObservationTemplate
	for _, tmpl := range templates {
		if tmpl.TemplateID == "error" {
			errorTmpl = tmpl
			break
		}
	}

	if errorTmpl == nil {
		t.Fatal("error template not found")
	}

	// Check auto-capture config
	if errorTmpl.AutoCapture == nil {
		t.Fatal("error should have auto_capture config")
	}
	if !errorTmpl.AutoCapture.OnError {
		t.Error("error should have on_error = true")
	}
	if errorTmpl.AutoCapture.OnSessionEnd {
		t.Error("error should NOT have on_session_end = true")
	}
}

func TestObservationTemplate_Validation(t *testing.T) {
	template := &ObservationTemplate{
		TemplateID: "test",
		SpaceID:    "test-space",
		Name:       "Test Template",
		Schema: map[string]interface{}{
			"type":     "object",
			"required": []string{"field1"},
			"properties": map[string]interface{}{
				"field1": map[string]interface{}{"type": "string"},
			},
		},
	}

	// Check template is valid
	if template.TemplateID == "" {
		t.Error("TemplateID should not be empty")
	}
	if template.SpaceID == "" {
		t.Error("SpaceID should not be empty")
	}
	if template.Name == "" {
		t.Error("Name should not be empty")
	}
	if template.Schema == nil {
		t.Error("Schema should not be nil")
	}
}
