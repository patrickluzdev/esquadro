package store

import (
	"slices"
	"strconv"

	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// HistoryMax is the most points a field's history answers: the newest.
const HistoryMax = 1000

// historyTypes are the types a field with a history may have: numbers,
// whose lows and highs mean something.
var historyTypes = []project.Type{project.TypeInt, project.TypeInt64, project.TypeDecimal, project.TypeMoney}

// History reports whether the store keeps every value a field takes, with
// when (@store(history)).
func History(f *project.Field) bool {
	on, _ := f.Ann["store"]["history"].(bool)
	return on
}

// Histories are e's fields with a history.
func Histories(e *project.Entity) []*project.Field {
	var out []*project.Field
	for _, f := range e.Fields {
		if History(f) && !f.Implicit {
			out = append(out, f)
		}
	}
	return out
}

// HistoryTable is the table of a field's history, in its entity's schema:
// offer_price_history.
func HistoryTable(e *project.Entity, f *project.Field) string {
	return e.Snake + "_" + f.Snake + "_history"
}

func historyTable(e *project.Entity, f *project.Field, opts map[string]any) string {
	return schemaOf(e.Module, opts) + "." + HistoryTable(e, f)
}

// historyIn converts the domain value expr of a field with a history to
// what the point's value column takes (never NULL).
func historyIn(f *project.Field, expr string) string {
	switch f.Type {
	case project.TypeInt:
		return "int32(" + expr + ")"
	case project.TypeDecimal:
		return "postgres.Numeric(" + expr + ")"
	case project.TypeMoney:
		return "postgres.MoneyAmount(" + expr + ")"
	}
	return expr
}

// historyOut converts a point's column expr (row.Value, row.Low) to the
// domain's value; a money's currency is in the column beside it.
func historyOut(f *project.Field, expr string) string {
	switch f.Type {
	case project.TypeInt:
		return "int(" + expr + ")"
	case project.TypeDecimal:
		return "postgres.Decimal(" + expr + ")"
	case project.TypeMoney:
		return "postgres.Money(" + expr + ", " + expr + "Currency)"
	}
	return expr
}

// historyChanged is the Go condition of a value after that is not the one
// before.
func historyChanged(f *project.Field, before, after string) string {
	switch f.Type {
	case project.TypeDecimal, project.TypeMoney:
		return "!" + before + ".Equal(" + after + ")"
	}
	return before + " != " + after
}

// historyCast is the SQL type a history's low and high are read as.
func historyCast(f *project.Field) string {
	switch f.Type {
	case project.TypeDecimal, project.TypeMoney:
		return "numeric"
	}
	return "bigint"
}

// historyImports are the imports the values of e's histories need.
func historyImports(e *project.Entity) []string {
	var out []string
	for _, f := range Histories(e) {
		switch f.Type {
		case project.TypeDecimal:
			out = append(out, "github.com/shopspring/decimal")
		case project.TypeMoney:
			out = append(out, "github.com/bojanz/currency")
		}
	}
	return out
}

// HistorySamples are two valid values of a field with a history, as Go
// expressions, the low one first, for tests; ok is false when its rules
// leave room for one value only.
func HistorySamples(f *project.Field) (low, high string, ok bool) {
	base := 1.0
	if f.Min != nil {
		base = f.Min.Value
	} else if f.Max != nil && f.Max.Value < 1 {
		base = f.Max.Value
	}
	lo, hi := base, base+1
	if f.Max != nil && hi > f.Max.Value {
		lo, hi = base-1, base
		if f.Min != nil && lo < f.Min.Value {
			return "", "", false
		}
	}
	value := func(n float64) string {
		s := strconv.FormatFloat(n, 'f', -1, 64)
		switch f.Type {
		case project.TypeDecimal:
			return `decimal.RequireFromString("` + s + `")`
		case project.TypeMoney:
			return `func() currency.Amount { a, _ := currency.NewAmount("` + s + `", "USD"); return a }()`
		case project.TypeInt64:
			return "int64(" + s + ")"
		}
		return s
	}
	return value(lo), value(hi), true
}

// historyEqual is the Go condition of two values of a field being equal.
func historyEqual(f *project.Field, x, y string) string {
	switch f.Type {
	case project.TypeDecimal, project.TypeMoney:
		return x + ".Equal(" + y + ")"
	}
	return x + " == " + y
}

// checkHistories checks @store(history): a declared field, of a number or
// money, written (not a total), of an entity with more than one record.
func checkHistories(p *project.Project, diags *diag.List) {
	for _, e := range p.Entities {
		for _, f := range e.Fields {
			if !History(f) {
				continue
			}
			switch {
			case f.Implicit:
				diags.Errorf(f.Pos, f.Pos, "history: %s.%s is a field esquadro keeps; a history is of a field declared", e.Raw, f.Raw)
			case !slices.Contains(historyTypes, f.Type):
				diags.Errorf(f.Pos, f.Pos, "history: %s.%s is %s; a history keeps int, int64, decimal or money", e.Raw, f.Raw, f.Type)
			case IsTotal(f):
				diags.Errorf(f.Pos, f.Pos, "history: %s.%s is a total the store keeps; a history keeps a field written", e.Raw, f.Raw)
			case Single(e):
				diags.Errorf(f.Pos, f.Pos, "history: %s is single; a history is of the records of an entity with many", e.Raw)
			}
		}
	}
}
