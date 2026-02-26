package parser

import "testing"

func TestInferRelationshipTypes(t *testing.T) {
	tests := []struct {
		name   string
		inputs []ParseResult
		want   []RelationshipType
	}{{
		"no parent",
		[]ParseResult{
			{Session: ParsedSession{ID: "abc"}},
		},
		[]RelationshipType{RelNone},
	},
		{
			"agent prefix gets subagent",
			[]ParseResult{
				{Session: ParsedSession{
					ID:              "agent-123",
					ParentSessionID: "parent",
				}},
			},
			[]RelationshipType{RelSubagent},
		},
		{
			"non-agent prefix gets continuation",
			[]ParseResult{
				{Session: ParsedSession{
					ID:              "child-session",
					ParentSessionID: "parent",
				}},
			},
			[]RelationshipType{RelContinuation},
		},
		{
			"explicit type preserved",
			[]ParseResult{
				{Session: ParsedSession{
					ID:               "abc-fork",
					ParentSessionID:  "parent",
					RelationshipType: RelFork,
				}},
			},
			[]RelationshipType{RelFork},
		},
		{
			"mixed results",
			[]ParseResult{
				{Session: ParsedSession{ID: "main"}},
				{Session: ParsedSession{
					ID:              "agent-task1",
					ParentSessionID: "main",
				}},
				{Session: ParsedSession{
					ID:               "main-fork-uuid",
					ParentSessionID:  "main",
					RelationshipType: RelFork,
				}},
				{Session: ParsedSession{
					ID:              "child",
					ParentSessionID: "main",
				}},
			},
			[]RelationshipType{
				RelNone, RelSubagent, RelFork, RelContinuation,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			InferRelationshipTypes(tt.inputs)
			if len(tt.inputs) != len(tt.want) {
				t.Fatalf("len(inputs) = %d, want %d", len(tt.inputs), len(tt.want))
			}
			for i, r := range tt.inputs {
				if r.Session.RelationshipType != tt.want[i] {
					t.Errorf(
						"inputs[%d].RelationshipType = %q, want %q",
						i, r.Session.RelationshipType, tt.want[i],
					)
				}
			}
		})
	}
}
