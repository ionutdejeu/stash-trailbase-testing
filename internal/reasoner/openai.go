package reasoner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/alash3al/stash/internal/models"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const systemPrompt = `You are a strict information extraction engine.

Rules:
- Extract ONLY what is explicitly stated in the provided text.
- You may use language understanding to interpret what is written.
- You may NOT infer, assume, generalize, or fill in gaps from your own knowledge.
- If a field cannot be found in the text, output null for that field.
- Never guess. Never approximate. Never complete missing information.
- When in doubt, output null.
- Output ONLY valid JSON matching the provided schema.
- No explanation. No preamble. No markdown. Just the JSON object.`

const retryWarning = "Your previous response was invalid or contained invented information. Follow the rules strictly. Output ONLY valid JSON matching the provided schema."

type OpenAI struct {
	client openai.Client
	model  string
}

func NewOpenAI(baseURL, apiKey, model string) (*OpenAI, error) {
	if apiKey == "" {
		return nil, errors.New("reasoner: apiKey is required")
	}
	if model == "" {
		return nil, errors.New("reasoner: model is required")
	}

	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(apiKey),
	)

	return &OpenAI{
		client: client,
		model:  model,
	}, nil
}

// --- JSON response types ---

type jsonFact struct {
	Entity   *string `json:"entity"`
	Property *string `json:"property"`
	Value    *string `json:"value"`
	Summary  *string `json:"summary"`
}

type jsonRelationship struct {
	From         string  `json:"from"`
	RelationType string  `json:"relation_type"`
	To           string  `json:"to"`
	Confidence   float32 `json:"confidence"`
}

type jsonPattern struct {
	Pattern     string  `json:"pattern"`
	Coherence   float32 `json:"coherence"`
	SourceFacts []int64 `json:"source_facts"`
	SourceRels  []int64 `json:"source_rels"`
}

type jsonContradiction struct {
	Classification string  `json:"classification"`
	Confidence     float32 `json:"confidence"`
	Explanation    string  `json:"explanation"`
}

type jsonCausalLink struct {
	CauseID    int64   `json:"cause_id"`
	EffectID   int64   `json:"effect_id"`
	Confidence float32 `json:"confidence"`
}

// --- ReasonStructured ---

func (o *OpenAI) ReasonStructured(ctx context.Context, texts []string) (*StructuredFact, error) {
	if len(texts) == 0 {
		return nil, errors.New("reasoner: texts must not be empty")
	}

	eventsList := strings.Join(texts, "\n- ")
	prompt := fmt.Sprintf(`Given these events, extract a single structured fact.

Events:
- %s

Output ONLY this exact JSON structure:
{"entity": "string or null", "property": "string or null", "value": "string or null", "summary": "string or null"}

Rules:
- All values MUST come ONLY from the Events listed above.
- Do not add details not present in the Events.
- If any field is not explicitly stated in the Events, use null.
- The summary must be a factual statement derived strictly from the Events.`, eventsList)

	msgs := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
		openai.UserMessage(prompt),
	}

	var result *StructuredFact
	var valErr error

	for attempt := 0; attempt < 2; attempt++ {
		resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
			Model:    o.model,
			Messages: msgs,
		})
		if err != nil {
			return nil, fmt.Errorf("chat.completions call failed: %w", err)
		}
		if len(resp.Choices) == 0 {
			return nil, errors.New("reasoner: no response from LLM")
		}

		output := strings.TrimSpace(resp.Choices[0].Message.Content)
		raw := extractJSON(output)

		var jf jsonFact
		if err := json.Unmarshal([]byte(raw), &jf); err != nil {
			valErr = fmt.Errorf("parse json: %w", err)
			msgs = append(msgs, openai.SystemMessage(retryWarning))
			continue
		}

		result = &StructuredFact{
			Entity:   ptrStr(jf.Entity),
			Property: ptrStr(jf.Property),
			Value:    ptrStr(jf.Value),
			Summary:  ptrStr(jf.Summary),
		}

		if err := validateFactGrounding(result, texts); err != nil {
			valErr = fmt.Errorf("grounding validation: %w", err)
			msgs = append(msgs, openai.SystemMessage(retryWarning+" "+err.Error()))
			result = nil
			continue
		}

		valErr = nil
		break
	}

	if valErr != nil {
		return nil, valErr
	}

	return result, nil
}

// --- ReasonRelationships ---

func (o *OpenAI) ReasonRelationships(ctx context.Context, factContent string) ([]*StructuredRelationship, error) {
	if factContent == "" {
		return nil, errors.New("reasoner: factContent must not be empty")
	}

	prompt := fmt.Sprintf(`Given this fact, extract all directly stated relationships.

