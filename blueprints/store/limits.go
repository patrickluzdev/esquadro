package store

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/patrickluzdev/esquadro/blueprints/postgres"
	"github.com/patrickluzdev/esquadro/blueprints/security"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// CreateLimit is how many records of an entity each person may create in
// a period (@rate_limit(create "10/h")). A person is a principal of the
// realm that owns the records; what they created is what they own, created
// in the period.
type CreateLimit struct {
	// Max is how many records a period allows.
	Max int
	// Period is the window the records are counted in.
	Period time.Duration
	// Text is the limit as the model writes it: 10/h.
	Text string
	// Realm is the owner's realm, whose people are limited.
	Realm string
	// Relation is the relation to the owner, and Column its column.
	Relation *project.Relation
	Column   string
}

// Seconds is the period in seconds.
func (l CreateLimit) Seconds() int { return int(l.Period / time.Second) }

// Unit is the period in a word: minute, hour or day.
func (l CreateLimit) Unit() string {
	switch l.Period {
	case time.Minute:
		return "minute"
	case time.Hour:
		return "hour"
	}
	return "day"
}

// Per is the period after a number: a minute, an hour or a day.
func (l CreateLimit) Per() string {
	if l.Period == time.Hour {
		return "an hour"
	}
	return "a " + l.Unit()
}

var limitText = regexp.MustCompile(`^(\d+)/(min|h|d)$`)

// ParseLimit reads a limit as the annotation writes it: 30/min, 10/h or
// 100/d.
func ParseLimit(s string) (int, time.Duration, error) {
	m := limitText.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, fmt.Errorf("%q is not N/min, N/h or N/d", s)
	}
	max, err := strconv.Atoi(m[1])
	if err != nil || max <= 0 {
		return 0, 0, fmt.Errorf("%q: the number of records must be more than 0", s)
	}
	return max, map[string]time.Duration{"min": time.Minute, "h": time.Hour, "d": 24 * time.Hour}[m[2]], nil
}

// CreateLimitText is the limit an entity's annotation writes, empty
// without one.
func CreateLimitText(e *project.Entity) string {
	s, _ := e.Ann["rate_limit"]["create"].(string)
	return s
}

// CreateLimitOf is the limit of creates of e, nil when it has none or one
// the check refuses: a limit needs an owner of the records, other than the
// record itself.
func CreateLimitOf(entities []*project.Entity, e *project.Entity) *CreateLimit {
	text := CreateLimitText(e)
	if text == "" || Single(e) {
		return nil
	}
	max, per, err := ParseLimit(text)
	if err != nil {
		return nil
	}
	o, ok := security.OwnerUsed(entities, e)
	if !ok || o.Relation == nil {
		return nil
	}
	return &CreateLimit{Max: max, Period: per, Text: text, Realm: o.Realm, Relation: o.Relation, Column: postgres.Ident(o.Relation.Snake + "_id")}
}

// LimitedModule reports whether a module has an entity with a limit of
// creates: it has CreateLimited.
func LimitedModule(entities []*project.Entity, m *project.Module) bool {
	return slices.ContainsFunc(m.Entities, func(e *project.Entity) bool { return CreateLimitOf(entities, e) != nil })
}
