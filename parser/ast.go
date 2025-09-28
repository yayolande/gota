package parser

import (
	"log"

	"github.com/yayolande/gota/lexer"
)

// TODO: enhance the Stringer method for 'GroupStatementNode' and 'CommentNode'
// Since new fields have been added
type SymbolDefinition map[string]AstNode

//go:generate go run ./generate.go
type AstNode interface {
	String() string
	Kind() Kind
	Range() lexer.Range
	SetKind(val Kind)
	DefinitionAnalysis(globalVariables, localVariables, functionDefinitions, templateDefinitionsGlobal, templateDefinitionsLocal SymbolDefinition) []lexer.Error
	// typeAnalysis()
}

// type Kind int
type Kind int

type VariableDeclarationNode struct {
	kind          Kind
	rng           lexer.Range
	VariableNames []*lexer.Token
	Value         *MultiExpressionNode
}

func (v VariableDeclarationNode) Kind() Kind {
	return v.kind
}

func (v VariableDeclarationNode) Range() lexer.Range {
	return v.rng
}

func (v *VariableDeclarationNode) SetKind(val Kind) {
	v.kind = val
}

func (v VariableDeclarationNode) DefinitionAnalysis(globalVariables, localVariables, functionDefinitions, templateDefinitionsGlobal, templateDefinitionsLocal SymbolDefinition) []lexer.Error {
	panic("not useful anymore")
}

func NewVariableDeclarationNode(kind Kind, rng lexer.Range, variables []*lexer.Token, value *MultiExpressionNode) *VariableDeclarationNode {
	node := &VariableDeclarationNode{
		kind:          kind,
		rng:           rng,
		Value:         value,
		VariableNames: nil,
	}

	return node
}

type VariableAssignationNode struct {
	kind          Kind
	rng           lexer.Range
	VariableNames []*lexer.Token
	// Value	AstNode	// of type expression
	Value *MultiExpressionNode // of type expression
}

func (v VariableAssignationNode) Kind() Kind {
	return v.kind
}

func (v VariableAssignationNode) Range() lexer.Range {
	return v.rng
}

func (v *VariableAssignationNode) SetKind(val Kind) {
	v.kind = val
}

func (v VariableAssignationNode) DefinitionAnalysis(globalVariables, localVariables, functionDefinitions, templateDefinitionsGlobal, templateDefinitionsLocal SymbolDefinition) []lexer.Error {
	panic("not useful anymore")
}

type MultiExpressionNode struct {
	kind        Kind
	rng         lexer.Range
	Expressions []*ExpressionNode
}

func (m MultiExpressionNode) Kind() Kind {
	return m.kind
}

func (m MultiExpressionNode) Range() lexer.Range {
	return m.rng
}

func (m *MultiExpressionNode) SetKind(val Kind) {
	m.kind = val
}

func (v *MultiExpressionNode) DefinitionAnalysis(globalVariables, localVariables, functionDefinitions, templateDefinitionsGlobal, templateDefinitionsLocal SymbolDefinition) []lexer.Error {
	panic("not useful anymore")
}

type ExpressionNode struct {
	kind    Kind
	rng     lexer.Range
	Symbols []*lexer.Token
}

func (v ExpressionNode) Kind() Kind {
	return v.kind
}

func (v ExpressionNode) Range() lexer.Range {
	return v.rng
}

func (v *ExpressionNode) SetKind(val Kind) {
	v.kind = val
}

func (v ExpressionNode) DefinitionAnalysis(globalVariables, localVariables, functionDefinitions, templateDefinitionsGlobal, templateDefinitionsLocal SymbolDefinition) []lexer.Error {
	panic("not useful anymore")
}

type TemplateStatementNode struct {
	kind         Kind
	rng          lexer.Range
	TemplateName *lexer.Token
	Expression   AstNode
	parent       *GroupStatementNode
}

func (t TemplateStatementNode) Kind() Kind {
	return t.kind
}

func (t *TemplateStatementNode) SetKind(val Kind) {
	t.kind = val
}

func (t TemplateStatementNode) Range() lexer.Range {
	return t.rng
}

func (t TemplateStatementNode) Parent() *GroupStatementNode {
	return t.parent
}

func (v TemplateStatementNode) DefinitionAnalysis(globalVariables, localVariables, functionDefinitions, templateDefinitionsGlobal, templateDefinitionsLocal SymbolDefinition) []lexer.Error {
	panic("not useful anymore")
}

type groupStatementShortcut struct {
	CommentGoCode *CommentNode

	VariableDeclarations map[string]*VariableDeclarationNode

	TemplateDefined map[string]*GroupStatementNode
	// TemplateCallUsed map[string]*TemplateStatementNode
	TemplateCallUsed []*TemplateStatementNode
	// TemplateCallUnresolved	map[string]*TemplateStatementNode
	// TODO: remove this above 'TemplateCallUnresolved' and 'TemplateCallUsed'
	// instead use 'TemplateCalls' to describe the union of both
}

type GroupStatementNode struct {
	kind        Kind
	rng         lexer.Range
	parent      *GroupStatementNode // use 'isRoot' to check that the node is the ROOT
	ControlFlow AstNode
	Statements  []AstNode
	// ShortcutsNode struct { TemplateDefine, TemplateUse, VarDeclaration, CommentGoCode }
	ShortCut groupStatementShortcut
	isRoot   bool // only this is consistently enforced to determine whether a node is ROOT or not
}

