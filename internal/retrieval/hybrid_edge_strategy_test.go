package retrieval

import (
	"testing"

	"mdemg/internal/config"
)

// TestGetEdgeTypesForHop_AllStrategy tests the "all" strategy (original behavior)
func TestGetEdgeTypesForHop_AllStrategy(t *testing.T) {
	cfg := config.Config{
		EdgeTypeStrategy:           "all",
		AllowedRelationshipTypes:   []string{"ASSOCIATED_WITH", "GENERALIZES", "CO_ACTIVATED_WITH"},
		StructuralEdgeTypes:        []string{"ASSOCIATED_WITH", "GENERALIZES"},
		LearnedEdgeTypes:           []string{"CO_ACTIVATED_WITH"},
		HybridSwitchHop:            1,
		QueryAwareExpansionEnabled: true,
	}
	s := &Service{cfg: cfg}

	// Hop 0: should use all types with attention
	types, attention := s.getEdgeTypesForHop(0)
	if len(types) != 3 {
		t.Errorf("Hop 0: expected 3 edge types, got %d", len(types))
	}
	if !attention {
		t.Error("Hop 0: expected attention=true for 'all' strategy")
	}

	// Hop 1: should still use all types with attention
	types, attention = s.getEdgeTypesForHop(1)
	if len(types) != 3 {
		t.Errorf("Hop 1: expected 3 edge types, got %d", len(types))
	}
	if !attention {
		t.Error("Hop 1: expected attention=true for 'all' strategy")
	}
}

// TestGetEdgeTypesForHop_HybridStrategy tests the "hybrid" strategy (recommended)
func TestGetEdgeTypesForHop_HybridStrategy(t *testing.T) {
	cfg := config.Config{
		EdgeTypeStrategy:           "hybrid",
		AllowedRelationshipTypes:   []string{"ASSOCIATED_WITH", "GENERALIZES", "CO_ACTIVATED_WITH"},
		StructuralEdgeTypes:        []string{"ASSOCIATED_WITH", "GENERALIZES", "ABSTRACTS_TO"},
		LearnedEdgeTypes:           []string{"CO_ACTIVATED_WITH"},
		HybridSwitchHop:            1,
		QueryAwareExpansionEnabled: true,
	}
	s := &Service{cfg: cfg}

	// Hop 0: should use structural types WITHOUT attention
	types, attention := s.getEdgeTypesForHop(0)
	if len(types) != 3 {
		t.Errorf("Hop 0: expected 3 structural edge types, got %d", len(types))
	}
	if types[0] != "ASSOCIATED_WITH" {
		t.Errorf("Hop 0: expected ASSOCIATED_WITH first, got %s", types[0])
	}
	if attention {
		t.Error("Hop 0: expected attention=false for structural hops")
	}

	// Hop 1: should use learned types WITH attention
	types, attention = s.getEdgeTypesForHop(1)
	if len(types) != 1 {
		t.Errorf("Hop 1: expected 1 learned edge type, got %d", len(types))
	}
	if types[0] != "CO_ACTIVATED_WITH" {
		t.Errorf("Hop 1: expected CO_ACTIVATED_WITH, got %s", types[0])
	}
	if !attention {
		t.Error("Hop 1: expected attention=true for learned hops")
	}

	// Hop 2: should still use learned types WITH attention
	types, attention = s.getEdgeTypesForHop(2)
	if len(types) != 1 {
		t.Errorf("Hop 2: expected 1 learned edge type, got %d", len(types))
	}
	if !attention {
		t.Error("Hop 2: expected attention=true for learned hops")
	}
}

// TestGetEdgeTypesForHop_StructuralFirstStrategy tests the "structural_first" strategy
func TestGetEdgeTypesForHop_StructuralFirstStrategy(t *testing.T) {
	cfg := config.Config{
		EdgeTypeStrategy:           "structural_first",
		AllowedRelationshipTypes:   []string{"ASSOCIATED_WITH", "GENERALIZES", "CO_ACTIVATED_WITH"},
		StructuralEdgeTypes:        []string{"ASSOCIATED_WITH", "GENERALIZES"},
		LearnedEdgeTypes:           []string{"CO_ACTIVATED_WITH"},
		HybridSwitchHop:            1,
		QueryAwareExpansionEnabled: true,
	}
	s := &Service{cfg: cfg}

	// Hop 0: should use structural types WITHOUT attention
	types, attention := s.getEdgeTypesForHop(0)
	if len(types) != 2 {
		t.Errorf("Hop 0: expected 2 structural edge types, got %d", len(types))
	}
	if attention {
		t.Error("Hop 0: expected attention=false for structural hops")
	}

	// Hop 1: should use ALL types WITH attention (not just learned)
	types, attention = s.getEdgeTypesForHop(1)
	if len(types) != 3 {
		t.Errorf("Hop 1: expected 3 edge types (all), got %d", len(types))
	}
	if !attention {
		t.Error("Hop 1: expected attention=true for later hops")
	}
}

