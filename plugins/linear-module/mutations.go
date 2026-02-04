package main

import (
	"fmt"
	"strings"
)

// buildIssueCreateMutation builds a GraphQL mutation for creating an issue.
func buildIssueCreateMutation(fields map[string]string) (map[string]interface{}, error) {
	title := fields["title"]
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}
	teamID := fields["team_id"]
	if teamID == "" {
		return nil, fmt.Errorf("team_id is required")
	}

	var inputParts []string
	inputParts = append(inputParts, fmt.Sprintf(`title: %q`, title))
	inputParts = append(inputParts, fmt.Sprintf(`teamId: %q`, teamID))

	if v := fields["description"]; v != "" {
		inputParts = append(inputParts, fmt.Sprintf(`description: %q`, v))
	}
	if v := fields["priority"]; v != "" {
		inputParts = append(inputParts, fmt.Sprintf(`priority: %s`, v))
	}
	if v := fields["assignee_id"]; v != "" {
		inputParts = append(inputParts, fmt.Sprintf(`assigneeId: %q`, v))
	}
	if v := fields["state_id"]; v != "" {
		inputParts = append(inputParts, fmt.Sprintf(`stateId: %q`, v))
	}
	if v := fields["project_id"]; v != "" {
		inputParts = append(inputParts, fmt.Sprintf(`projectId: %q`, v))
	}
	if v := fields["label_ids"]; v != "" {
		// label_ids is a comma-separated list
		ids := strings.Split(v, ",")
		var quoted []string
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id != "" {
				quoted = append(quoted, fmt.Sprintf("%q", id))
			}
		}
		if len(quoted) > 0 {
			inputParts = append(inputParts, fmt.Sprintf(`labelIds: [%s]`, strings.Join(quoted, ", ")))
		}
	}

	query := fmt.Sprintf(`mutation {
		issueCreate(input: { %s }) {
			success
			issue {
				id
				identifier
				title
				description
				priority
				state { id name type }
				team { id key name }
				project { id name }
				assignee { id name email }
				labels { nodes { id name } }
				createdAt
				updatedAt
			}
		}
	}`, strings.Join(inputParts, ", "))

	return map[string]interface{}{"query": query}, nil
}

// buildIssueUpdateMutation builds a GraphQL mutation for updating an issue.
func buildIssueUpdateMutation(id string, fields map[string]string) (map[string]interface{}, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}

	var inputParts []string
	if v := fields["title"]; v != "" {
		inputParts = append(inputParts, fmt.Sprintf(`title: %q`, v))
	}
	if v := fields["description"]; v != "" {
		inputParts = append(inputParts, fmt.Sprintf(`description: %q`, v))
	}
	if v := fields["priority"]; v != "" {
		inputParts = append(inputParts, fmt.Sprintf(`priority: %s`, v))
	}
	if v := fields["assignee_id"]; v != "" {
		inputParts = append(inputParts, fmt.Sprintf(`assigneeId: %q`, v))
	}
	if v := fields["state_id"]; v != "" {
		inputParts = append(inputParts, fmt.Sprintf(`stateId: %q`, v))
	}
	if v := fields["project_id"]; v != "" {
		inputParts = append(inputParts, fmt.Sprintf(`projectId: %q`, v))
	}
	if v := fields["label_ids"]; v != "" {
		ids := strings.Split(v, ",")
		var quoted []string
		for _, lid := range ids {
			lid = strings.TrimSpace(lid)
			if lid != "" {
				quoted = append(quoted, fmt.Sprintf("%q", lid))
			}
		}
		if len(quoted) > 0 {
			inputParts = append(inputParts, fmt.Sprintf(`labelIds: [%s]`, strings.Join(quoted, ", ")))
		}
	}

	if len(inputParts) == 0 {
		return nil, fmt.Errorf("at least one field to update is required")
	}

	query := fmt.Sprintf(`mutation {
		issueUpdate(id: %q, input: { %s }) {
			success
			issue {
				id
				identifier
				title
				description
				priority
				state { id name type }
				team { id key name }
				project { id name }
				assignee { id name email }
				labels { nodes { id name } }
				createdAt
				updatedAt
			}
		}
	}`, id, strings.Join(inputParts, ", "))

	return map[string]interface{}{"query": query}, nil
}

