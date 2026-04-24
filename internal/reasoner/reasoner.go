// Package reasoner synthesizes structured reasoning over text.
package reasoner

import (
	"context"

	"github.com/alash3al/stash/internal/models"
)

// StructuredFact represents an extracted fact with entity, property, and value.
type StructuredFact struct {
	Entity   string
	Property string
	Value    string
	Summary  string
}

// StructuredRelationship represents an extracted relationship between two entities.
type StructuredRelationship struct {
	FromEntity   string
	RelationType string
	ToEntity     string
	Confidence   float32
}

// StructuredPattern represents an abstract pattern derived from facts and relationships.
type StructuredPattern struct {
	Content        string
	CoherenceScore float32
	SourceFactIDs  []int64
	SourceRelIDs   []int64
}

// ContradictionClassification is the result of classifying a fact pair.
type ContradictionClassification string

const (
	ClassificationReplacement   ContradictionClassification = "replacement"
	ClassificationContradiction ContradictionClassification = "contradiction"
	ClassificationCompatible    ContradictionClassification = "compatible"
)

// ContradictionResult is the LLM output for classifying two facts about the same entity+property.
type ContradictionResult struct {
	Classification ContradictionClassification
	Confidence     float32
	Explanation    string
}

// StructuredCausalLink represents an extracted cause-effect relationship between two facts.
type StructuredCausalLink struct {
	CauseFactID  int64
	EffectFactID int64
	Confidence   float32
}

// Reasoner synthesizes structured reasoning over text input.
type Reasoner interface {
	// ReasonStructured takes a list of text inputs and returns a structured fact.
	ReasonStructured(ctx context.Context, texts []string) (*StructuredFact, error)

	// ReasonRelationships takes a fact and extracts relationships between entities.
	ReasonRelationships(ctx context.Context, factContent string) ([]*StructuredRelationship, error)

	// ReasonPatterns takes facts and relationships and extracts abstract patterns.
	ReasonPatterns(ctx context.Context, facts []models.Fact, relationships []models.Relationship) ([]*StructuredPattern, error)

	// ReasonContradiction classifies whether a new fact replaces, contradicts, or is compatible with an old fact.
	ReasonContradiction(ctx context.Context, entity, property, oldValue, newValue string) (*ContradictionResult, error)

	// ReasonCausalLinks takes a batch of facts and extracts cause-effect pairs.
	ReasonCausalLinks(ctx context.Context, facts []models.Fact) ([]*StructuredCausalLink, error)
}