func NewGroupStatementNode(kind Kind, reach lexer.Range) *GroupStatementNode {
	scope := &GroupStatementNode{
		kind:   kind,
		rng:    reach,
		isRoot: false,
	}

	scope.ShortCut.TemplateDefined = make(map[string]*GroupStatementNode)
	scope.ShortCut.TemplateCallUsed = make([]*TemplateStatementNode, 0)
	scope.ShortCut.VariableDeclarations = make(map[string]*VariableDeclarationNode)

	return scope
}

func (g GroupStatementNode) Kind() Kind {
	return g.kind
}

func (g GroupStatementNode) Range() lexer.Range {
	return g.rng
}

func (g *GroupStatementNode) SetKind(val Kind) {
	g.kind = val
}

func (g GroupStatementNode) Parent() *GroupStatementNode {
	return g.parent
}

func (g GroupStatementNode) IsRoot() bool {
	return g.isRoot
}

func (g GroupStatementNode) TemplateNameToken() *lexer.Token {
	if !g.IsTemplate() {
		log.Printf("impossible to find the template name because the node in question is not a template definition\n group = %#v", g)
		panic("impossible to find the template name because the node in question is not a template definition")
	}

	templateNode := g.ControlFlow.(*TemplateStatementNode)
	if templateNode == nil {
		log.Println("template contains structure, and expected a valid 'ControlFlow' but instead got <nil>\n group = ", g)
		panic("template contains structure, and expected a valid 'ControlFlow' but instead got <nil>")
	}

	return templateNode.TemplateName
}

func (g GroupStatementNode) TemplateName() string {
	return string(g.TemplateNameToken().Value)
}

func (g GroupStatementNode) IsTemplate() bool {
	ok := false
	ok = ok || g.kind == KIND_DEFINE_TEMPLATE
	ok = ok || g.kind == KIND_BLOCK_TEMPLATE

	if !ok {
		return false
	}

	// make the template definition follow the standard
	if g.ControlFlow == nil {
		log.Printf("a template definition cannot have a 'nil' 'ControlFlow'\n"+
			"templateGroup = %s", g)
		panic("a template definition cannot have a 'nil' 'ControlFlow'")
	}

	// make the template definition follow the standard
	if _, ok := g.ControlFlow.(*TemplateStatementNode); !ok {
		log.Printf("template definition cannot have any 'ControlFlow' "+
			"other than of type '*TemplateStatementNode'\n"+
			"templateGroup = %s", g)
		panic("template definition cannot have any 'ControlFlow' " +
			"other than of type '*TemplateStatementNode'")
	}

	return true
}

func (g GroupStatementNode) IsGroupWithControlFlow() bool {
	switch g.kind {
	case KIND_IF, KIND_ELSE_IF, KIND_RANGE_LOOP, KIND_WITH,
		KIND_ELSE_WITH, KIND_DEFINE_TEMPLATE, KIND_BLOCK_TEMPLATE:

		return true
	}

	return false
}

func (g GroupStatementNode) IsGroupWithNoVariableReset() bool {
	switch g.kind {
	case KIND_IF, KIND_ELSE, KIND_ELSE_IF, KIND_END:

		return true
	}

	return false
}

func (g GroupStatementNode) IsGroupWithDotVariableReset() bool {

	switch g.kind {
	case KIND_RANGE_LOOP, KIND_WITH, KIND_ELSE_WITH:

		return true
	}

	return false
}

func (g GroupStatementNode) IsGroupWithDollarAndDotVariableReset() bool {
	switch g.kind {
	case KIND_DEFINE_TEMPLATE, KIND_BLOCK_TEMPLATE, KIND_GROUP_STATEMENT:

		return true
	}

	return false
}

func (v GroupStatementNode) DefinitionAnalysis(globalVariables, localVariables, functionDefinitions, templateDefinitionsGlobal, templateDefinitionsLocal SymbolDefinition) []lexer.Error {
	panic("not useful anymore")
}

type CommentNode struct {
	kind  Kind
	rng   lexer.Range
	Value *lexer.Token
	// GoCode			[]byte
	GoCode *lexer.Token
	// TODO: add those field for 'DefinitionAnalysis()'
}

func (c CommentNode) Kind() Kind {
	return c.kind
}

func (c CommentNode) Range() lexer.Range {
	return c.rng
}

func (v *CommentNode) SetKind(val Kind) {
	v.kind = val
}

func (v CommentNode) DefinitionAnalysis(globalVariables, localVariables, functionDefinitions, templateDefinitionsGlobal, templateDefinitionsLocal SymbolDefinition) []lexer.Error {
	panic("not useful anymore")
}

// --------------
// --------------
// Tree Traversal
// --------------
// --------------

type Visitor interface {
	Visit(node AstNode) Visitor
	SetHeaderFlag(ok bool)
}

func Walk(action Visitor, node AstNode) {
	if action.Visit(node) == nil {
		return
	}

	switch n := node.(type) {
	case *GroupStatementNode:

		action.SetHeaderFlag(true)

		if n.ControlFlow != nil {
			Walk(action, n.ControlFlow)
		}

		action.SetHeaderFlag(false)

		for _, statement := range n.Statements {
			Walk(action, statement)
		}

	case *TemplateStatementNode:
		Walk(action, n.Expression)

	case *VariableDeclarationNode:
		Walk(action, n.Value)

	case *VariableAssignationNode:
		Walk(action, n.Value)

	case *MultiExpressionNode:
		for _, expression := range n.Expressions {
			Walk(action, expression)
		}

	case *ExpressionNode:
		//	do nothing

	case *CommentNode:
		// do nothing

	default:
		log.Printf("unknown type for the traversal.\n node = %#v\n", n)
		panic("unknown type for the traversal")
	}

	action.Visit(nil)
}
