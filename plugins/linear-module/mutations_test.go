package main

import (
	"strings"
	"testing"
)

func TestBuildIssueCreateMutation(t *testing.T) {
	tests := []struct {
		name    string
		fields  map[string]string
		wantErr string
		check   func(t *testing.T, query string)
	}{
		{
			name:    "missing title",
			fields:  map[string]string{"team_id": "t1"},
			wantErr: "title is required",
		},
		{
			name:    "missing team_id",
			fields:  map[string]string{"title": "Test"},
			wantErr: "team_id is required",
		},
		{
			name: "minimal required fields",
			fields: map[string]string{
				"title":   "Test Issue",
				"team_id": "team-123",
			},
			check: func(t *testing.T, query string) {
				if !strings.Contains(query, "issueCreate") {
					t.Error("expected issueCreate mutation")
				}
				if !strings.Contains(query, `title: "Test Issue"`) {
					t.Error("expected title field")
				}
				if !strings.Contains(query, `teamId: "team-123"`) {
					t.Error("expected teamId field")
				}
			},
		},
		{
			name: "all optional fields",
			fields: map[string]string{
				"title":       "Full Issue",
				"team_id":     "team-123",
				"description": "A description",
				"priority":    "2",
				"assignee_id": "user-456",
				"state_id":    "state-789",
				"project_id":  "proj-abc",
				"label_ids":   "label-1,label-2",
			},
			check: func(t *testing.T, query string) {
				if !strings.Contains(query, `description: "A description"`) {
					t.Error("expected description field")
				}
				if !strings.Contains(query, `priority: 2`) {
					t.Error("expected priority field")
				}
				if !strings.Contains(query, `assigneeId: "user-456"`) {
					t.Error("expected assigneeId field")
				}
				if !strings.Contains(query, `stateId: "state-789"`) {
					t.Error("expected stateId field")
				}
				if !strings.Contains(query, `projectId: "proj-abc"`) {
					t.Error("expected projectId field")
				}
				if !strings.Contains(query, `labelIds: ["label-1", "label-2"]`) {
					t.Error("expected labelIds field")
				}
			},
		},
		{
			name: "optional fields omitted when empty",
			fields: map[string]string{
				"title":   "Simple",
				"team_id": "t1",
			},
			check: func(t *testing.T, query string) {
				// The input section should not contain description or assigneeId.
				// "description:" appears in the input, while "description" appears in response selection.
				inputSection := query[strings.Index(query, "input:"):]
				closingBrace := strings.Index(inputSection, "})")
				if closingBrace > 0 {
					inputOnly := inputSection[:closingBrace]
					if strings.Contains(inputOnly, "description:") {
						t.Error("description should be omitted from input when empty")
					}
					if strings.Contains(inputOnly, "assigneeId:") {
						t.Error("assigneeId should be omitted from input when empty")
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildIssueCreateMutation(tt.fields)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			query, _ := result["query"].(string)
			if tt.check != nil {
				tt.check(t, query)
			}
		})
	}
}

func TestBuildIssueUpdateMutation(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		fields  map[string]string
		wantErr string
		check   func(t *testing.T, query string)
	}{
		{
			name:    "missing id",
			id:      "",
			fields:  map[string]string{"title": "New"},
			wantErr: "id is required",
		},
		{
			name:    "no fields to update",
			id:      "issue-1",
			fields:  map[string]string{},
			wantErr: "at least one field",
		},
		{
			name:   "partial update title",
			id:     "issue-1",
			fields: map[string]string{"title": "Updated Title"},
			check: func(t *testing.T, query string) {
				if !strings.Contains(query, "issueUpdate") {
					t.Error("expected issueUpdate mutation")
				}
				if !strings.Contains(query, `id: "issue-1"`) {
					t.Error("expected issue ID in mutation")
				}
				if !strings.Contains(query, `title: "Updated Title"`) {
					t.Error("expected title in input")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildIssueUpdateMutation(tt.id, tt.fields)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			query, _ := result["query"].(string)
			if tt.check != nil {
				tt.check(t, query)
			}
		})
	}
}

func TestBuildIssueDeleteMutation(t *testing.T) {
	_, err := buildIssueDeleteMutation("")
	if err == nil || !strings.Contains(err.Error(), "id is required") {
		t.Errorf("expected error for empty id, got %v", err)
	}

	result, err := buildIssueDeleteMutation("issue-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	query, _ := result["query"].(string)
	if !strings.Contains(query, "issueArchive") {
		t.Error("expected issueArchive mutation")
	}
}

func TestBuildIssueReadQuery(t *testing.T) {
	_, err := buildIssueReadQuery("")
	if err == nil || !strings.Contains(err.Error(), "id is required") {
		t.Errorf("expected error for empty id, got %v", err)
	}

	result, err := buildIssueReadQuery("issue-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	query, _ := result["query"].(string)
	if !strings.Contains(query, `issue(id: "issue-1")`) {
		t.Error("expected issue query with ID")
	}
}

func TestBuildIssueListQuery(t *testing.T) {
	// Default limit
	result := buildIssueListQuery(nil, 0, "")
	query, _ := result["query"].(string)
	if !strings.Contains(query, "issues(first: 50") {
		t.Error("expected default limit of 50")
	}

	// With filters
	filters := map[string]string{
		"team":  "ENG",
		"state": "In Progress",
	}
	result = buildIssueListQuery(filters, 10, "cursor-123")
	query, _ = result["query"].(string)
	if !strings.Contains(query, "first: 10") {
		t.Error("expected limit of 10")
	}
	if !strings.Contains(query, `after: "cursor-123"`) {
		t.Error("expected cursor")
	}
	if !strings.Contains(query, `team: { key: { eq: "ENG" } }`) {
		t.Error("expected team filter")
	}
}

func TestBuildProjectCreateMutation(t *testing.T) {
	_, err := buildProjectCreateMutation(map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected name required error, got %v", err)
	}

	result, err := buildProjectCreateMutation(map[string]string{
		"name":        "My Project",
		"description": "A new project",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	query, _ := result["query"].(string)
	if !strings.Contains(query, "projectCreate") {
		t.Error("expected projectCreate mutation")
	}
}

func TestBuildCommentCreateMutation(t *testing.T) {
	_, err := buildCommentCreateMutation(map[string]string{"body": "hi"})
	if err == nil || !strings.Contains(err.Error(), "issue_id is required") {
		t.Errorf("expected issue_id required error, got %v", err)
	}

	_, err = buildCommentCreateMutation(map[string]string{"issue_id": "iss-1"})
	if err == nil || !strings.Contains(err.Error(), "body is required") {
		t.Errorf("expected body required error, got %v", err)
	}

	result, err := buildCommentCreateMutation(map[string]string{
		"issue_id": "iss-1",
		"body":     "Great work!",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	query, _ := result["query"].(string)
	if !strings.Contains(query, "commentCreate") {
		t.Error("expected commentCreate mutation")
	}
}

func TestParseIssueFields(t *testing.T) {
	node := map[string]interface{}{
		"id":         "id-1",
		"identifier": "ENG-123",
		"title":      "Test Issue",
		"priority":   float64(2),
		"createdAt":  "2024-01-01T00:00:00Z",
		"updatedAt":  "2024-01-02T00:00:00Z",
		"state": map[string]interface{}{
			"id":   "state-1",
			"name": "In Progress",
			"type": "started",
		},
		"team": map[string]interface{}{
			"id":   "team-1",
			"key":  "ENG",
			"name": "Engineering",
		},
	}

	fields := parseIssueFields(node)

	if fields["id"] != "id-1" {
		t.Errorf("expected id=id-1, got %s", fields["id"])
	}
	if fields["identifier"] != "ENG-123" {
		t.Errorf("expected identifier=ENG-123, got %s", fields["identifier"])
	}
	if fields["state"] != "In Progress" {
		t.Errorf("expected state=In Progress, got %s", fields["state"])
	}
	if fields["team_key"] != "ENG" {
		t.Errorf("expected team_key=ENG, got %s", fields["team_key"])
	}
	if fields["priority"] != "2" {
		t.Errorf("expected priority=2, got %s", fields["priority"])
	}
}

func TestParseProjectFields(t *testing.T) {
	node := map[string]interface{}{
		"id":        "proj-1",
		"name":      "Project X",
		"state":     "started",
		"progress":  float64(0.45),
		"createdAt": "2024-01-01T00:00:00Z",
		"updatedAt": "2024-01-02T00:00:00Z",
	}

	fields := parseProjectFields(node)

	if fields["id"] != "proj-1" {
		t.Errorf("expected id=proj-1, got %s", fields["id"])
	}
	if fields["name"] != "Project X" {
		t.Errorf("expected name=Project X, got %s", fields["name"])
	}
	if fields["progress"] != "0.45" {
		t.Errorf("expected progress=0.45, got %s", fields["progress"])
	}
}
