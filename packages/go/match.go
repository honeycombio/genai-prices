package genaiprices

// matchKind enumerates the clause types in a MatchLogic.
type matchKind int

// MatchLogic is the recursive boolean logic used to match a string (a model or
// provider identifier). Exactly one clause kind is set per node.
//
// The matching implementation is added in a later commit; this is the type
// skeleton only.
type MatchLogic struct {
	kind     matchKind
	value    string       // for equals/starts_with/ends_with/contains/regex
	children []MatchLogic // for or/and
}

// IsMatch reports whether text satisfies this match logic. Stubbed here; the
// real logic is added in a later commit.
func (m *MatchLogic) IsMatch(text string) bool { return false }