// buildIssueDeleteMutation builds a GraphQL mutation to archive/delete an issue.
func buildIssueDeleteMutation(id string) (map[string]interface{}, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}

	query := fmt.Sprintf(`mutation {
		issueArchive(id: %q) {
			success
		}
	}`, id)

	return map[string]interface{}{"query": query}, nil
}

// buildIssueReadQuery builds a GraphQL query to read a single issue.
func buildIssueReadQuery(id string) (map[string]interface{}, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}

	query := fmt.Sprintf(`query {
		issue(id: %q) {
			id
			identifier
			title
			description
			priority
			state { id name type }
			team { id key name }
			project { id name }
			assignee { id name email }
			labels { nodes { id name } }
			createdAt
			updatedAt
			completedAt
		}
	}`, id)

	return map[string]interface{}{"query": query}, nil
}

// buildIssueListQuery builds a GraphQL query to list issues with filters.
func buildIssueListQuery(filters map[string]string, limit int, cursor string) map[string]interface{} {
	if limit <= 0 {
		limit = 50
	}

	afterClause := ""
	if cursor != "" {
		afterClause = fmt.Sprintf(`, after: %q`, cursor)
	}

	var filterParts []string
	if v := filters["team"]; v != "" {
		filterParts = append(filterParts, fmt.Sprintf(`team: { key: { eq: %q } }`, v))
	}
	if v := filters["state"]; v != "" {
		filterParts = append(filterParts, fmt.Sprintf(`state: { name: { eq: %q } }`, v))
	}
	if v := filters["assignee"]; v != "" {
		filterParts = append(filterParts, fmt.Sprintf(`assignee: { name: { eq: %q } }`, v))
	}
	if v := filters["project"]; v != "" {
		filterParts = append(filterParts, fmt.Sprintf(`project: { name: { eq: %q } }`, v))
	}
	if v := filters["query"]; v != "" {
		// Linear supports fulltext search via the filter
		filterParts = append(filterParts, fmt.Sprintf(`searchableContent: { contains: %q }`, v))
	}

	filterClause := ""
	if len(filterParts) > 0 {
		filterClause = fmt.Sprintf(`, filter: { %s }`, strings.Join(filterParts, ", "))
	}

	query := fmt.Sprintf(`query {
		issues(first: %d%s%s) {
			pageInfo {
				hasNextPage
				endCursor
			}
			nodes {
				id
				identifier
				title
				description
				priority
				state { id name type }
				team { id key name }
				project { id name }
				assignee { id name email }
				labels { nodes { id name } }
				createdAt
				updatedAt
				completedAt
			}
		}
	}`, limit, afterClause, filterClause)

	return map[string]interface{}{"query": query}
}

// buildProjectCreateMutation builds a GraphQL mutation for creating a project.
func buildProjectCreateMutation(fields map[string]string) (map[string]interface{}, error) {
	name := fields["name"]
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	var inputParts []string
	inputParts = append(inputParts, fmt.Sprintf(`name: %q`, name))

	if v := fields["description"]; v != "" {
		inputParts = append(inputParts, fmt.Sprintf(`description: %q`, v))
	}
	if v := fields["team_ids"]; v != "" {
		ids := strings.Split(v, ",")
		var quoted []string
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id != "" {
				quoted = append(quoted, fmt.Sprintf("%q", id))
			}
		}
		if len(quoted) > 0 {
			inputParts = append(inputParts, fmt.Sprintf(`teamIds: [%s]`, strings.Join(quoted, ", ")))
		}
	}

	query := fmt.Sprintf(`mutation {
		projectCreate(input: { %s }) {
			success
			project {
				id
				name
				description
				state
				progress
				startDate
				targetDate
				teams { nodes { id key name } }
				lead { id name email }
				createdAt
				updatedAt
			}
		}
	}`, strings.Join(inputParts, ", "))

	return map[string]interface{}{"query": query}, nil
}

