package reducer

import (
	"strconv"
	"github.com/adamcolton/parlex"
	"github.com/adamcolton/parlex/grammar/regexgram"
	"github.com/adamcolton/parlex/lexer/simplelexer"
	"github.com/adamcolton/parlex/parser/packrat"
	"github.com/adamcolton/parlex/tree"
)

const lexerRules = `
  If
  ChildIs
  PromoteChild
  PromoteChildrenOf
  PromoteChildValue
  PromoteGrandChildren
  RemoveAll
  RemoveChild
  RemoveChildren
  PromoteSingleChild
  ReplaceWithChild
  Nil
  number  /-?\d*\.?\d+/
  rule    /(\w+)/
  string  /\"([^\"\\]|(\\.))*\"/
  lp      /\(/
  rp      /\)/
  comma   /,/
  period  /\./
  comment /\/\/[^\n]*/ -
  space   /\s+/ -
`

var lxr = parlex.MustLexer(simplelexer.New(lexerRules))

const grammarRules = `
  Rules        -> Rule*
  Rule         -> rule Chain
  Chain        -> (Reduction period)* Reduction
  Reduction    -> PromoteSingleChild
               -> RemoveChildren VarNumArg
               -> PromoteChildValue OneNumArg
               -> RemoveChild OneNumArg
               -> ReplaceWithChild OneNumArg
               -> Nil
               -> If lp Condition comma Chain comma Chain rp
  VarNumArg    -> lp (number comma)* number rp
  OneNumArg    -> lp number rp
  Condition    -> ChildIsBlock
  ChildIsBlock -> ChildIs lp number comma string rp
`

var grmr, grmrRdcr = regexgram.Must(grammarRules)
var prsr = packrat.New(grmr)

var rdcr = tree.Merge(grmrRdcr, tree.Reducer{
	"Rule":         tree.PromoteChildValue(0).PromoteChildrenOf(0).RemoveAll("period"),
	"Reduction":    tree.RemoveAll("comma","lp","rp").PromoteChild(0),
	"VarNumArg":    tree.RemoveChildren(0, -1).RemoveAll("comma"),
	"OneNumArg":    tree.RemoveChildren(0, -1),
	"ChildIsBlock": tree.RemoveAll("comma","lp","rp").PromoteChild(0),
})

var runner = parlex.New(lxr, prsr, rdcr)

func Parse(str string) (tree.Reducer, error) {
	root, err := runner.Run(str)
	if err != nil {
		return nil, err
	}
	rdcr := make(tree.Reducer)
	for _,n := range root.(*tree.PN).C {
		if n.Kind().String() == "Rule" {
			k,v := evalRule(n)
			rdcr[k]=v
		}
	}
	return rdcr, nil
}

func Must(str string) tree.Reducer {
	rt, err := Parse(str)
	if err!=nil{
		panic(err)
	} 
	return rt
}

func evalRule(n *tree.PN)(string, tree.Reduction){
	return n.Value(), evalReduction(n.C...)
}

func evalReduction(ns ...*tree.PN) tree.Reduction{
	var r tree.Reduction
	for _, n := range ns {
			switch n.Kind().String(){
			case "PromoteSingleChild":
				r = r.PromoteSingleChild()
			case "RemoveChildren":
				r = r.RemoveChildren(evalVarNumArgs(n.C[0])...)
			case "PromoteChildValue":
				r = r.PromoteChildValue(evalOneNumArg(n.C[0]))
			case "RemoveChild":
				r = r.RemoveChild(evalOneNumArg(n.C[0]))
			case "ReplaceWithChild":
				r = r.ReplaceWithChild(evalOneNumArg(n.C[0]))
			case "If":
				c := evalConditional(n.C[0])
				t := evalReduction(n.C[1].C...)
				e := evalReduction(n.C[2].C...)
				r = r.If(c,t,e)
			}
	}
	return r
}

func evalVarNumArgs(n *tree.PN) []int {
	args := make( []int, len(n.C) )
	for i, n := range n.C{
		args[i], _ = strconv.Atoi(n.Value())
	}
	return args
}

func evalOneNumArg(n *tree.PN) int {
	if len(n.C)<1{
		return 0
	}
	i, _ :=strconv.Atoi(n.C[0].Value())
	return i
}

func evalConditional(n *tree.PN) tree.Condition{
	switch n.Kind().String(){
	case "ChildIs":
		i,_:=strconv.Atoi(n.C[0].Value())
		return tree.ChildIs(i, n.C[1].Value())
	}
	return nil
}