Fact: %s

Output ONLY a JSON array of objects:
[{"from": "subject entity", "relation_type": "lowercase_type", "to": "object entity", "confidence": 0.0}]

Rules:
- Only extract relationships DIRECTLY stated in the Fact above.
- Do NOT infer transitive or implied relationships (e.g., if "Alice works at TechCorp in Paris", do NOT output Alice --located_in--> Paris).
- "from" and "to" must be entity names mentioned verbatim in the Fact.
- relation_type must be a simple lowercase identifier (e.g., works_at, manages, knows).
- confidence must be between 0.7 and 1.0.
- If no relationships are explicitly stated, output: []`, factContent)

	msgs := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
		openai.UserMessage(prompt),
	}

	var result []*StructuredRelationship
	var valErr error

	for attempt := 0; attempt < 2; attempt++ {
		resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
			Model:    o.model,
			Messages: msgs,
		})
		if err != nil {
			return nil, fmt.Errorf("chat.completions call failed: %w", err)
		}
		if len(resp.Choices) == 0 {
			return nil, errors.New("reasoner: no response from LLM")
		}

		output := strings.TrimSpace(resp.Choices[0].Message.Content)
		raw := extractJSON(output)

		var jrels []jsonRelationship
		if err := json.Unmarshal([]byte(raw), &jrels); err != nil {
			valErr = fmt.Errorf("parse json: %w", err)
			msgs = append(msgs, openai.SystemMessage(retryWarning))
			continue
		}

		var validated []*StructuredRelationship
		groundingFailed := false
		for _, jr := range jrels {
			if jr.From == "" || jr.RelationType == "" || jr.To == "" {
				continue
			}
			if !stringsContains(factContent, jr.From) || !stringsContains(factContent, jr.To) {
				groundingFailed = true
				break
			}
			conf := jr.Confidence
			if conf < 0.7 {
				conf = 0.7
			}
			if conf > 1.0 {
				conf = 1.0
			}
			validated = append(validated, &StructuredRelationship{
				FromEntity:   jr.From,
				RelationType: jr.RelationType,
				ToEntity:     jr.To,
				Confidence:   conf,
			})
		}

		if groundingFailed {
			valErr = errors.New("relationship entity not found in fact content")
			msgs = append(msgs, openai.SystemMessage(retryWarning+" Entity names must appear verbatim in the fact."))
			continue
		}

		result = validated
		valErr = nil
		break
	}

	if valErr != nil {
		return nil, valErr
	}

	return result, nil
}

// --- ReasonPatterns ---

func (o *OpenAI) ReasonPatterns(ctx context.Context, facts []models.Fact, relationships []models.Relationship) ([]*StructuredPattern, error) {
	if len(facts) == 0 {
		return nil, nil
	}

	var factLines []string
	for _, f := range facts {
		factLines = append(factLines, fmt.Sprintf("[Fact %d] %s (confidence: %.2f)", f.ID, f.Content, f.Confidence))
	}

	var relLines []string
	for _, r := range relationships {
		relLines = append(relLines, fmt.Sprintf("[Rel %d] %s --%s--> %s (confidence: %.2f)", r.ID, r.FromEntity, r.RelationType, r.ToEntity, r.Confidence))
	}

	prompt := fmt.Sprintf(`Given these facts and relationships, extract patterns.

Facts:
%s

Relationships:
%s

Output ONLY a JSON array of objects:
[{"pattern": "string", "coherence": 0.0, "source_facts": [1,2], "source_rels": [3,4]}]