// buildProjectUpdateMutation builds a GraphQL mutation for updating a project.
func buildProjectUpdateMutation(id string, fields map[string]string) (map[string]interface{}, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}

	var inputParts []string
	if v := fields["name"]; v != "" {
		inputParts = append(inputParts, fmt.Sprintf(`name: %q`, v))
	}
	if v := fields["description"]; v != "" {
		inputParts = append(inputParts, fmt.Sprintf(`description: %q`, v))
	}
	if v := fields["state"]; v != "" {
		inputParts = append(inputParts, fmt.Sprintf(`state: %q`, v))
	}

	if len(inputParts) == 0 {
		return nil, fmt.Errorf("at least one field to update is required")
	}

	query := fmt.Sprintf(`mutation {
		projectUpdate(id: %q, input: { %s }) {
			success
			project {
				id
				name
				description
				state
				progress
				startDate
				targetDate
				teams { nodes { id key name } }
				lead { id name email }
				createdAt
				updatedAt
			}
		}
	}`, id, strings.Join(inputParts, ", "))

	return map[string]interface{}{"query": query}, nil
}

// buildProjectReadQuery builds a GraphQL query to read a single project.
func buildProjectReadQuery(id string) (map[string]interface{}, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}

	query := fmt.Sprintf(`query {
		project(id: %q) {
			id
			name
			description
			state
			progress
			startDate
			targetDate
			teams { nodes { id key name } }
			lead { id name email }
			createdAt
			updatedAt
		}
	}`, id)

	return map[string]interface{}{"query": query}, nil
}

// buildProjectListQuery builds a GraphQL query to list projects.
func buildProjectListQuery(limit int, cursor string) map[string]interface{} {
	if limit <= 0 {
		limit = 50
	}

	afterClause := ""
	if cursor != "" {
		afterClause = fmt.Sprintf(`, after: %q`, cursor)
	}

	query := fmt.Sprintf(`query {
		projects(first: %d%s) {
			pageInfo {
				hasNextPage
				endCursor
			}
			nodes {
				id
				name
				description
				state
				progress
				startDate
				targetDate
				teams { nodes { id key name } }
				lead { id name email }
				createdAt
				updatedAt
			}
		}
	}`, limit, afterClause)

	return map[string]interface{}{"query": query}
}

// buildCommentCreateMutation builds a GraphQL mutation for creating a comment.
func buildCommentCreateMutation(fields map[string]string) (map[string]interface{}, error) {
	issueID := fields["issue_id"]
	if issueID == "" {
		return nil, fmt.Errorf("issue_id is required")
	}
	body := fields["body"]
	if body == "" {
		return nil, fmt.Errorf("body is required")
	}

	query := fmt.Sprintf(`mutation {
		commentCreate(input: { issueId: %q, body: %q }) {
			success
			comment {
				id
				body
				createdAt
				updatedAt
				user { id name email }
				issue { id identifier }
			}
		}
	}`, issueID, body)

	return map[string]interface{}{"query": query}, nil
}

