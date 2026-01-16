package validation

import (
	"testing"
)

// Test structs for validation tests
type testEmbeddingStruct struct {
	Embedding []float32 `json:"embedding" validate:"omitempty,embedding_dims"`
}

type testRequiredEmbeddingStruct struct {
	Embedding []float32 `json:"embedding" validate:"required,embedding_dims"`
}

type testRequiredStruct struct {
	SpaceID string `json:"space_id" validate:"required,min=1,max=256"`
}

type testMinMaxStruct struct {
	Name string `json:"name" validate:"min=3,max=10"`
	Age  int    `json:"age" validate:"min=1,max=100"`
}

type testOneOfStruct struct {
	Sensitivity string `json:"sensitivity" validate:"oneof=public internal confidential"`
}

type testSliceStruct struct {
	Tags []string `json:"tags" validate:"min=1,max=5,dive,min=1"`
}

type testRequiredWithoutStruct struct {
	QueryText      string    `json:"query_text" validate:"required_without=QueryEmbedding,omitempty,min=1"`
	QueryEmbedding []float32 `json:"query_embedding" validate:"required_without=QueryText,omitempty,embedding_dims"`
}

// TestGetValidator_Singleton tests that GetValidator returns the same instance
func TestGetValidator_Singleton(t *testing.T) {
	v1 := GetValidator()
	v2 := GetValidator()

	if v1 != v2 {
		t.Error("GetValidator() should return the same instance (singleton)")
	}

	if v1 == nil {
		t.Error("GetValidator() should not return nil")
	}
}

