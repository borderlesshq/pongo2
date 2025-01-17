package pongo2

import (
	"fmt"
	"sync"
)

// FilterFunction is the type filter functions must fulfil
type FilterFunction func(in *Value, param *Value, bind map[string]any) (out *Value, err *Error)

// var filters map[string]FilterFunction
var filters *sync.Map

func init() {
	filters = new(sync.Map)
}

// FilterExists returns true if the given filter is already registered
func FilterExists(name string) bool {
	_, existing := filters.Load(name)
	return existing
}

// RegisterFilter registers a new filter. If there's already a filter with the same. You usually
// want to call this function in the filter's init() function:
//
//	http://golang.org/doc/effective_go.html#init
func RegisterFilter(name string, fn FilterFunction) error {
	if FilterExists(name) {
		return fmt.Errorf("filter with name '%s' is already registered", name)
	}

	filters.Store(name, fn)
	return nil
}

// ReplaceFilter replaces an already registered filter with a new implementation. Use this
// function with caution since it allows you to change existing filter behaviour.
func ReplaceFilter(name string, fn FilterFunction) error {
	if !FilterExists(name) {
		return fmt.Errorf("filter with name '%s' does not exist (therefore cannot be overridden)", name)
	}
	filters.Swap(name, fn)
	return nil
}

func OverrideFilter(name string, fn FilterFunction) error {
	filters.Delete(name)
	filters.Store(name, fn)
	return nil
}

// MustApplyFilter behaves like ApplyFilter, but panics on an error.
//func MustApplyFilter(name string, value *Value, param *Value) *Value {
//	val, err := ApplyFilter(name, value, param)
//	if err != nil {
//		panic(err)
//	}
//	return val
//}

// ApplyFilter applies a filter to a given value using the given parameters.
// Returns a *pongo2.Value or an error.
func ApplyFilter(name string, value *Value, param *Value, bind map[string]any) (*Value, *Error) {
	storedValue, existing := filters.Load(name)
	if !existing {
		return nil, &Error{
			Sender:    "applyfilter",
			OrigError: fmt.Errorf("filter with name '%s' not found", name),
		}
	}

	// Make sure param is a *Value
	if param == nil {
		param = AsValue(nil)
	}

	fn, _ := storedValue.(FilterFunction)
	return fn(value, param, bind)
}

type filterCall struct {
	token *Token

	name      string
	parameter IEvaluator

	filterFunc FilterFunction
}

func (fc *filterCall) Execute(v *Value, ctx *ExecutionContext) (*Value, *Error) {
	var param *Value
	var err *Error

	if fc.parameter != nil {
		param, err = fc.parameter.Evaluate(ctx)
		if err != nil {
			return nil, err
		}
	} else {
		param = AsValue(nil)
	}

	filteredValue, err := fc.filterFunc(v, param, ctx.Public)
	if err != nil {
		return nil, err.updateFromTokenIfNeeded(ctx.template, fc.token)
	}
	return filteredValue, nil
}

// Filter = IDENT | IDENT ":" FilterArg | IDENT "|" Filter
func (p *Parser) parseFilter() (*filterCall, *Error) {
	identToken := p.MatchType(TokenIdentifier)

	// Check filter ident
	if identToken == nil {
		return nil, p.Error("Filter name must be an identifier.", nil)
	}

	filter := &filterCall{
		token: identToken,
		name:  identToken.Val,
	}

	// Get the appropriate filter function and bind it
	storedFunc, exists := filters.Load(identToken.Val)
	if !exists {
		return nil, p.Error(fmt.Sprintf("Filter '%s' does not exist.", identToken.Val), identToken)
	}
	filterFn, _ := storedFunc.(FilterFunction)

	filter.filterFunc = filterFn

	// Check for filter-argument (2 tokens needed: ':' ARG)
	if p.Match(TokenSymbol, ":") != nil {
		if p.Peek(TokenSymbol, "}}") != nil {
			return nil, p.Error("Filter parameter required after ':'.", nil)
		}

		// Get filter argument expression
		v, err := p.parseVariableOrLiteral()
		if err != nil {
			return nil, err
		}
		filter.parameter = v
	}

	return filter, nil
}
