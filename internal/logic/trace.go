package logic

import "github.com/patrickwarner/openadserve/internal/models"

// TraceStep records candidate creatives and their line items at a selection stage.
type TraceStep struct {
	Stage       string            `json:"stage"`
	CreativeIDs []int             `json:"creative_ids"`
	LineItemIDs []int             `json:"line_item_ids"`
	Details     map[string]string `json:"details,omitempty"`
}

// SelectionTrace captures the ordered list of steps performed by a selector.
type SelectionTrace struct {
	Steps []TraceStep `json:"steps"`
}

// AddStep appends a trace entry for the given stage using the supplied creatives.
// Duplicate line item IDs are removed.
func (t *SelectionTrace) AddStep(stage string, creatives []models.Creative) {
	if t == nil {
		return
	}
	step := TraceStep{Stage: stage}
	seen := make(map[int]struct{})
	for _, c := range creatives {
		step.CreativeIDs = append(step.CreativeIDs, c.ID)
		if _, ok := seen[c.LineItemID]; !ok {
			seen[c.LineItemID] = struct{}{}
			step.LineItemIDs = append(step.LineItemIDs, c.LineItemID)
		}
	}
	t.Steps = append(t.Steps, step)
}

// AddStepWithDetails appends a trace entry with additional details about filtering.
func (t *SelectionTrace) AddStepWithDetails(stage string, creatives []models.Creative, details map[string]string) {
	if t == nil {
		return
	}
	step := TraceStep{Stage: stage, Details: details}
	seen := make(map[int]struct{})
	for _, c := range creatives {
		step.CreativeIDs = append(step.CreativeIDs, c.ID)
		if _, ok := seen[c.LineItemID]; !ok {
			seen[c.LineItemID] = struct{}{}
			step.LineItemIDs = append(step.LineItemIDs, c.LineItemID)
		}
	}
	t.Steps = append(t.Steps, step)
}
