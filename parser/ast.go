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
	SetKind(val Kind)
	Range() lexer.Range
	SetRange(lexer.Range)
	Error() *ParseError
	SetError(err *ParseError)
}

// type Kind int
type Kind int

type VariableDeclarationNode struct {
	kind          Kind
	rng           lexer.Range
	VariableNames []*lexer.Token
	Value         *MultiExpressionNode
	Err           *ParseError
}

func (v VariableDeclarationNode) Kind() Kind {
	return v.kind
}

func (v VariableDeclarationNode) Range() lexer.Range {
	return v.rng
}

func (v *VariableDeclarationNode) SetRange(reach lexer.Range) {
	v.rng = reach
}

func (v *VariableDeclarationNode) SetKind(val Kind) {
	v.kind = val
}

func (v VariableDeclarationNode) Error() *ParseError {
	return v.Err
}

func (v *VariableDeclarationNode) SetError(err *ParseError) {
	v.Err = err
}

func NewVariableDeclarationNode(kind Kind, start, end lexer.Position, err *ParseError) *VariableDeclarationNode {
	node := &VariableDeclarationNode{
		kind: kind,
		rng:  lexer.Range{Start: start, End: end},
		Err:  nil,
	}

	return node
}

type VariableAssignationNode struct {
	kind          Kind
	rng           lexer.Range
	VariableNames []*lexer.Token
	Value         *MultiExpressionNode // of type expression
	Err           *ParseError
}

func (v VariableAssignationNode) Kind() Kind {
	return v.kind
}

func (v VariableAssignationNode) Range() lexer.Range {
	return v.rng
}

func (v *VariableAssignationNode) SetRange(reach lexer.Range) {
	v.rng = reach
}

func (v *VariableAssignationNode) SetKind(val Kind) {
	v.kind = val
}

func (v VariableAssignationNode) Error() *ParseError {
	return v.Err
}

func (v *VariableAssignationNode) SetError(err *ParseError) {
	v.Err = err
}

func NewVariableAssignmentNode(kind Kind, start, end lexer.Position, err *ParseError) *VariableAssignationNode {
	node := &VariableAssignationNode{
		kind:  kind,
		rng:   lexer.Range{Start: start, End: end},
		Err:   err,
		Value: nil,
	}

	return node
}

type MultiExpressionNode struct {
	kind        Kind
	rng         lexer.Range
	Expressions []*ExpressionNode
	Err         *ParseError
}

func (m MultiExpressionNode) Kind() Kind {
	return m.kind
}

func (m MultiExpressionNode) Range() lexer.Range {
	return m.rng
}

func (m *MultiExpressionNode) SetRange(reach lexer.Range) {
	m.rng = reach
}

func (m *MultiExpressionNode) SetKind(val Kind) {
	m.kind = val
}

func (m MultiExpressionNode) Error() *ParseError {
	return m.Err
}

func (m *MultiExpressionNode) SetError(err *ParseError) {
	m.Err = err
}

func NewMultiExpressionNode(kind Kind, start, end lexer.Position, err *ParseError) *MultiExpressionNode {
	node := &MultiExpressionNode{
		kind: kind,
		rng:  lexer.Range{Start: start, End: end},
		Err:  err,
	}

	return node
}

type ExpressionNode struct {
	kind           Kind
	rng            lexer.Range
	Symbols        []*lexer.Token
	ExpandedTokens []AstNode
	Err            *ParseError
}

func (v ExpressionNode) Kind() Kind {
	return v.kind
}

func (v ExpressionNode) Range() lexer.Range {
	return v.rng
}

func (v *ExpressionNode) SetRange(reach lexer.Range) {
	v.rng = reach
}

func (v *ExpressionNode) SetKind(val Kind) {
	v.kind = val
}

func (v ExpressionNode) Error() *ParseError {
	return v.Err
}

func (v *ExpressionNode) SetError(err *ParseError) {
	v.Err = err
}

func NewExpressionNode(kind Kind, reach lexer.Range) *ExpressionNode {
	node := &ExpressionNode{
		kind:           kind,
		rng:            reach,
		Symbols:        nil,
		Err:            nil,
		ExpandedTokens: make([]AstNode, 0),
	}

	return node
}

