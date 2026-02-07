package conversation

import (
	"testing"
)

func TestConstraintDetector_MustPattern(t *testing.T) {
	d := NewConstraintDetector(0.6)
	results := d.Detect("We must always validate user input before storing", ObsTypeDecision)
	if len(results) == 0 {
		t.Fatal("expected at least 1 constraint, got 0")
	}
	found := false
	for _, r := range results {
		t.Logf("type=%s confidence=%.2f name=%s", r.ConstraintType, r.Confidence, r.Name)
		if r.ConstraintType == "must" {
			found = true
		}
	}
	if !found {
		t.Error("expected a 'must' constraint type")
	}
}

func TestConstraintDetector_MustNotPattern(t *testing.T) {
	d := NewConstraintDetector(0.6)
	results := d.Detect("We must never use raw SQL queries directly", ObsTypeDecision)
	if len(results) == 0 {
		t.Fatal("expected at least 1 constraint, got 0")
	}
	types := map[string]bool{}
	for _, r := range results {
		t.Logf("type=%s confidence=%.2f", r.ConstraintType, r.Confidence)
		types[r.ConstraintType] = true
	}
	if !types["must_not"] && !types["must"] {
		t.Error("expected 'must_not' or 'must' constraint type")
	}
}

func TestConstraintDetector_ShouldPattern(t *testing.T) {
	d := NewConstraintDetector(0.4) // Lower threshold for should
	results := d.Detect("We should prefer using interfaces over concrete types", ObsTypeLearning)
	if len(results) == 0 {
		t.Fatal("expected at least 1 constraint, got 0")
	}
}

func TestConstraintDetector_NoMatch(t *testing.T) {
	d := NewConstraintDetector(0.6)
	results := d.Detect("The weather is nice today", ObsTypeProgress)
	if len(results) != 0 {
		t.Errorf("expected 0 constraints for generic text, got %d", len(results))
	}
}

func TestConstraintDetector_BoostFromDecision(t *testing.T) {
	d := NewConstraintDetector(0.8) // High threshold
	// "must" base=0.7, +0.2 decision boost = 0.9 >= 0.8
	results := d.Detect("We must validate all inputs", ObsTypeDecision)
	if len(results) == 0 {
		t.Fatal("expected decision boost to push confidence above 0.8")
	}
	// Same content without boost
	results2 := d.Detect("We must validate all inputs", ObsTypeProgress)
	// "must" base=0.7 + 0 boost = 0.7 < 0.8
	if len(results2) != 0 {
		t.Error("expected no results without decision boost at 0.8 threshold")
	}
}