// TestGetEdgeTypesForHop_LearnedOnlyStrategy tests the "learned_only" strategy
func TestGetEdgeTypesForHop_LearnedOnlyStrategy(t *testing.T) {
	cfg := config.Config{
		EdgeTypeStrategy:           "learned_only",
		AllowedRelationshipTypes:   []string{"ASSOCIATED_WITH", "GENERALIZES", "CO_ACTIVATED_WITH"},
		StructuralEdgeTypes:        []string{"ASSOCIATED_WITH", "GENERALIZES"},
		LearnedEdgeTypes:           []string{"CO_ACTIVATED_WITH"},
		HybridSwitchHop:            1,
		QueryAwareExpansionEnabled: true,
	}
	s := &Service{cfg: cfg}

	// Hop 0: should use ONLY learned types WITH attention
	types, attention := s.getEdgeTypesForHop(0)
	if len(types) != 1 {
		t.Errorf("Hop 0: expected 1 learned edge type, got %d", len(types))
	}
	if types[0] != "CO_ACTIVATED_WITH" {
		t.Errorf("Hop 0: expected CO_ACTIVATED_WITH, got %s", types[0])
	}
	if !attention {
		t.Error("Hop 0: expected attention=true for learned_only strategy")
	}

	// Hop 1: should still use ONLY learned types WITH attention
	types, attention = s.getEdgeTypesForHop(1)
	if len(types) != 1 {
		t.Errorf("Hop 1: expected 1 learned edge type, got %d", len(types))
	}
	if !attention {
		t.Error("Hop 1: expected attention=true for learned_only strategy")
	}
}

// TestGetEdgeTypesForHop_AttentionDisabled tests behavior when QueryAwareExpansionEnabled=false
func TestGetEdgeTypesForHop_AttentionDisabled(t *testing.T) {
	cfg := config.Config{
		EdgeTypeStrategy:           "hybrid",
		AllowedRelationshipTypes:   []string{"ASSOCIATED_WITH", "GENERALIZES", "CO_ACTIVATED_WITH"},
		StructuralEdgeTypes:        []string{"ASSOCIATED_WITH", "GENERALIZES"},
		LearnedEdgeTypes:           []string{"CO_ACTIVATED_WITH"},
		HybridSwitchHop:            1,
		QueryAwareExpansionEnabled: false, // Disabled
	}
	s := &Service{cfg: cfg}

	// Hop 0: should use structural types, attention still false
	_, attention := s.getEdgeTypesForHop(0)
	if attention {
		t.Error("Hop 0: expected attention=false")
	}

	// Hop 1: should use learned types, but attention=false because QA expansion is disabled
	types, attention := s.getEdgeTypesForHop(1)
	if len(types) != 1 {
		t.Errorf("Hop 1: expected 1 learned edge type, got %d", len(types))
	}
	if attention {
		t.Error("Hop 1: expected attention=false when QueryAwareExpansionEnabled=false")
	}
}

// TestGetEdgeTypesForHop_CustomSwitchHop tests custom HybridSwitchHop values
func TestGetEdgeTypesForHop_CustomSwitchHop(t *testing.T) {
	cfg := config.Config{
		EdgeTypeStrategy:           "hybrid",
		AllowedRelationshipTypes:   []string{"ASSOCIATED_WITH", "GENERALIZES", "CO_ACTIVATED_WITH"},
		StructuralEdgeTypes:        []string{"ASSOCIATED_WITH", "GENERALIZES"},
		LearnedEdgeTypes:           []string{"CO_ACTIVATED_WITH"},
		HybridSwitchHop:            2, // Switch at hop 2 instead of 1
		QueryAwareExpansionEnabled: true,
	}
	s := &Service{cfg: cfg}

	// Hop 0: structural, no attention
	types, attention := s.getEdgeTypesForHop(0)
	if len(types) != 2 {
		t.Errorf("Hop 0: expected 2 structural edge types, got %d", len(types))
	}
	if attention {
		t.Error("Hop 0: expected attention=false")
	}

	// Hop 1: still structural, no attention (because switch is at hop 2)
	types, attention = s.getEdgeTypesForHop(1)
	if len(types) != 2 {
		t.Errorf("Hop 1: expected 2 structural edge types, got %d", len(types))
	}
	if attention {
		t.Error("Hop 1: expected attention=false (switch at hop 2)")
	}

	// Hop 2: learned with attention
	types, attention = s.getEdgeTypesForHop(2)
	if len(types) != 1 {
		t.Errorf("Hop 2: expected 1 learned edge type, got %d", len(types))
	}
	if !attention {
		t.Error("Hop 2: expected attention=true")
	}
}

// TestGetEdgeTypesForHop_ZeroSwitchHop tests HybridSwitchHop=0 (learned from start)
func TestGetEdgeTypesForHop_ZeroSwitchHop(t *testing.T) {
	cfg := config.Config{
		EdgeTypeStrategy:           "hybrid",
		AllowedRelationshipTypes:   []string{"ASSOCIATED_WITH", "GENERALIZES", "CO_ACTIVATED_WITH"},
		StructuralEdgeTypes:        []string{"ASSOCIATED_WITH", "GENERALIZES"},
		LearnedEdgeTypes:           []string{"CO_ACTIVATED_WITH"},
		HybridSwitchHop:            0, // Use learned from hop 0
		QueryAwareExpansionEnabled: true,
	}
	s := &Service{cfg: cfg}

	// Hop 0: learned with attention (because switch is at hop 0)
	types, attention := s.getEdgeTypesForHop(0)
	if len(types) != 1 {
		t.Errorf("Hop 0: expected 1 learned edge type, got %d", len(types))
	}
	if types[0] != "CO_ACTIVATED_WITH" {
		t.Errorf("Hop 0: expected CO_ACTIVATED_WITH, got %s", types[0])
	}
	if !attention {
		t.Error("Hop 0: expected attention=true")
	}
}
