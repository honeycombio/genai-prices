package genaiprices

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// matchKind enumerates the clause types in a MatchLogic.
type matchKind int

const (
	matchNone matchKind = iota
	matchEquals
	matchStartsWith
	matchEndsWith
	matchContains
	matchRegex
	matchOr
	matchAnd
)

// MatchLogic is the recursive boolean logic used to match a string (a model or
// provider identifier). Exactly one clause kind is set per node.
type MatchLogic struct {
	kind     matchKind
	value    string       // for equals/starts_with/ends_with/contains/regex
	children []MatchLogic // for or/and
}

func (m *MatchLogic) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for key, val := range raw {
		switch key {
		case "equals", "starts_with", "ends_with", "contains", "regex":
			var s string
			if err := json.Unmarshal(val, &s); err != nil {
				return err
			}
			m.value = s
			m.kind = scalarKind(key)
		case "or", "and":
			var children []MatchLogic
			if err := json.Unmarshal(val, &children); err != nil {
				return err
			}
			m.children = children
			if key == "or" {
				m.kind = matchOr
			} else {
				m.kind = matchAnd
			}
		default:
			return fmt.Errorf("genaiprices: unknown match clause %q", key)
		}
		// Each MatchLogic node has exactly one clause.
		return nil
	}
	return fmt.Errorf("genaiprices: empty match clause")
}

// Constructors for building MatchLogic programmatically (e.g. for a custom
// Provider passed via WithProvider). These mirror the JSON clause shapes used
// in the catalog: {equals}, {starts_with}, {ends_with}, {contains}, {regex},
// {or}, {and}.

// Equals matches text equal to s (case-insensitive).
func Equals(s string) MatchLogic { return MatchLogic{kind: matchEquals, value: s} }

// StartsWith matches text with prefix s (case-insensitive).
func StartsWith(s string) MatchLogic { return MatchLogic{kind: matchStartsWith, value: s} }

// EndsWith matches text with suffix s (case-insensitive).
func EndsWith(s string) MatchLogic { return MatchLogic{kind: matchEndsWith, value: s} }

// Contains matches text containing s (case-insensitive).
func Contains(s string) MatchLogic { return MatchLogic{kind: matchContains, value: s} }

// Regex matches text against the regular expression s (case-sensitive).
func Regex(s string) MatchLogic { return MatchLogic{kind: matchRegex, value: s} }

// Or matches when any child clause matches.
func Or(clauses ...MatchLogic) MatchLogic { return MatchLogic{kind: matchOr, children: clauses} }

// And matches when all child clauses match.
func And(clauses ...MatchLogic) MatchLogic { return MatchLogic{kind: matchAnd, children: clauses} }

func scalarKind(key string) matchKind {
	switch key {
	case "equals":
		return matchEquals
	case "starts_with":
		return matchStartsWith
	case "ends_with":
		return matchEndsWith
	case "contains":
		return matchContains
	case "regex":
		return matchRegex
	default:
		return matchNone
	}
}

// IsMatch reports whether text satisfies this match logic. All comparisons are
// case-insensitive except regex.
func (m *MatchLogic) IsMatch(text string) bool {
	switch m.kind {
	case matchOr:
		for i := range m.children {
			if m.children[i].IsMatch(text) {
				return true
			}
		}
		return false
	case matchAnd:
		for i := range m.children {
			if !m.children[i].IsMatch(text) {
				return false
			}
		}
		return true
	case matchEquals:
		return strings.EqualFold(text, m.value)
	case matchStartsWith:
		return strings.HasPrefix(strings.ToLower(text), strings.ToLower(m.value))
	case matchEndsWith:
		return strings.HasSuffix(strings.ToLower(text), strings.ToLower(m.value))
	case matchContains:
		return strings.Contains(strings.ToLower(text), strings.ToLower(m.value))
	case matchRegex:
		re := compileRegex(m.value)
		return re != nil && re.MatchString(text)
	default:
		return false
	}
}

var regexCache sync.Map // string -> *regexp.Regexp

// compileRegex compiles and caches a regex pattern. Patterns that fail to
// compile return nil (treated as a non-match), matching the lenient behavior of
// the JS/Python implementations which never reject the dataset for a bad regex.
func compileRegex(pattern string) *regexp.Regexp {
	if v, ok := regexCache.Load(pattern); ok {
		re, _ := v.(*regexp.Regexp)
		return re
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		re = nil
	}
	regexCache.Store(pattern, re)
	return re
}
