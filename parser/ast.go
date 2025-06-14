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
	GetKind() Kind
	GetRange() lexer.Range
	SetKind(val Kind)
	DefinitionAnalysis(globalVariables, localVariables, functionDefinitions, templateDefinitionsGlobal, templateDefinitionsLocal SymbolDefinition) []lexer.Error
	// typeAnalysis()
}

// type Kind int
type Kind int

type VariableDeclarationNode struct {
	Kind          Kind
	Range         lexer.Range
	VariableNames []*lexer.Token
	Value         *MultiExpressionNode
}

func (v VariableDeclarationNode) GetKind() Kind {
	return v.Kind
}

func (v VariableDeclarationNode) GetRange() lexer.Range {
	return v.Range
}

func (v *VariableDeclarationNode) SetKind(val Kind) {
	v.Kind = val
}

func (v VariableDeclarationNode) DefinitionAnalysis(globalVariables, localVariables, functionDefinitions, templateDefinitionsGlobal, templateDefinitionsLocal SymbolDefinition) []lexer.Error {
	panic("not useful anymore")
}

type VariableAssignationNode struct {
	Kind         Kind
	Range        lexer.Range
	VariableName *lexer.Token
	// Value	AstNode	// of type expression
	Value *MultiExpressionNode // of type expression
}

func (v VariableAssignationNode) GetKind() Kind {
	return v.Kind
}

func (v VariableAssignationNode) GetRange() lexer.Range {
	return v.Range
}

func (v *VariableAssignationNode) SetKind(val Kind) {
	v.Kind = val
}

func (v VariableAssignationNode) DefinitionAnalysis(globalVariables, localVariables, functionDefinitions, templateDefinitionsGlobal, templateDefinitionsLocal SymbolDefinition) []lexer.Error {
	panic("not useful anymore")
}

type MultiExpressionNode struct {
	Kind
	Range       lexer.Range
	Expressions []*ExpressionNode
}

func (m MultiExpressionNode) GetKind() Kind {
	return m.Kind
}

func (m MultiExpressionNode) GetRange() lexer.Range {
	return m.Range
}

func (m *MultiExpressionNode) SetKind(val Kind) {
	m.Kind = val
}

func (v *MultiExpressionNode) DefinitionAnalysis(globalVariables, localVariables, functionDefinitions, templateDefinitionsGlobal, templateDefinitionsLocal SymbolDefinition) []lexer.Error {
	panic("not useful anymore")
}

type ExpressionNode struct {
	Kind
	Range   lexer.Range
	Symbols []*lexer.Token
}

func (v ExpressionNode) GetKind() Kind {
	return v.Kind
}

func (v ExpressionNode) GetRange() lexer.Range {
	return v.Range
}

func (v *ExpressionNode) SetKind(val Kind) {
	v.Kind = val
}

func (v ExpressionNode) DefinitionAnalysis(globalVariables, localVariables, functionDefinitions, templateDefinitionsGlobal, templateDefinitionsLocal SymbolDefinition) []lexer.Error {
	panic("not useful anymore")
}

type TemplateStatementNode struct {
	Kind
	Range        lexer.Range
	TemplateName *lexer.Token
	Expression   AstNode
	parent       *GroupStatementNode
}

func (t TemplateStatementNode) GetKind() Kind {
	return t.Kind
}

func (t *TemplateStatementNode) SetKind(val Kind) {
	t.Kind = val
}

func (t TemplateStatementNode) GetRange() lexer.Range {
	return t.Range
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

	TemplateDefined  map[string]*GroupStatementNode
	TemplateCallUsed map[string]*TemplateStatementNode
	// TemplateCallUnresolved	map[string]*TemplateStatementNode
	// TODO: remove this above 'TemplateCallUnresolved' and 'TemplateCallUsed'
	// instead use 'TemplateCalls' to describe the union of both
}

type GroupStatementNode struct {
	Kind
	Range       lexer.Range
	parent      *GroupStatementNode // use 'isRoot' to check that the node is the ROOT
	ControlFlow AstNode
	Statements  []AstNode
	// ShortcutsNode struct { TemplateDefine, TemplateUse, VarDeclaration, CommentGoCode }
	ShortCut groupStatementShortcut
	isRoot   bool // only this is consistently enforced to determine whether a node is ROOT or not
}

func NewGroupStatementNode(kind Kind, reach lexer.Range) *GroupStatementNode {
	scope := &GroupStatementNode{
		Kind:   kind,
		Range:  reach,
		isRoot: false,
	}

	scope.ShortCut.TemplateDefined = make(map[string]*GroupStatementNode)
	scope.ShortCut.TemplateCallUsed = make(map[string]*TemplateStatementNode)
	scope.ShortCut.VariableDeclarations = make(map[string]*VariableDeclarationNode)

	return scope
}

func (g GroupStatementNode) GetKind() Kind {
	return g.Kind
}

func (g GroupStatementNode) GetRange() lexer.Range {
	return g.Range
}

func (g *GroupStatementNode) SetKind(val Kind) {
	g.Kind = val
}

func (g GroupStatementNode) Parent() *GroupStatementNode {
	return g.parent
}

func (g GroupStatementNode) IsRoot() bool {
	return g.isRoot
}

/*
func (g *GroupStatementNode) SetRoot(val bool) {
	g.isRoot = val
}
*/

func (g GroupStatementNode) IsTemplate() bool {
	ok := false
	ok = ok || g.Kind == KIND_DEFINE_TEMPLATE
	ok = ok || g.Kind == KIND_BLOCK_TEMPLATE

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

func (v GroupStatementNode) DefinitionAnalysis(globalVariables, localVariables, functionDefinitions, templateDefinitionsGlobal, templateDefinitionsLocal SymbolDefinition) []lexer.Error {
	panic("not useful anymore")
}

type CommentNode struct {
	Kind
	Range lexer.Range
	Value *lexer.Token
	// GoCode			[]byte
	GoCode *lexer.Token
	// TODO: add those field for 'DefinitionAnalysis()'
}

func (c CommentNode) GetKind() Kind {
	return c.Kind
}

func (c CommentNode) GetRange() lexer.Range {
	return c.Range
}

func (v *CommentNode) SetKind(val Kind) {
	v.Kind = val
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
}

func Walk(action Visitor, node AstNode) {
	if action.Visit(node) == nil {
		return
	}

	switch n := node.(type) {
	case *GroupStatementNode:
		if n.ControlFlow != nil {
			Walk(action, n.ControlFlow)
		}

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