// parseIssueFields extracts fields from a GraphQL issue response node.
func parseIssueFields(node map[string]interface{}) map[string]string {
	fields := map[string]string{
		"id":         getString(node, "id"),
		"identifier": getString(node, "identifier"),
		"title":      getString(node, "title"),
		"priority":   fmt.Sprintf("%d", getInt(node, "priority")),
		"created_at": getString(node, "createdAt"),
		"updated_at": getString(node, "updatedAt"),
	}

	if desc := getString(node, "description"); desc != "" {
		fields["description"] = desc
	}
	if completedAt := getString(node, "completedAt"); completedAt != "" {
		fields["completed_at"] = completedAt
	}

	if state, ok := node["state"].(map[string]interface{}); ok {
		fields["state_id"] = getString(state, "id")
		fields["state"] = getString(state, "name")
		fields["state_type"] = getString(state, "type")
	}
	if team, ok := node["team"].(map[string]interface{}); ok {
		fields["team_id"] = getString(team, "id")
		fields["team_key"] = getString(team, "key")
		fields["team_name"] = getString(team, "name")
	}
	if project, ok := node["project"].(map[string]interface{}); ok && project != nil {
		fields["project_id"] = getString(project, "id")
		fields["project_name"] = getString(project, "name")
	}
	if assignee, ok := node["assignee"].(map[string]interface{}); ok && assignee != nil {
		fields["assignee_id"] = getString(assignee, "id")
		fields["assignee_name"] = getString(assignee, "name")
		fields["assignee_email"] = getString(assignee, "email")
	}
	if labels, ok := node["labels"].(map[string]interface{}); ok {
		if labelNodes, ok := labels["nodes"].([]interface{}); ok {
			var names []string
			var ids []string
			for _, ln := range labelNodes {
				if lnMap, ok := ln.(map[string]interface{}); ok {
					names = append(names, getString(lnMap, "name"))
					ids = append(ids, getString(lnMap, "id"))
				}
			}
			if len(names) > 0 {
				fields["labels"] = strings.Join(names, ",")
				fields["label_ids"] = strings.Join(ids, ",")
			}
		}
	}

	return fields
}

// parseProjectFields extracts fields from a GraphQL project response node.
func parseProjectFields(node map[string]interface{}) map[string]string {
	fields := map[string]string{
		"id":         getString(node, "id"),
		"name":       getString(node, "name"),
		"state":      getString(node, "state"),
		"progress":   fmt.Sprintf("%.2f", getFloat(node, "progress")),
		"created_at": getString(node, "createdAt"),
		"updated_at": getString(node, "updatedAt"),
	}

	if desc := getString(node, "description"); desc != "" {
		fields["description"] = desc
	}
	if startDate := getString(node, "startDate"); startDate != "" {
		fields["start_date"] = startDate
	}
	if targetDate := getString(node, "targetDate"); targetDate != "" {
		fields["target_date"] = targetDate
	}

	if lead, ok := node["lead"].(map[string]interface{}); ok && lead != nil {
		fields["lead_id"] = getString(lead, "id")
		fields["lead_name"] = getString(lead, "name")
		fields["lead_email"] = getString(lead, "email")
	}
	if teams, ok := node["teams"].(map[string]interface{}); ok {
		if teamNodes, ok := teams["nodes"].([]interface{}); ok {
			var keys []string
			var ids []string
			for _, tn := range teamNodes {
				if tnMap, ok := tn.(map[string]interface{}); ok {
					keys = append(keys, getString(tnMap, "key"))
					ids = append(ids, getString(tnMap, "id"))
				}
			}
			if len(keys) > 0 {
				fields["team_keys"] = strings.Join(keys, ",")
				fields["team_ids"] = strings.Join(ids, ",")
			}
		}
	}

	return fields
}

// parseCommentFields extracts fields from a GraphQL comment response node.
func parseCommentFields(node map[string]interface{}) map[string]string {
	fields := map[string]string{
		"id":         getString(node, "id"),
		"body":       getString(node, "body"),
		"created_at": getString(node, "createdAt"),
		"updated_at": getString(node, "updatedAt"),
	}

	if user, ok := node["user"].(map[string]interface{}); ok && user != nil {
		fields["user_id"] = getString(user, "id")
		fields["user_name"] = getString(user, "name")
		fields["user_email"] = getString(user, "email")
	}
	if issue, ok := node["issue"].(map[string]interface{}); ok && issue != nil {
		fields["issue_id"] = getString(issue, "id")
		fields["issue_identifier"] = getString(issue, "identifier")
	}

	return fields
}
