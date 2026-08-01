package storage

import (
	"fmt"
	"strconv"
	"strings"
)

// RPNEvaluator evaluates Reverse Polish Notation expressions against a set of fields/tags.
type RPNEvaluator struct {
	expr string
}

// NewRPNEvaluator creates a new RPN evaluator.
func NewRPNEvaluator(expr string) *RPNEvaluator {
	return &RPNEvaluator{expr: expr}
}

// Evaluate evaluates the RPN expression against the given point.
func (e *RPNEvaluator) Evaluate(pt TSDBPoint) (bool, error) {
	if e.expr == "" {
		return true, nil // Empty expression matches everything
	}

	tokens := strings.Fields(e.expr)
	var stack []float64

	// Helper to resolve variables
	resolveVar := func(name string) (float64, error) {
		// First check fields
		if val, ok := pt.Fields[name]; ok {
			switch v := val.(type) {
			case float64:
				return v, nil
			case int:
				return float64(v), nil
			case string:
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					return f, nil
				}
				return 0, fmt.Errorf("field %s is a string but not numeric", name)
			}
		}
		// Then check tags
		if val, ok := pt.Tags[name]; ok {
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				return f, nil
			}
			return 0, fmt.Errorf("tag %s is a string but not numeric", name)
		}
		return 0, fmt.Errorf("variable %s not found", name)
	}

	for _, token := range tokens {
		switch token {
		case "+", "-", "*", "/", ">", "<", "==", ">=", "<=":
			if len(stack) < 2 {
				return false, fmt.Errorf("invalid expression: not enough operands for %s", token)
			}
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]

			var res float64
			switch token {
			case "+":
				res = a + b
			case "-":
				res = a - b
			case "*":
				res = a * b
			case "/":
				if b == 0 {
					return false, fmt.Errorf("division by zero")
				}
				res = a / b
			case ">":
				if a > b {
					res = 1
				} else {
					res = 0
				}
			case "<":
				if a < b {
					res = 1
				} else {
					res = 0
				}
			case "==":
				if a == b {
					res = 1
				} else {
					res = 0
				}
			case ">=":
				if a >= b {
					res = 1
				} else {
					res = 0
				}
			case "<=":
				if a <= b {
					res = 1
				} else {
					res = 0
				}
			}
			stack = append(stack, res)
		default:
			// Try to parse as number
			if val, err := strconv.ParseFloat(token, 64); err == nil {
				stack = append(stack, val)
			} else {
				// Treat as variable
				val, err := resolveVar(token)
				if err != nil {
					return false, err
				}
				stack = append(stack, val)
			}
		}
	}

	if len(stack) != 1 {
		return false, fmt.Errorf("invalid expression: stack has %d items left", len(stack))
	}

	return stack[0] != 0, nil
}
