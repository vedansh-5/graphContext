package archlint

import (
	"encoding/json"
	"os"

	"github.com/vedansh-5/graphcontext/pkg/store"
)

type ForbiddenRule struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Message string `json:"message,omitempty"`
}

type LayerRule struct {
	Layers []string `json:"layers"`
}

type RuleSet struct {
	Forbidden []ForbiddenRule `json:"forbidden,omitempty"`
	Layers    *LayerRule      `json:"layers,omitempty"`
}

type Violation struct {
	RuleType   string         `json:"rule_type"`
	SourceID   string         `json:"source_id"`
	SourceFile string         `json:"source_file"`
	SourceLine int            `json:"source_line"`
	TargetID   string         `json:"target_id"`
	TargetFile string         `json:"target_file"`
	EdgeKind   store.EdgeKind `json:"edge_kind"`
	Message    string         `json:"message"`
}

func LoadRulesFromFile(path string) (RuleSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuleSet{}, err
	}

	var rs RuleSet
	if err := json.Unmarshal(data, &rs); err != nil {
		return RuleSet{}, err
	}
	return rs, nil
}
