package security

import (
	"slices"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/project"
)

// Flow is the changes an enum field may go through (@store(flow [...])):
// each from a state to another, by anyone who may change the record or,
// with by, only by those its rules name. A record starts in the field's
// default.
type Flow struct {
	Field *project.Field
	Moves []Move
}

// Move is one change of a flow: "REVIEW -> PUBLISHED by user.admin".
type Move struct {
	From, To *project.EnumValue
	// By are who may make it, as @security's rules name them; empty, anyone
	// who may change the record.
	By []string
}

// Step is a state of a flow with where it may go, for templates that
// switch on the state.
type Step struct {
	From  *project.EnumValue
	Moves []Move
}

// MoveText is a move as written: FROM, TO and who.
type MoveText struct {
	From, To string
	By       []string
}

// ParseMove reads "FROM -> TO" or "FROM -> TO by a, b"; ok is false when it
// is neither.
func ParseMove(s string) (MoveText, bool) {
	from, rest, ok := strings.Cut(s, "->")
	if !ok {
		return MoveText{}, false
	}
	to, by, hasBy := strings.Cut(strings.TrimSpace(rest), " by ")
	m := MoveText{From: strings.TrimSpace(from), To: strings.TrimSpace(to)}
	if hasBy {
		for w := range strings.SplitSeq(by, ",") {
			if w = strings.TrimSpace(w); w != "" {
				m.By = append(m.By, w)
			}
		}
		if len(m.By) == 0 {
			return MoveText{}, false
		}
	}
	if m.From == "" || m.To == "" || strings.ContainsAny(m.From+m.To, " ,") {
		return MoveText{}, false
	}
	return m, true
}

// FlowOf is the flow of a field: ok is false when it has none, or one that
// names what its enum does not have.
func FlowOf(f *project.Field) (Flow, bool) {
	raw, _ := f.Ann["store"]["flow"].([]any)
	if raw == nil || f.Enum == nil {
		return Flow{}, false
	}
	fl := Flow{Field: f}
	for _, x := range raw {
		s, _ := x.(string)
		m, ok := ParseMove(s)
		if !ok {
			return Flow{}, false
		}
		from, to := f.Enum.Value(m.From), f.Enum.Value(m.To)
		if from == nil || to == nil {
			return Flow{}, false
		}
		fl.Moves = append(fl.Moves, Move{From: from, To: to, By: m.By})
	}
	return fl, true
}

// Flows are the flows of e's fields.
func Flows(e *project.Entity) []Flow {
	var out []Flow
	for _, f := range e.Fields {
		if fl, ok := FlowOf(f); ok {
			out = append(out, fl)
		}
	}
	return out
}

// FlowsBy are e's flows with a move only some may make: those the API asks
// the policy about.
func FlowsBy(e *project.Entity) []Flow {
	var out []Flow
	for _, fl := range Flows(e) {
		if fl.HasBy() {
			out = append(out, fl)
		}
	}
	return out
}

// HasBy reports whether a move of the flow names who may make it.
func (fl Flow) HasBy() bool {
	for _, m := range fl.Moves {
		if len(m.By) > 0 {
			return true
		}
	}
	return false
}

// Steps are the flow's states that go somewhere, in the enum's order, each
// with its moves.
func (fl Flow) Steps() []Step {
	var out []Step
	for _, v := range fl.Field.Enum.Values {
		s := Step{From: v}
		for _, m := range fl.Moves {
			if m.From == v {
				s.Moves = append(s.Moves, m)
			}
		}
		if len(s.Moves) > 0 {
			out = append(out, s)
		}
	}
	return out
}

// Targets are the states the flow goes to, each once, in the enum's order:
// those a record's can may name.
func (fl Flow) Targets() []*project.EnumValue {
	var out []*project.EnumValue
	for _, v := range fl.Field.Enum.Values {
		for _, m := range fl.Moves {
			if m.To == v {
				out = append(out, v)
				break
			}
		}
	}
	return out
}

// Start is the state records start in: the field's default.
func (fl Flow) Start() *project.EnumValue {
	if fl.Field.Default == nil {
		return nil
	}
	return fl.Field.Default.EnumValue
}

// Next are the states v may go to.
func (fl Flow) Next(v *project.EnumValue) []*project.EnumValue {
	var out []*project.EnumValue
	for _, m := range fl.Moves {
		if m.From == v {
			out = append(out, m.To)
		}
	}
	return out
}

// Off is a state v may not go to, for tests; nil when it may go to all.
func (fl Flow) Off(v *project.EnumValue) *project.EnumValue {
	for _, x := range fl.Field.Enum.Values {
		if x != v && !slices.Contains(fl.Next(v), x) {
			return x
		}
	}
	return nil
}

// PathTo are the states a record goes through from the start to v, v the
// last, by the shortest way; empty for the start, nil when v is out of
// reach.
func (fl Flow) PathTo(v *project.EnumValue) []*project.EnumValue {
	start := fl.Start()
	if start == nil {
		return nil
	}
	if v == start {
		return []*project.EnumValue{}
	}
	prev := map[*project.EnumValue]*project.EnumValue{start: nil}
	queue := []*project.EnumValue{start}
	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]
		for _, to := range fl.Next(at) {
			if _, seen := prev[to]; seen {
				continue
			}
			prev[to] = at
			if to == v {
				var path []*project.EnumValue
				for x := v; x != start; x = prev[x] {
					path = append([]*project.EnumValue{x}, path...)
				}
				return path
			}
			queue = append(queue, to)
		}
	}
	return nil
}

// Text is the flow as the contract describes it: DRAFT -> REVIEW;
// REVIEW -> PUBLISHED (user.admin, user.moderator).
func (fl Flow) Text() string {
	parts := make([]string, 0, len(fl.Moves))
	for _, m := range fl.Moves {
		s := m.From.Raw + " -> " + m.To.Raw
		if len(m.By) > 0 {
			s += " (" + strings.Join(m.By, ", ") + ")"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "; ")
}

// CanNames are what a record's can may say of e's flows: "status:PUBLISHED"
// for each state a flow goes to.
func CanNames(e *project.Entity) []string {
	var out []string
	for _, fl := range Flows(e) {
		for _, v := range fl.Targets() {
			out = append(out, fl.Field.Snake+":"+v.Raw)
		}
	}
	return out
}

// FirstValue is the value of an enum field a test writes: the start of its
// flow, or its enum's first.
func FirstValue(f *project.Field) *project.EnumValue {
	if fl, ok := FlowOf(f); ok && fl.Start() != nil {
		return fl.Start()
	}
	return f.Enum.Values[0]
}
