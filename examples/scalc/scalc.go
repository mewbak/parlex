package scalc

import (
	"fmt"
	"github.com/adamcolton/parlex"
	"github.com/adamcolton/parlex/grammar"
	"github.com/adamcolton/parlex/lexer/simplelexer"
	"github.com/adamcolton/parlex/parser/packrat"
	"github.com/adamcolton/parlex/tree"
	"math"
	"strconv"
)

const lexerRules = `
  space /\s+/ -
  int   /(\+|-)?\d+/
  dec   /\.\d+/
  uop   /(--)|(abs)/
  bop   /(cmpr)|[\*\/+\-\^%><=]/
  sop   /(len)|(sum)|(avg)|(min)|(max)|(first)|(last)/
  smp   /(swap)|(drop)|(clear)/
  ?     /\?/
  (     /\(/
  )     /\)/
`

const grammarRules = `
  Stack  -> Stack Smp
         -> E Stack
         -> Stack P Stack
         ->
  E      -> Stack Sop
         -> E E E ?
         -> E Uop
         -> E E Bop
         -> Number
  Number -> int
         -> int dec
  P      -> ( Stack )
  Bop    -> bop
         -> Bop Bop E ?
  Uop    -> uop
         -> Uop Uop E ?
  Sop    -> sop
         -> Sop Sop E ?
  Smp    -> Smp Smp E ?
         -> smp
`

var rdcr = tree.Reducer{
	"Stack": stack,
	"E":     tree.PromoteChild(-1),
	"P":     tree.ReplaceWithChild(1),
	"Bop":   tree.PromoteChild(-1),
	"Uop":   tree.PromoteChild(-1),
	"Sop":   tree.PromoteChild(-1),
	"Smp":   tree.PromoteChild(-1),
}

func stack(node *tree.PN) {
	if node.ChildAt(-1, "Stack") {
		node.PromoteChildrenOf(-1)
	}
	if node.ChildAt(0, "Stack") {
		node.PromoteChildrenOf(0)
	}
	if node.ChildAt(-1, "smp", "?") {
		node.PromoteChild(-1)
	} else {
		node.PromoteSingleChild()
	}
}

var lxr = parlex.MustLexer(simplelexer.New(lexerRules))
var grmr = parlex.MustGrammar(grammar.New(grammarRules))
var prsr = packrat.New(grmr)

// Parse will return the root of the Parse Tree.
func Parse(str string) parlex.ParseNode {
	return rdcr.Reduce(prsr.Parse(lxr.Lex(str)))
}

// Eval will evaluate a string and return a stack of Pfloats.
func Eval(str string) []Pfloat {
	t := Parse(str)
	if t == nil {
		return nil
	}
	return evalStack(t.(*tree.PN))
}

// Pfloat or precision float represents a value and a precision.
type Pfloat struct {
	V float64
	P int
}

// String fulfills stringer and prints the Pfloat to the correct precision.
func (p Pfloat) String() string {
	f := fmt.Sprintf("%%.%df", p.P)
	return fmt.Sprintf(f, p.V)
}

func evalStack(node *tree.PN) []Pfloat {
	kind := node.Kind().String()

	switch kind {
	case "?":
		v := evalE(node.Child(-1).(*tree.PN)).V
		node.RemoveChild(-1)
		if v > 0 {
			node.RemoveChild(-2)
			node.PromoteChild(-1)
		} else {
			node.RemoveChild(-1)
			node.PromoteChild(-1)
		}
		return evalStack(node)
	case "smp":
		evalSmp(node)
		fallthrough
	case "Stack":
		out := make([]Pfloat, len(node.C))
		for i, ch := range node.C {
			out[i] = evalE(ch)
		}
		return out
	default:
		return []Pfloat{evalE(node)}
	}
	return nil
}

func evalSmp(op *tree.PN) {
	switch op.Value() {
	case "swap":
		ln := len(op.C)
		if ln > 1 {
			op.C[ln-1], op.C[ln-2] = op.C[ln-2], op.C[ln-1]
		}
	case "drop":
		if len(op.C) > 0 {
			op.C = op.C[:len(op.C)-1]
		}
	case "clear":
		op.C = nil
	}
}

func evalE(node *tree.PN) Pfloat {
	switch node.Kind().String() {
	case "Number":
		if c := node.Children(); c == 2 {
			c1 := node.C[1].Value()
			f, _ := strconv.ParseFloat(node.C[0].Value()+c1, 64)
			return Pfloat{f, len(c1) - 1}
		} else if c == 1 {
			f, _ := strconv.ParseFloat(node.C[0].Value(), 64)
			return Pfloat{f, 0}
		}
	case "uop":
		return evalUop(node.C[0], node)
	case "bop":
		return evalBop(node.C[0], node.C[1], node)
	case "sop":
		return evalSop(evalStack(node.C[0]), node)
	}
	return Pfloat{}
}

func evalUop(a, op *tree.PN) Pfloat {
	ae := evalE(a)
	switch op.Value() {
	case "--":
		ae.V = -ae.V
	case "abs":
		if ae.V < 0 {
			ae.V = -ae.V
		}
	}
	return ae
}

func evalBop(a, b, op *tree.PN) Pfloat {
	ae := evalE(a)
	be := evalE(b)
	p := maxPrecision(ae, be)
	var v float64
	switch op.Value() {
	case "+":
		v = ae.V + be.V
	case "*":
		v = ae.V * be.V
	case "/":
		v = ae.V / be.V
	case "-":
		v = ae.V - be.V
	case "^":
		v = math.Pow(ae.V, be.V)
	case "%":
		v = math.Mod(ae.V, be.V)
	case ">":
		if ae.V > be.V {
			v = 1
		}
	case "<":
		if ae.V < be.V {
			v = 1
		}
	case "=":
		if ae.V == be.V {
			v = 1
		}
	case "cmpr":
		if ae.V > be.V {
			v = 1
		} else if ae.V < be.V {
			v = -1
		}
	}

	return Pfloat{v, p}
}

func evalSop(stack []Pfloat, op *tree.PN) Pfloat {
	var v Pfloat
	switch op := op.Value(); op {
	case "sum", "avg":
		v.P = maxPrecision(stack...)
		for _, p := range stack {
			v.V += p.V
		}
		if op == "avg" && len(stack) > 0 {
			v.V /= float64(len(stack))
		}
	case "len":
		v.V = float64(len(stack))
	case "min":
		if len(stack) > 0 {
			v = stack[0]
			for _, p := range stack[1:] {
				if p.V < v.V {
					v = p
				}
			}
		}
	case "max":
		if len(stack) > 0 {
			v = stack[0]
			for _, p := range stack[1:] {
				if p.V > v.V {
					v = p
				}
			}
		}
	case "last":
		if len(stack) > 0 {
			v = stack[0]
		}
	case "first":
		if len(stack) > 0 {
			v = stack[len(stack)-1]
		}
	}

	return v
}

func maxPrecision(pfs ...Pfloat) int {
	m := 0
	for _, p := range pfs {
		if p.P > m {
			m = p.P
		}
	}
	return m
}