type TemplateStatementNode struct {
	kind         Kind
	rng          lexer.Range
	TemplateName *lexer.Token
	Expression   AstNode
	parent       *GroupStatementNode
	KeywordRange lexer.Range
	Err          *ParseError
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

func (t *TemplateStatementNode) SetRange(reach lexer.Range) {
	t.rng = reach
}

func (t TemplateStatementNode) Parent() *GroupStatementNode {
	return t.parent
}

func (t TemplateStatementNode) Error() *ParseError {
	return t.Err
}

func (t *TemplateStatementNode) SetError(err *ParseError) {
	t.Err = err
}

func NewTemplateStatementNode(kind Kind, reach lexer.Range) *TemplateStatementNode {
	node := &TemplateStatementNode{
		kind:   kind,
		rng:    reach,
		parent: nil,
	}

	return node
}

type groupStatementShortcut struct {
	CommentGoCode        *CommentNode
	VariableDeclarations map[string]*VariableDeclarationNode
	TemplateDefined      map[string]*GroupStatementNode
	TemplateCallUsed     []*TemplateStatementNode
	// TemplateCallUsed map[string]*TemplateStatementNode
}

type GroupStatementNode struct {
	kind        Kind // useful to know the group keyword ('if', 'with', ...)
	rng         lexer.Range
	parent      *GroupStatementNode // use 'isRoot' to check that the node is the ROOT
	ControlFlow AstNode
	Statements  []AstNode
	ShortCut    groupStatementShortcut

	Err                *ParseError
	KeywordRange       lexer.Range
	KeywordToken       *lexer.Token
	StreamToken        *lexer.StreamToken
	NextLinkedSibling  *GroupStatementNode
	IsProcessingHeader bool
	isRoot             bool // only this is consistently enforced to determine whether a node is ROOT or not
}

func NewGroupStatementNode(kind Kind, scopeRange lexer.Range, tokenStream *lexer.StreamToken) *GroupStatementNode {
	scope := &GroupStatementNode{
		kind:               kind,
		rng:                scopeRange,
		isRoot:             false,
		IsProcessingHeader: false,
		Err:                nil,
		StreamToken:        tokenStream,
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

func (g *GroupStatementNode) SetRange(reach lexer.Range) {
	g.rng = reach
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

func (g GroupStatementNode) Error() *ParseError {
	return g.Err
}

func (g *GroupStatementNode) SetError(err *ParseError) {
	g.Err = err
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

	if IsGroupNode(g.kind) == false {
		return false
	}

	if g.isRoot {
		return true
	}

	if g.Err != nil {
		return true
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

// WARNING: The name of this function is not accurate
func IsGroupNode(kind Kind) bool {
	ok := false
	ok = ok || kind == KIND_GROUP_STATEMENT
	ok = ok || kind == KIND_DEFINE_TEMPLATE
	ok = ok || kind == KIND_BLOCK_TEMPLATE

	return ok
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

type CommentNode struct {
	kind   Kind
	rng    lexer.Range
	Value  *lexer.Token
	GoCode *lexer.Token
	Err    *ParseError
}

func (c CommentNode) Kind() Kind {
	return c.kind
}

func (c CommentNode) Range() lexer.Range {
	return c.rng
}

func (c *CommentNode) SetRange(reach lexer.Range) {
	c.rng = reach
}

func (c *CommentNode) SetKind(val Kind) {
	c.kind = val
}

func (c CommentNode) Error() *ParseError {
	return c.Err
}

func (c *CommentNode) SetError(err *ParseError) {
	c.Err = err
}

// keyword involved: break, continue
type SpecialCommandNode struct {
	kind   Kind
	rng    lexer.Range
	Value  *lexer.Token
	Err    *ParseError
	Target *GroupStatementNode // must be non <nil>
}

func (s SpecialCommandNode) Kind() Kind {
	return s.kind
}

func (s SpecialCommandNode) Range() lexer.Range {
	return s.rng
}

func (s *SpecialCommandNode) SetRange(reach lexer.Range) {
	s.rng = reach
}

func (s *SpecialCommandNode) SetKind(val Kind) {
	s.kind = val
}

func (s SpecialCommandNode) Error() *ParseError {
	return s.Err
}

func (s *SpecialCommandNode) SetError(err *ParseError) {
	s.Err = err
}

func NewSpecialCommandNode(kind Kind, val *lexer.Token, reach lexer.Range) *SpecialCommandNode {
	node := &SpecialCommandNode{
		kind:  kind,
		rng:   reach,
		Value: val,
		Err:   nil,
	}

	return node
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
		if n.Expression != nil {
			Walk(action, n.Expression)
		}

	case *VariableDeclarationNode:
		if n.Value != nil {
			Walk(action, n.Value)
		}

	case *VariableAssignationNode:
		if n.Value != nil {
			Walk(action, n.Value)
		}

	case *MultiExpressionNode:
		for _, expression := range n.Expressions {
			Walk(action, expression)
		}

	case *ExpressionNode:
		for _, subexpression := range n.ExpandedTokens {
			if subexpression == nil {
				continue
			}

			Walk(action, subexpression)
		}

	case *CommentNode:
		// do nothing

	case *SpecialCommandNode:
		// do nothing

	default:
		log.Printf("unknown type for the traversal.\n node = %#v\n", n)
		panic("unknown type for the traversal")
	}

	action.Visit(nil)
}