// TestValidateEmbeddingDims tests the embedding_dims custom validator
func TestValidateEmbeddingDims(t *testing.T) {
	tests := []struct {
		name        string
		embedding   []float32
		expectError bool
	}{
		{
			name:        "nil embedding passes (let required handle it)",
			embedding:   nil,
			expectError: false,
		},
		{
			name:        "empty embedding fails",
			embedding:   []float32{},
			expectError: true,
		},
		{
			name:        "768 dimensions passes",
			embedding:   make([]float32, 768),
			expectError: false,
		},
		{
			name:        "1536 dimensions passes",
			embedding:   make([]float32, 1536),
			expectError: false,
		},
		{
			name:        "wrong dimensions (512) fails",
			embedding:   make([]float32, 512),
			expectError: true,
		},
		{
			name:        "wrong dimensions (100) fails",
			embedding:   make([]float32, 100),
			expectError: true,
		},
		{
			name:        "wrong dimensions (3072) fails",
			embedding:   make([]float32, 3072),
			expectError: true,
		},
		{
			name:        "wrong dimensions (1) fails",
			embedding:   make([]float32, 1),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testEmbeddingStruct{Embedding: tt.embedding}
			err := Validate(s)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// TestValidateEmbeddingDims_Required tests embedding_dims with required validation
func TestValidateEmbeddingDims_Required(t *testing.T) {
	tests := []struct {
		name        string
		embedding   []float32
		expectError bool
	}{
		{
			name:        "nil embedding fails when required",
			embedding:   nil,
			expectError: true,
		},
		{
			name:        "768 dimensions passes",
			embedding:   make([]float32, 768),
			expectError: false,
		},
		{
			name:        "1536 dimensions passes",
			embedding:   make([]float32, 1536),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testRequiredEmbeddingStruct{Embedding: tt.embedding}
			err := Validate(s)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// TestValidate_Required tests required field validation
func TestValidate_Required(t *testing.T) {
	tests := []struct {
		name        string
		spaceID     string
		expectError bool
	}{
		{
			name:        "valid space_id",
			spaceID:     "test-space",
			expectError: false,
		},
		{
			name:        "empty space_id fails",
			spaceID:     "",
			expectError: true,
		},
		{
			name:        "single character passes min",
			spaceID:     "a",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testRequiredStruct{SpaceID: tt.spaceID}
			err := Validate(s)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// TestValidate_MinMax tests min/max validation for strings and numbers
func TestValidate_MinMax(t *testing.T) {
	tests := []struct {
		name        string
		nameField   string
		age         int
		expectError bool
	}{
		{
			name:        "valid name and age",
			nameField:   "John",
			age:         25,
			expectError: false,
		},
		{
			name:        "name too short",
			nameField:   "Jo",
			age:         25,
			expectError: true,
		},
		{
			name:        "name too long",
			nameField:   "JohnJacobson",
			age:         25,
			expectError: true,
		},
		{
			name:        "age too low",
			nameField:   "John",
			age:         0,
			expectError: true,
		},
		{
			name:        "age too high",
			nameField:   "John",
			age:         101,
			expectError: true,
		},
		{
			name:        "boundary: name exactly min",
			nameField:   "Jon",
			age:         50,
			expectError: false,
		},
		{
			name:        "boundary: name exactly max",
			nameField:   "JohnJacobs",
			age:         50,
			expectError: false,
		},
		{
			name:        "boundary: age exactly min",
			nameField:   "John",
			age:         1,
			expectError: false,
		},
		{
			name:        "boundary: age exactly max",
			nameField:   "John",
			age:         100,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testMinMaxStruct{Name: tt.nameField, Age: tt.age}
			err := Validate(s)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// TestValidate_OneOf tests oneof validation
func TestValidate_OneOf(t *testing.T) {
	tests := []struct {
		name        string
		sensitivity string
		expectError bool
	}{
		{
			name:        "public is valid",
			sensitivity: "public",
			expectError: false,
		},
		{
			name:        "internal is valid",
			sensitivity: "internal",
			expectError: false,
		},
		{
			name:        "confidential is valid",
			sensitivity: "confidential",
			expectError: false,
		},
		{
			name:        "invalid value fails",
			sensitivity: "secret",
			expectError: true,
		},
		{
			name:        "empty string fails",
			sensitivity: "",
			expectError: true,
		},
		{
			name:        "case sensitive - Public fails",
			sensitivity: "Public",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testOneOfStruct{Sensitivity: tt.sensitivity}
			err := Validate(s)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// TestValidate_SliceValidation tests slice validation with dive
func TestValidate_SliceValidation(t *testing.T) {
	tests := []struct {
		name        string
		tags        []string
		expectError bool
	}{
		{
			name:        "valid tags",
			tags:        []string{"tag1", "tag2"},
			expectError: false,
		},
		{
			name:        "empty slice fails (min=1)",
			tags:        []string{},
			expectError: true,
		},
		{
			name:        "too many tags fails (max=5)",
			tags:        []string{"a", "b", "c", "d", "e", "f"},
			expectError: true,
		},
		{
			name:        "empty string in slice fails (dive,min=1)",
			tags:        []string{"valid", ""},
			expectError: true,
		},
		{
			name:        "single valid tag passes",
			tags:        []string{"single"},
			expectError: false,
		},
		{
			name:        "exactly 5 tags passes",
			tags:        []string{"a", "b", "c", "d", "e"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testSliceStruct{Tags: tt.tags}
			err := Validate(s)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// TestValidate_RequiredWithout tests required_without validation
func TestValidate_RequiredWithout(t *testing.T) {
	tests := []struct {
		name           string
		queryText      string
		queryEmbedding []float32
		expectError    bool
	}{
		{
			name:           "query_text only passes",
			queryText:      "search query",
			queryEmbedding: nil,
			expectError:    false,
		},
		{
			name:           "query_embedding only passes",
			queryText:      "",
			queryEmbedding: make([]float32, 1536),
			expectError:    false,
		},
		{
			name:           "both provided passes",
			queryText:      "search query",
			queryEmbedding: make([]float32, 768),
			expectError:    false,
		},
		{
			name:           "neither provided fails",
			queryText:      "",
			queryEmbedding: nil,
			expectError:    true,
		},
		{
			name:           "embedding with wrong dimensions fails",
			queryText:      "",
			queryEmbedding: make([]float32, 512),
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testRequiredWithoutStruct{
				QueryText:      tt.queryText,
				QueryEmbedding: tt.queryEmbedding,
			}
			err := Validate(s)

			if tt.expectError && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// TestFormatValidationErrors tests error formatting
func TestFormatValidationErrors(t *testing.T) {
	tests := []struct {
		name           string
		input          any
		expectError    string
		expectMinItems int
	}{
		{
			name:           "required field missing",
			input:          testRequiredStruct{SpaceID: ""},
			expectError:    "validation_failed",
			expectMinItems: 1,
		},
		{
			name:           "multiple errors",
			input:          testMinMaxStruct{Name: "Jo", Age: 0},
			expectError:    "validation_failed",
			expectMinItems: 2,
		},
		{
			name:           "embedding_dims error",
			input:          testEmbeddingStruct{Embedding: make([]float32, 100)},
			expectError:    "validation_failed",
			expectMinItems: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.input)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}

			resp := FormatValidationErrors(err)

			if resp.Error != tt.expectError {
				t.Errorf("Error = %q, want %q", resp.Error, tt.expectError)
			}

			if len(resp.Details) < tt.expectMinItems {
				t.Errorf("Details count = %d, want at least %d", len(resp.Details), tt.expectMinItems)
			}
		})
	}
}

// TestFormatValidationErrors_NonValidationError tests handling of non-validation errors
func TestFormatValidationErrors_NonValidationError(t *testing.T) {
	// Create a simple error that's not a ValidationErrors type
	err := Validate(nil)
	if err == nil {
		// nil passes validation for non-pointer types
		// Use a different approach - just test with a generic error
		resp := FormatValidationErrors(nil)
		if resp.Error != "validation_failed" {
			t.Errorf("Error = %q, want %q", resp.Error, "validation_failed")
		}
		if resp.Details != nil {
			t.Errorf("Details = %v, want nil", resp.Details)
		}
		return
	}

	resp := FormatValidationErrors(err)
	if resp.Error != "validation_failed" {
		t.Errorf("Error = %q, want %q", resp.Error, "validation_failed")
	}
}

// TestFormatValidationErrors_FieldNames tests that JSON field names are used
func TestFormatValidationErrors_FieldNames(t *testing.T) {
	s := testRequiredStruct{SpaceID: ""}
	err := Validate(s)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	resp := FormatValidationErrors(err)

	if len(resp.Details) == 0 {
		t.Fatal("expected at least one detail")
	}

	// Should use JSON name "space_id" not Go name "SpaceID"
	found := false
	for _, d := range resp.Details {
		if d.Field == "space_id" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected field 'space_id' in details, got: %v", resp.Details)
	}
}

// TestFormatValidationErrors_Messages tests that error messages are formatted correctly
func TestFormatValidationErrors_Messages(t *testing.T) {
	tests := []struct {
		name            string
		input           any
		expectedField   string
		expectedMessage string
	}{
		{
			name:            "required message",
			input:           testRequiredStruct{SpaceID: ""},
			expectedField:   "space_id",
			expectedMessage: "this field is required",
		},
		{
			name:            "embedding_dims message",
			input:           testEmbeddingStruct{Embedding: make([]float32, 100)},
			expectedField:   "embedding",
			expectedMessage: "must have 768 or 1536 dimensions",
		},
		{
			name:            "oneof message",
			input:           testOneOfStruct{Sensitivity: "invalid"},
			expectedField:   "sensitivity",
			expectedMessage: "must be one of: public internal confidential",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.input)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}

			resp := FormatValidationErrors(err)

			var found bool
			for _, d := range resp.Details {
				if d.Field == tt.expectedField {
					found = true
					if d.Message != tt.expectedMessage {
						t.Errorf("Message for %s = %q, want %q", tt.expectedField, d.Message, tt.expectedMessage)
					}
					break
				}
			}

			if !found {
				t.Errorf("field %q not found in details: %v", tt.expectedField, resp.Details)
			}
		})
	}
}

// TestValidationError_Struct tests ValidationError struct fields
func TestValidationError_Struct(t *testing.T) {
	ve := ValidationError{
		Field:   "test_field",
		Message: "test message",
	}

	if ve.Field != "test_field" {
		t.Errorf("Field = %q, want %q", ve.Field, "test_field")
	}
	if ve.Message != "test message" {
		t.Errorf("Message = %q, want %q", ve.Message, "test message")
	}
}

// TestValidationErrorResponse_Struct tests ValidationErrorResponse struct fields
func TestValidationErrorResponse_Struct(t *testing.T) {
	resp := ValidationErrorResponse{
		Error: "validation_failed",
		Details: []ValidationError{
			{Field: "field1", Message: "message1"},
			{Field: "field2", Message: "message2"},
		},
	}

	if resp.Error != "validation_failed" {
		t.Errorf("Error = %q, want %q", resp.Error, "validation_failed")
	}
	if len(resp.Details) != 2 {
		t.Errorf("Details count = %d, want 2", len(resp.Details))
	}
	if resp.Details[0].Field != "field1" {
		t.Errorf("Details[0].Field = %q, want %q", resp.Details[0].Field, "field1")
	}
}