Rules:
- A pattern MUST have at least 2 items in source_facts OR at least 2 items in source_rels. Single-source patterns are NOT allowed.
- source_facts IDs must be from the Facts list above.
- source_rels IDs must be from the Relationships list above.
- Do not generalize beyond the evidence. If no pattern meets this threshold, output: []
- The pattern statement must be directly supported by the referenced sources.`, strings.Join(factLines, "\n"), strings.Join(relLines, "\n"))

	msgs := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
		openai.UserMessage(prompt),
	}

	factIDs := make(map[int64]bool)
	for _, f := range facts {
		factIDs[f.ID] = true
	}
	relIDs := make(map[int64]bool)
	for _, r := range relationships {
		relIDs[r.ID] = true
	}

	var result []*StructuredPattern
	var valErr error

	for attempt := 0; attempt < 2; attempt++ {
		resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
			Model:    o.model,
			Messages: msgs,
		})
		if err != nil {
			return nil, fmt.Errorf("chat.completions call failed: %w", err)
		}
		if len(resp.Choices) == 0 {
			return nil, errors.New("reasoner: no response from LLM")
		}

		output := strings.TrimSpace(resp.Choices[0].Message.Content)
		raw := extractJSON(output)

		var jpats []jsonPattern
		if err := json.Unmarshal([]byte(raw), &jpats); err != nil {
			valErr = fmt.Errorf("parse json: %w", err)
			msgs = append(msgs, openai.SystemMessage(retryWarning))
			continue
		}

		var validated []*StructuredPattern
		validationFailed := false

		for _, jp := range jpats {
			if jp.Pattern == "" {
				continue
			}

			validFacts, badFacts := filterIDs(jp.SourceFacts, factIDs)
			validRels, badRels := filterIDs(jp.SourceRels, relIDs)

			if badFacts || badRels {
				validationFailed = true
				break
			}

			if len(validFacts)+len(validRels) < 2 {
				continue
			}

			coherence := jp.Coherence
			if coherence <= 0 || coherence > 1.0 {
				coherence = 0.5
			}

			validated = append(validated, &StructuredPattern{
				Content:       jp.Pattern,
				CoherenceScore: coherence,
				SourceFactIDs: validFacts,
				SourceRelIDs:  validRels,
			})
		}

		if validationFailed {
			valErr = errors.New("pattern references non-existent source IDs")
			msgs = append(msgs, openai.SystemMessage(retryWarning+" Source IDs must be from the provided lists only."))
			continue
		}

		result = validated
		valErr = nil
		break
	}

	if valErr != nil {
		return nil, valErr
	}

	return result, nil
}

// --- ReasonContradiction ---

func (o *OpenAI) ReasonContradiction(ctx context.Context, entity, property, oldValue, newValue string) (*ContradictionResult, error) {
	if entity == "" || property == "" || oldValue == "" || newValue == "" {
		return nil, errors.New("reasoner: entity, property, oldValue, and newValue are required")
	}

	prompt := fmt.Sprintf(`Given two facts about the same entity and property, classify their relationship.

Entity: %s
Property: %s
Old value: %s
New value: %s

Output ONLY this exact JSON structure:
{"classification": "replacement|contradiction|compatible", "confidence": 0.0, "explanation": "string"}

Rules:
- "replacement": The new value replaces the old one (e.g., changed address, updated title, new phone number). The old value is no longer true.
- "contradiction": Both values cannot be true simultaneously, but it is unclear which is correct. Requires human resolution.
- "compatible": Both values can be true at the same time (e.g., multiple roles, parallel attributes). No conflict.
- confidence must be between 0.0 and 1.0.
- explanation must be a brief sentence justifying the classification.`, entity, property, oldValue, newValue)

	msgs := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
		openai.UserMessage(prompt),
	}

	for attempt := 0; attempt < 2; attempt++ {
		resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
			Model:    o.model,
			Messages: msgs,
		})
		if err != nil {
			return nil, fmt.Errorf("chat.completions call failed: %w", err)
		}
		if len(resp.Choices) == 0 {
			return nil, errors.New("reasoner: no response from LLM")
		}

		output := strings.TrimSpace(resp.Choices[0].Message.Content)
		raw := extractJSON(output)

		var jc jsonContradiction
		if err := json.Unmarshal([]byte(raw), &jc); err != nil {
			msgs = append(msgs, openai.SystemMessage(retryWarning))
			continue
		}

		class := ContradictionClassification(jc.Classification)
		switch class {
		case ClassificationReplacement, ClassificationContradiction, ClassificationCompatible:
		default:
			msgs = append(msgs, openai.SystemMessage(retryWarning+" Classification must be one of: replacement, contradiction, compatible."))
			continue
		}

		conf := jc.Confidence
		if conf < 0 {
			conf = 0
		}
		if conf > 1 {
			conf = 1
		}

		return &ContradictionResult{
			Classification: class,
			Confidence:     conf,
			Explanation:    jc.Explanation,
		}, nil
	}

	return nil, errors.New("reasoner: failed to get valid contradiction classification after retries")
}

// --- ReasonCausalLinks ---

func (o *OpenAI) ReasonCausalLinks(ctx context.Context, facts []models.Fact) ([]*StructuredCausalLink, error) {
	if len(facts) < 2 {
		return nil, nil
	}

	var factLines []string
	for _, f := range facts {
		factLines = append(factLines, fmt.Sprintf("[Fact %d] %s", f.ID, f.Content))
	}

	prompt := fmt.Sprintf(`Given these facts, identify cause-effect relationships between them.

