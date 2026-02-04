package main

import (
	"testing"
)

func TestMatchCondition_Eq(t *testing.T) {
	c := Condition{Field: "priority", Operator: "eq", Value: "1"}
	fields := map[string]string{"priority": "1"}

	if !MatchCondition(c, fields, nil) {
		t.Error("eq should match when values are equal")
	}

	fields["priority"] = "2"
	if MatchCondition(c, fields, nil) {
		t.Error("eq should not match when values differ")
	}
}

func TestMatchCondition_Neq(t *testing.T) {
	c := Condition{Field: "state", Operator: "neq", Value: "closed"}
	fields := map[string]string{"state": "open"}

	if !MatchCondition(c, fields, nil) {
		t.Error("neq should match when values differ")
	}

	fields["state"] = "closed"
	if MatchCondition(c, fields, nil) {
		t.Error("neq should not match when values are equal")
	}
}

func TestMatchCondition_Contains(t *testing.T) {
	c := Condition{Field: "title", Operator: "contains", Value: "bug"}
	fields := map[string]string{"title": "Fix the login bug"}

	if !MatchCondition(c, fields, nil) {
		t.Error("contains should match when field contains value")
	}

	fields["title"] = "Add new feature"
	if MatchCondition(c, fields, nil) {
		t.Error("contains should not match when field doesn't contain value")
	}
}

func TestMatchCondition_ChangedTo(t *testing.T) {
	c := Condition{Field: "state_type", Operator: "changed_to", Value: "completed"}

	fields := map[string]string{"state_type": "completed"}
	previousFields := map[string]string{"state_type": "started"}

	if !MatchCondition(c, fields, previousFields) {
		t.Error("changed_to should match when field changed to target value")
	}

	// Already was completed — no change
	previousFields["state_type"] = "completed"
	if MatchCondition(c, fields, previousFields) {
		t.Error("changed_to should not match when value was already the target")
	}

	// No previous fields (create event)
	if MatchCondition(c, fields, nil) {
		t.Error("changed_to should not match without previous fields")
	}
}

func TestMatchCondition_Exists(t *testing.T) {
	c := Condition{Field: "assignee_id", Operator: "exists"}

	fields := map[string]string{"assignee_id": "user-1"}
	if !MatchCondition(c, fields, nil) {
		t.Error("exists should match when field is present")
	}

	delete(fields, "assignee_id")
	if MatchCondition(c, fields, nil) {
		t.Error("exists should not match when field is missing")
	}
}

func TestMatchCondition_UnknownOperator(t *testing.T) {
	c := Condition{Field: "x", Operator: "bogus", Value: "y"}
	fields := map[string]string{"x": "y"}

	if MatchCondition(c, fields, nil) {
		t.Error("unknown operator should not match")
	}
}

func TestWorkflowEngine_LoadFromBytes(t *testing.T) {
	yaml := `
workflows:
  - name: "test-workflow"
    trigger:
      event: "on-create"
      entity_type: "issue"
      conditions:
        - field: "priority"
          operator: "eq"
          value: "1"
    actions:
      - type: "add-comment"
        params:
          body: "Urgent!"
`

	engine := NewWorkflowEngine()
	if err := engine.LoadFromBytes([]byte(yaml)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(engine.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(engine.Workflows))
	}

	wf := engine.Workflows[0]
	if wf.Name != "test-workflow" {
		t.Errorf("expected name 'test-workflow', got %q", wf.Name)
	}
	if wf.Trigger.Event != "on-create" {
		t.Errorf("expected event 'on-create', got %q", wf.Trigger.Event)
	}
	if wf.Trigger.EntityType != "issue" {
		t.Errorf("expected entity_type 'issue', got %q", wf.Trigger.EntityType)
	}
	if len(wf.Trigger.Conditions) != 1 {
		t.Errorf("expected 1 condition, got %d", len(wf.Trigger.Conditions))
	}
	if len(wf.Actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(wf.Actions))
	}
	if wf.Actions[0].Type != "add-comment" {
		t.Errorf("expected action type 'add-comment', got %q", wf.Actions[0].Type)
	}
}

func TestWorkflowEngine_HasChangedToConditions(t *testing.T) {
	engine := NewWorkflowEngine()
	engine.Workflows = []Workflow{
		{
			Trigger: Trigger{
				EntityType: "issue",
				Conditions: []Condition{
					{Field: "state", Operator: "changed_to", Value: "done"},
				},
			},
		},
	}

	if !engine.HasChangedToConditions("issue") {
		t.Error("should detect changed_to condition for issue")
	}
	if engine.HasChangedToConditions("project") {
		t.Error("should not detect changed_to for project")
	}
}

func TestInterpolateTemplate(t *testing.T) {
	fields := map[string]string{
		"identifier": "ENG-123",
		"title":      "Fix login",
	}

	result := interpolateTemplate("Issue {{identifier}} - {{title}} done", fields)
	expected := "Issue ENG-123 - Fix login done"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}

	// No placeholders
	result = interpolateTemplate("Plain text", fields)
	if result != "Plain text" {
		t.Errorf("expected plain text unchanged, got %q", result)
	}
}

func TestWorkflowEngine_EvaluateConditions(t *testing.T) {
	engine := NewWorkflowEngine()

	// All conditions must match (AND logic)
	conditions := []Condition{
		{Field: "priority", Operator: "eq", Value: "1"},
		{Field: "team_key", Operator: "eq", Value: "ENG"},
	}

	fields := map[string]string{"priority": "1", "team_key": "ENG"}
	if !engine.evaluateConditions(conditions, fields, nil) {
		t.Error("all conditions match, should return true")
	}

	fields["team_key"] = "OPS"
	if engine.evaluateConditions(conditions, fields, nil) {
		t.Error("one condition fails, should return false")
	}

	// Empty conditions = always match
	if !engine.evaluateConditions(nil, fields, nil) {
		t.Error("no conditions should always match")
	}
}