Facts:
%s

Output ONLY a JSON array of objects:
[{"cause_id": 1, "effect_id": 2, "confidence": 0.9}]

Rules:
- cause_id and effect_id must be fact IDs from the list above.
- Only identify relationships where one fact DIRECTLY caused or led to another.
- Do NOT infer transitive or indirect causation.
- A fact can be both a cause and an effect of different facts.
- cause_id and effect_id must be different (no self-loops).
- confidence must be between 0.5 and 1.0.
- If no causal relationships are evident, output: []`, strings.Join(factLines, "\n"))

	msgs := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
		openai.UserMessage(prompt),
	}

	factIDs := make(map[int64]bool)
	for _, f := range facts {
		factIDs[f.ID] = true
	}

	var result []*StructuredCausalLink
	var valErr error

	for attempt := 0; attempt < 2; attempt++ {
		resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
			Model:    o.model,
			Messages: msgs,
		})
		if err != nil {
			return nil, fmt.Errorf("chat.completions call failed: %w", err)
		}
		if len(resp.Choices) == 0 {
			return nil, errors.New("reasoner: no response from LLM")
		}

		output := strings.TrimSpace(resp.Choices[0].Message.Content)
		raw := extractJSON(output)

		var jlinks []jsonCausalLink
		if err := json.Unmarshal([]byte(raw), &jlinks); err != nil {
			valErr = fmt.Errorf("parse json: %w", err)
			msgs = append(msgs, openai.SystemMessage(retryWarning))
			continue
		}

		var validated []*StructuredCausalLink
		validationFailed := false

		for _, jl := range jlinks {
			if !factIDs[jl.CauseID] || !factIDs[jl.EffectID] {
				validationFailed = true
				break
			}
			if jl.CauseID == jl.EffectID {
				continue
			}

			conf := jl.Confidence
			if conf < 0.5 {
				conf = 0.5
			}
			if conf > 1.0 {
				conf = 1.0
			}

			validated = append(validated, &StructuredCausalLink{
				CauseFactID:  jl.CauseID,
				EffectFactID: jl.EffectID,
				Confidence:   conf,
			})
		}

		if validationFailed {
			valErr = errors.New("causal link references non-existent fact ID")
			msgs = append(msgs, openai.SystemMessage(retryWarning+" Fact IDs must be from the provided list only."))
			continue
		}

		result = validated
		valErr = nil
		break
	}

	if valErr != nil {
		return nil, valErr
	}

	return result, nil
}

// --- Helpers ---

func extractJSON(s string) string {
	s = strings.TrimSpace(s)

	trimmed := strings.TrimPrefix(s, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return trimmed
	}

	return s
}

func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func validateFactGrounding(sf *StructuredFact, texts []string) error {
	if sf.Summary == "" {
		return nil
	}

	joined := strings.ToLower(strings.Join(texts, " "))
	wordSet := tokenize(joined)

	for _, field := range []string{sf.Entity, sf.Property, sf.Value, sf.Summary} {
		if field == "" {
			continue
		}
		words := tokenize(strings.ToLower(field))
		ungrounded := 0
		for w := range words {
			if !wordSet[w] {
				ungrounded++
			}
		}
		total := len(words)
		if total > 0 && float32(ungrounded)/float32(total) > 0.3 {
			return fmt.Errorf("field contains ungrounded words: %q", field)
		}
	}

	return nil
}

func tokenize(s string) map[string]bool {
	words := make(map[string]bool)
	var buf strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			buf.WriteRune(unicode.ToLower(r))
		} else {
			if buf.Len() >= 3 {
				words[buf.String()] = true
			}
			buf.Reset()
		}
	}
	if buf.Len() >= 3 {
		words[buf.String()] = true
	}
	return words
}

func stringsContains(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

func filterIDs(ids []int64, valid map[int64]bool) (filtered []int64, hasInvalid bool) {
	for _, id := range ids {
		if valid[id] {
			filtered = append(filtered, id)
		} else {
			hasInvalid = true
		}
	}
	return
}
