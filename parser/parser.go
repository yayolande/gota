package parser

import (
	"bytes"
	"errors"
	"fmt"
	"log" // TODO: to remove
	"strconv"

	"github.com/yayolande/gota/lexer"
)

var UNIQUE_UNIVERSAL_COUNTER int = 0

// Call this function instead of interacting with 'UNIQUE_UNIVERSAL_COUNTER' instead.
// Its goal is to return an unique 'int' at each call.
// In other word, this function guaranty that that 2 function call will ever have the same value.
// Very useful to generate number that must never enter in collision.
func GetUniqueNumber() int {
	UNIQUE_UNIVERSAL_COUNTER++
	return UNIQUE_UNIVERSAL_COUNTER
}

type ParseError struct {
	Err   error
	Range lexer.Range
	Token *lexer.Token
}

func (p ParseError) GetError() string {
	return p.Err.Error()
}

func (p ParseError) GetRange() lexer.Range {
	return p.Range
}

type Parser struct {
	stream            *lexer.StreamToken
	lastToken         *lexer.Token // token before 'EOL', and only computed once at 'Reset()'
	indexCurrentToken int
	sizeStream        int

	maxRecursionDepth     int
	currentRecursionDepth int
}

func (p *Parser) Reset(streamOfToken *lexer.StreamToken) {
	p.lastToken = nil
	p.stream = streamOfToken
	p.sizeStream = len(streamOfToken.Tokens)
	p.indexCurrentToken = 0

	if p.sizeStream >= 2 {
		p.lastToken = &p.stream.Tokens[p.sizeStream-2] // careful, last token is 'EOL'
	}

	if p.sizeStream < 1 { // even an empty statement have 'EOL'
		panic("every token stream must at least have 'EOL' token, even an empty statement")
	}

	p.maxRecursionDepth = 15
	p.currentRecursionDepth = 0
}

func appendParseError(errs []lexer.Error, err *ParseError) []lexer.Error {
	if err == nil {
		return errs
	}

	return append(errs, err)
}

func appendStatementToScopeShortcut(scope *GroupStatementNode, statement AstNode) *ParseError {
	switch stmt := statement.(type) {
	case *GroupStatementNode:
		if stmt.IsTemplate() == false {
			return nil
		}

		if stmt.ControlFlow == nil && stmt.Err != nil {
			return nil
		} else if stmt.ControlFlow == nil {
			panic("expected non <nil> ControlFlow for 'GroupStatementNode' while appending it to parent scope")
		}

		templateNode := stmt.ControlFlow.(*TemplateStatementNode)
		if templateNode == nil || templateNode.TemplateName == nil {
			return nil
		}

		templateName := string(templateNode.TemplateName.Value)

		if scope.ShortCut.TemplateDefined[templateName] != nil {
			err := NewParseError(templateNode.TemplateName, errors.New("template already defined"))
			return err
		}

		scope.ShortCut.TemplateDefined[templateName] = stmt

	case *TemplateStatementNode:
		if stmt.TemplateName == nil {
			return nil
		}

		// Look for template group that will hold the template call shortcut
		// This assume the root group is always available for the program to not crash
		for IsGroupNode(scope.kind) == false {
			scope = scope.parent
		}

		scope.ShortCut.TemplateCallUsed = append(scope.ShortCut.TemplateCallUsed, stmt)

	case *CommentNode:
		if stmt.GoCode == nil {
			return nil
		}

		if scope.ShortCut.CommentGoCode != nil {
			// stmt.GoCode = nil
			err := errors.New("cannot redeclare 'go:code' in the same scope")
			return NewParseError(stmt.Value, err)
		}

		scope.ShortCut.CommentGoCode = stmt

	case *VariableDeclarationNode:
		for _, variableToken := range stmt.VariableNames {
			variableName := string(variableToken.Value)

			scope.ShortCut.VariableDeclarations[variableName] = stmt
		}
	}

	return nil
}

func appendStatementToCurrentScope(scope *GroupStatementNode, statement AstNode) {
	if statement == nil {
		panic("cannot add empty statement to 'group'")
	}

	if scope == nil {
		panic("cannot add statement to empty group. Group must be created before hand")
	}

	scope.Statements = append(scope.Statements, statement)
	scope.rng.End = statement.Range().End
}

type groupMerger struct {
	openedNodeStack []*GroupStatementNode
	topLinkedGroup  []*GroupStatementNode // list of group oponer only
}

func newGroupMerger() *groupMerger {
	rootGroup := NewGroupStatementNode(KIND_GROUP_STATEMENT, lexer.Range{}, nil)
	rootGroup.isRoot = true

	groupNodeStack := []*GroupStatementNode{}
	groupNodeStack = append(groupNodeStack, rootGroup)

	merger := &groupMerger{
		openedNodeStack: groupNodeStack,
		topLinkedGroup:  nil,
	}

	return merger
}

func (p *groupMerger) safelyGroupStatement(node AstNode) *ParseError {
	if node == nil {
		log.Printf("cannot add <nil> AST to group")
		panic("cannot add <nil> AST to group")
	}

	// TODO: change var name to : "openedGroupNodeStack", "activeScopeNodes", "activeScopeStack", "openedScopeStack"
	if len(p.openedNodeStack) == 0 {
		panic("no initial scope available to hold the statements. There must always exist at least one 'scope/group' at any moment")
	}

	if p.openedNodeStack[0].isRoot == false {
		panic("root node has'nt been marked as such. root node, and only the root node, can be marked as 'isRoot'")
	}

	// this is only useful to keep track of the top most group, so that 'end' statement can link to it later
	if len(p.topLinkedGroup) != len(p.openedNodeStack)-1 {
		panic("size of open node stack do not match the one only containing opener group (eg. if, range, with, define)")
	}

	var err *ParseError
	var ROOT_SCOPE *GroupStatementNode = p.openedNodeStack[0]

	stackSize := len(p.openedNodeStack)
	currentScope := p.openedNodeStack[stackSize-1]

	newScope, isScope := node.(*GroupStatementNode)

	if isScope == false {
		appendStatementToCurrentScope(currentScope, node)
		err = appendStatementToScopeShortcut(currentScope, node)

		switch node.Kind() {
		case KIND_CONTINUE, KIND_BREAK:
			loopControl, ok := node.(*SpecialCommandNode)
			if ok == false {
				panic("expected 'SpecialCommandNode' to safelyGroupStatement, but found something else :: " + node.String())
			}

			found := false

			for index := len(p.openedNodeStack) - 1; index >= 0; index-- { // reverse looping
				scope := p.openedNodeStack[index]

				if scope.kind == KIND_RANGE_LOOP {
					loopControl.Target = scope
					found = true

					break
				}
			}

			if found == false {
				err = NewParseError(&lexer.Token{}, errors.New("missing 'range' loop ancestor"))
				err.Range = node.Range()
			}
		}

	} else {
		if newScope.IsRoot() {
			log.Printf("non-root node cannot be flaged as 'root'.\n culprit node = %#v\n", newScope)
			panic("only the root node, can be marked as 'isRoot', but found it on non-root node")
		}

		newScope.parent = currentScope
		isStatementAlreadyAppendedToParentScope := false

		switch newScope.Kind() {
		case KIND_IF, KIND_WITH, KIND_RANGE_LOOP, KIND_BLOCK_TEMPLATE, KIND_DEFINE_TEMPLATE:
			err = appendStatementToScopeShortcut(currentScope, newScope)
			appendStatementToCurrentScope(currentScope, newScope)
			isStatementAlreadyAppendedToParentScope = true
			newScope.parent = currentScope

			p.openedNodeStack = append(p.openedNodeStack, newScope)
			p.topLinkedGroup = append(p.topLinkedGroup, newScope)

		case KIND_ELSE_IF:
			if stackSize >= 2 {
				switch currentScope.Kind() {
				case KIND_IF, KIND_ELSE_IF: // Remove the last element from the stack and switch it with 'KIND_ELSE_IF' scope
					scopeToClose := currentScope
					scopeToClose.rng.End = newScope.Range().Start
					scopeToClose.NextLinkedSibling = newScope

					parentScope := p.openedNodeStack[stackSize-2]
					p.openedNodeStack = p.openedNodeStack[:stackSize-1]

					newScope.parent = parentScope
					isStatementAlreadyAppendedToParentScope = true
					appendStatementToCurrentScope(parentScope, newScope)

					p.openedNodeStack = append(p.openedNodeStack, newScope)

					size := len(p.topLinkedGroup)
					newScope.NextLinkedSibling = p.topLinkedGroup[size-1]

				default:
					err = &ParseError{Range: newScope.KeywordRange, // currentScope.Range(),
						Err: errors.New("not compatible with " + currentScope.Kind().String())}
				}
			} else {
				err = &ParseError{Range: newScope.KeywordRange, // newScope.Range(),
					Err: errors.New("extraneous statement '" + newScope.Kind().String() + "'")}
			}
		case KIND_ELSE_WITH:
			if stackSize >= 2 {
				switch currentScope.Kind() {
				case KIND_WITH, KIND_ELSE_WITH:
					scopeToClose := currentScope
					scopeToClose.rng.End = newScope.Range().Start
					scopeToClose.NextLinkedSibling = newScope

					// Remove the last element from the stack and switch it with 'KIND_ELSE_WITH' scope
					parentScope := p.openedNodeStack[stackSize-2]
					p.openedNodeStack = p.openedNodeStack[:stackSize-1] // fold current scope

					newScope.parent = parentScope
					isStatementAlreadyAppendedToParentScope = true
					appendStatementToCurrentScope(parentScope, newScope)

					p.openedNodeStack = append(p.openedNodeStack, newScope)

					size := len(p.topLinkedGroup)
					newScope.NextLinkedSibling = p.topLinkedGroup[size-1]

				default:
					err = &ParseError{Range: newScope.KeywordRange, // currentScope.Range(),
						Err: errors.New("not compatible with " + currentScope.Kind().String())}
				}
			} else {
				err = &ParseError{Range: newScope.KeywordRange, // newScope.Range(),
					Err: errors.New("extraneous statement '" + newScope.Kind().String() + "'")}
			}
		case KIND_ELSE:
			if stackSize >= 2 {
				switch currentScope.Kind() {
				case KIND_IF, KIND_ELSE_IF, KIND_WITH, KIND_ELSE_WITH, KIND_RANGE_LOOP:
					// Remove the last element from the stack and switch it with 'KIND_ELSE' scope

					scopeToClose := currentScope
					scopeToClose.rng.End = newScope.Range().Start
					scopeToClose.NextLinkedSibling = newScope

					parentScope := p.openedNodeStack[stackSize-2]
					p.openedNodeStack = p.openedNodeStack[:stackSize-1] // fold previous scope

					newScope.parent = parentScope
					isStatementAlreadyAppendedToParentScope = true
					appendStatementToCurrentScope(parentScope, newScope)

					p.openedNodeStack = append(p.openedNodeStack, newScope)

					size := len(p.topLinkedGroup)
					newScope.NextLinkedSibling = p.topLinkedGroup[size-1]

				default:
					err = &ParseError{Range: newScope.KeywordRange, // newScope.Range(),
						Err: errors.New("not compatible with " + currentScope.Kind().String())}
				}
			} else {
				err = &ParseError{Range: newScope.KeywordRange, // newScope.Range(),
					Err: errors.New("extraneous statement '" + newScope.Kind().String() + "'")}
			}
		case KIND_END:
			if stackSize >= 2 {
				scopeToClose := currentScope
				scopeToClose.rng.End = newScope.Range().Start
				scopeToClose.NextLinkedSibling = newScope

				parentScope := p.openedNodeStack[stackSize-2]
				p.openedNodeStack = p.openedNodeStack[:stackSize-1] // fold/close current scope

				newScope.parent = parentScope
				isStatementAlreadyAppendedToParentScope = true
				appendStatementToCurrentScope(parentScope, newScope)

				size := len(p.topLinkedGroup)
				newScope.NextLinkedSibling = p.topLinkedGroup[size-1]

				p.topLinkedGroup = p.topLinkedGroup[:size-1]

			} else {
				err = &ParseError{Range: newScope.Range(), Err: errors.New("extraneous 'end' statement")}
			}
		default:
			log.Printf("unhandled scope type error\n scope = %#v\n", newScope)
			panic("scope type '" + newScope.Kind().String() + "' is not yet handled for statement grouping\n" + newScope.String())
		}

		// only do this if for some reasons the statement hasn't been added to any existing scope
		if isStatementAlreadyAppendedToParentScope == false {
			appendStatementToCurrentScope(currentScope, newScope)
		}
	}

	if len(p.openedNodeStack) == 0 {
		panic("'openedNodeStack' cannot be empty ! you have inadvertly close the 'root scope'. You should not interact with it")
	}

	if ROOT_SCOPE != p.openedNodeStack[0] {
		log.Printf("root scope change error. new Root = %#v\n", p.openedNodeStack[0])
		panic("error, the root scope have been modified. The root scope should never change under any circumstance")
	}

	return err
}

// Parse tokens into AST and return syntax errors found during the process
func Parse(streams []*lexer.StreamToken) (*GroupStatementNode, []lexer.Error) {
	if streams == nil {
		return nil, nil
	}

	var errs []lexer.Error

	merger := newGroupMerger()
	parser := Parser{}

	// main processing
	for _, stream := range streams {
		parser.Reset(stream)

		node, err := parser.ParseStatement()
		node.SetError(err)

		if stream.Err == nil { // otherwise do not report the error since it was already done in lexing
			errs = appendParseError(errs, err)
		}

		if node == nil {
			log.Printf("unexpected <nil> AST. Even partial ASP must be non <nil> for better code analysis\n statementToken = %#v\n fileToken = %#v\n", stream, streams)
			panic("unexpected <nil> AST. Even partial ASP must be non <nil> for better code analysis")
		}

		err = merger.safelyGroupStatement(node)
		errs = appendParseError(errs, err)
	}

	if len(merger.openedNodeStack) == 0 {
		log.Printf("fatal error while building the parse tree. Expected at least one scope/group but found nothing\n stream = %#v", streams)
		panic("fatal error while building the parse tree. Expected at least one scope/group but found nothing")
	}

	var unclosedScopes []*GroupStatementNode = nil
	if size := len(merger.openedNodeStack); size > 1 {
		unclosedScopes = merger.openedNodeStack[1:]
	}

	// for _, currentScope := range unclosedScopes {
	for index := len(unclosedScopes) - 1; index >= 0; index-- {
		// very imporant to start from the back
		// since the last element have the most accurate end of range value
		currentScope := unclosedScopes[index]

		size := len(currentScope.Statements)
		if size > 0 {
			lastStatement := currentScope.Statements[size-1]
			currentScope.rng.End = lastStatement.Range().End
		}

		err := NewParseError(currentScope.KeywordToken, errors.New("missing matching '{{ end }}' statement"))
		errs = append(errs, err)
	}

	defaultGroupStatementNode := merger.openedNodeStack[0]
	if size := len(defaultGroupStatementNode.Statements); size > 0 {
		defaultGroupStatementNode.rng.Start = defaultGroupStatementNode.Statements[0].Range().Start
		defaultGroupStatementNode.rng.End = defaultGroupStatementNode.Statements[size-1].Range().End
	}

	return defaultGroupStatementNode, errs
}

// Parse statement form tokens. It is the function that does the real parsing work.
// Whenever parsing fail, you must call 'flushInputUntilNextStatement()' method
// to parse the next statement accurately or else you might be in for a nasty surprise.
// NB: when an error occur, the returned 'ast' can be nil or not. Most of the time,
// the ast will be partially constructed instead of being nil.
// Thus always use the returned error value to deternime whether parsing completed succesfully.
// NB: you should not call this function directly, unless you want to build a costum parsing procedure
// instead it is recommened to use 'Parse()' for regular use cases
func (p *Parser) ParseStatement() (ast AstNode, er *ParseError) {
	if p.stream.IsEmpty() {
		err := NewParseError(p.peek(), errors.New("empty statement"))
		multiExpression := NewMultiExpressionNode(KIND_MULTI_EXPRESSION, p.peek().Range.Start, p.peek().Range.End, err)
		return multiExpression, err
	}

	// 1. Escape infinite recursion
	p.incRecursionDepth()
	defer p.decRecursionDepth()

	if multi, err := p.checkRecursionStatus(); err != nil {
		return multi, err
	}

	if p.lastToken == nil {
		panic("unexpected empty token found at end of the current instruction")
	}

	// 3. Syntax Parser for the Go Template language
	if p.accept(lexer.KEYWORD) {
		keywordToken := p.peek()

		// INFO: most composite statements (else if xxx, etc) do not check for 'EOL' on purpose

		if bytes.Compare(keywordToken.Value, []byte("if")) == 0 {

			ifExpression := NewGroupStatementNode(KIND_IF, keywordToken.Range, p.stream)
			ifExpression.rng.End = p.lastToken.Range.End
			ifExpression.KeywordRange = keywordToken.Range
			ifExpression.KeywordToken = keywordToken

			p.nextToken() // skip keyword "if"

			expression, err := p.ParseStatement()

			ifExpression.ControlFlow = expression
			ifExpression.Err = err

			ifExpression.rng.End = expression.Range().End

			if err != nil {
				return ifExpression, err
			}

			if expression == nil { // because if err == nil, then expression != nil
				panic("returned AST was nil although parsing completed succesfully. can't be added to ControlFlow\n" + ifExpression.String())
			}

			switch expression.Kind() {
			case KIND_VARIABLE_ASSIGNMENT, KIND_VARIABLE_DECLARATION, KIND_MULTI_EXPRESSION, KIND_EXPRESSION:

			case KIND_ELSE:
				err = NewParseError(&lexer.Token{}, errors.New("did not mean 'else if' ?"))
				err.Range = expression.Range()
				err.Range.Start = ifExpression.rng.Start
				ifExpression.Err = err
				return ifExpression, err

			default:
				err = NewParseError(&lexer.Token{}, errors.New("'if' do not accept this kind of statement"))
				err.Range = expression.Range()
				ifExpression.Err = err

				return ifExpression, err
			}

			return ifExpression, nil

		} else if bytes.Compare(keywordToken.Value, []byte("else")) == 0 {

			elseExpression := NewGroupStatementNode(KIND_ELSE, keywordToken.Range, p.stream)
			elseExpression.rng.End = p.lastToken.Range.End
			elseExpression.KeywordRange = keywordToken.Range
			elseExpression.KeywordToken = keywordToken

			p.nextToken() // skip 'else' token

			if p.expect(lexer.EOL) {
				return elseExpression, nil
			}

			elseControlFlow, err := p.ParseStatement()
			elseCompositeExpression, _ := elseControlFlow.(*GroupStatementNode)

			if elseCompositeExpression == nil {
				err = NewParseError(keywordToken, errors.New("else statement expect either 'if' or 'with' or nothing"))
				err.Range.End = elseControlFlow.Range().End
				elseExpression.Err = err

				return elseExpression, err
			}

			// merge old token value with the newer one, separating them by a space ' '
			newValue := elseCompositeExpression.KeywordToken.Value
			elseCompositeExpression.KeywordToken = lexer.CloneToken(elseCompositeExpression.KeywordToken)
			elseCompositeExpression.KeywordToken.Value = append(elseExpression.KeywordToken.Value, byte(' '))
			elseCompositeExpression.KeywordToken.Value = append(elseCompositeExpression.KeywordToken.Value, newValue...)
			elseCompositeExpression.KeywordToken.Range.Start = elseExpression.rng.Start

			elseCompositeExpression.KeywordRange = elseCompositeExpression.KeywordToken.Range
			elseCompositeExpression.rng.Start = elseExpression.rng.Start
			elseCompositeExpression.Err = err

			switch elseCompositeExpression.Kind() {
			case KIND_IF:
				elseCompositeExpression.SetKind(KIND_ELSE_IF)
			case KIND_WITH:
				elseCompositeExpression.SetKind(KIND_ELSE_WITH)
			default:
				err = NewParseError(keywordToken, errors.New("else statement expect either 'if' or 'with' or nothing"))
				err.Range = elseCompositeExpression.Range()
				elseCompositeExpression.Err = err
				return elseCompositeExpression, err
			}

			if err != nil {
				return elseCompositeExpression, err
			}

			if elseCompositeExpression.Range().End != p.lastToken.Range.End {
				panic("ending location mismatch between 'else if/with/...' statement and its expression\n" + elseCompositeExpression.String())
			}

			return elseCompositeExpression, nil

		} else if bytes.Compare(keywordToken.Value, []byte("range")) == 0 {

			rangeExpression := NewGroupStatementNode(KIND_RANGE_LOOP, keywordToken.Range, p.stream)
			rangeExpression.rng.End = p.lastToken.Range.End
			rangeExpression.KeywordRange = keywordToken.Range
			rangeExpression.KeywordToken = keywordToken

			p.nextToken()

			expression, err := p.ParseStatement()
			rangeExpression.ControlFlow = expression
			rangeExpression.Err = err

			rangeExpression.rng.End = expression.Range().End

			if expression == nil {
				panic("unexpected <nil> AST return while parsing 'range expression'")
			}

			if err != nil {
				return rangeExpression, err
			}

			switch expression.Kind() {
			case KIND_VARIABLE_ASSIGNMENT, KIND_VARIABLE_DECLARATION, KIND_MULTI_EXPRESSION, KIND_EXPRESSION:

			default:
				err = NewParseError(p.lastToken, errors.New("'range' do not accept this type of expression"))
				err.Range = expression.Range()
				rangeExpression.Err = err

				return rangeExpression, err
			}

			return rangeExpression, nil

		} else if bytes.Compare(keywordToken.Value, []byte("with")) == 0 {

			withExpression := NewGroupStatementNode(KIND_WITH, keywordToken.Range, p.stream)
			withExpression.rng.End = p.lastToken.Range.End
			withExpression.KeywordRange = keywordToken.Range
			withExpression.KeywordToken = keywordToken

			p.nextToken() // skip 'with' token

			expression, err := p.ParseStatement()
			withExpression.ControlFlow = expression
			withExpression.Err = err

			withExpression.rng.End = expression.Range().End

			if expression == nil {
				panic("unexpected <nil> AST return while parsing 'with expression'")
			}

			if err != nil {
				return withExpression, err
			}

			switch expression.Kind() {
			case KIND_VARIABLE_ASSIGNMENT, KIND_VARIABLE_DECLARATION, KIND_MULTI_EXPRESSION, KIND_EXPRESSION:

			default:
				err = NewParseError(p.lastToken, errors.New("'with' do not accept this type of expression"))
				err.Range = expression.Range()
				withExpression.Err = err

				return withExpression, err
			}

			return withExpression, nil

		} else if bytes.Compare(keywordToken.Value, []byte("block")) == 0 {

			blockExpression := NewGroupStatementNode(KIND_BLOCK_TEMPLATE, keywordToken.Range, p.stream)
			blockExpression.rng.End = p.lastToken.Range.End
			blockExpression.KeywordRange = keywordToken.Range
			blockExpression.KeywordToken = keywordToken

			p.nextToken() // skip 'block' token

			if !p.accept(lexer.STRING) {
				err := NewParseError(p.peek(), errors.New("'block' expect a string next to it"))
				blockExpression.Err = err
				return blockExpression, err
			}

			templateExpression := NewTemplateStatementNode(KIND_BLOCK_TEMPLATE, p.peek().Range)
			templateExpression.TemplateName = p.peek()
			templateExpression.parent = blockExpression
			blockExpression.ControlFlow = templateExpression

			p.nextToken()

			expression, err := p.ParseStatement()
			templateExpression.Expression = expression
			templateExpression.rng.End = expression.Range().End

			blockExpression.Err = err
			blockExpression.rng.End = expression.Range().End
			if expression == nil {
				panic("unexpected <nil> AST return while parsing 'block expression'")
			}

			if err != nil {
				return blockExpression, err
			}

			switch expression.Kind() {
			case KIND_VARIABLE_ASSIGNMENT, KIND_VARIABLE_DECLARATION, KIND_MULTI_EXPRESSION, KIND_EXPRESSION:

			default:
				err = NewParseError(templateExpression.TemplateName, errors.New("'block' do not accept this type of expression"))
				err.Range = expression.Range()
				blockExpression.Err = err
				return blockExpression, err
			}

			return blockExpression, nil

		} else if bytes.Compare(keywordToken.Value, []byte("define")) == 0 {

			defineExpression := NewGroupStatementNode(KIND_DEFINE_TEMPLATE, keywordToken.Range, p.stream)
			defineExpression.rng.End = p.lastToken.Range.End
			defineExpression.KeywordRange = keywordToken.Range
			defineExpression.KeywordToken = keywordToken

			p.nextToken() // skip 'define' token

			if !p.accept(lexer.STRING) {
				err := NewParseError(p.peek(), errors.New("'define' expect a string next to it"))
				defineExpression.Err = err
				return defineExpression, err
			}

			templateExpression := NewTemplateStatementNode(KIND_DEFINE_TEMPLATE, p.peek().Range)
			templateExpression.TemplateName = p.peek()
			templateExpression.parent = defineExpression
			defineExpression.ControlFlow = templateExpression

			p.nextToken()

			if !p.expect(lexer.EOL) {
				err := NewParseError(p.peek(), errors.New("'define' do not accept any expression"))
				err.Range.End = p.lastToken.Range.End
				defineExpression.Err = err
				return defineExpression, err
			}

			return defineExpression, nil

		} else if bytes.Compare(keywordToken.Value, []byte("template")) == 0 {

			templateExpression := NewTemplateStatementNode(KIND_USE_TEMPLATE, keywordToken.Range)
			templateExpression.rng.End = p.lastToken.Range.End
			templateExpression.KeywordRange = keywordToken.Range

			p.nextToken() // skip 'template' tokens

			if !p.accept(lexer.STRING) {
				err := NewParseError(p.peek(), errors.New("missing template name"))
				templateExpression.Err = err
				return templateExpression, err
			}

			templateExpression.TemplateName = p.peek()
			p.nextToken()

			if p.accept(lexer.EOL) {
				return templateExpression, nil
			}

			expression, err := p.expressionStatementParser()
			templateExpression.Expression = expression
			templateExpression.Err = err

			templateExpression.rng.End = expression.Range().End

			if expression == nil {
				panic("unexpected <nil> AST return while parsing 'template expression'")
			}

			if err != nil {
				return templateExpression, err
			}

			switch expression.Kind() {
			case KIND_VARIABLE_ASSIGNMENT, KIND_VARIABLE_DECLARATION, KIND_MULTI_EXPRESSION, KIND_EXPRESSION:

			default:
				err = NewParseError(templateExpression.TemplateName, errors.New("'template' do not accept this type of expression"))
				err.Range = expression.Range()
				templateExpression.Err = err
				return templateExpression, err
			}

			if !p.expect(lexer.EOL) {
				err := NewParseError(p.peek(), errors.New("early template end"))
				err.Range.End = p.lastToken.Range.End
				templateExpression.Err = err
				return templateExpression, err
			}

			return templateExpression, nil

		} else if bytes.Compare(keywordToken.Value, []byte("end")) == 0 {

			endExpression := NewGroupStatementNode(KIND_END, keywordToken.Range, p.stream)
			endExpression.rng.End = p.lastToken.Range.End
			endExpression.KeywordRange = keywordToken.Range
			endExpression.KeywordToken = keywordToken

			p.nextToken() // skip 'end' token

			if !p.expect(lexer.EOL) {
				err := NewParseError(keywordToken, errors.New("'end' is a standalone command"))
				err.Range.End = p.lastToken.Range.End
				endExpression.Err = err
				return endExpression, err
			}

			return endExpression, nil

		} else if bytes.Compare(keywordToken.Value, []byte("break")) == 0 {

			breakCommand := NewSpecialCommandNode(KIND_BREAK, keywordToken, keywordToken.Range)

			p.nextToken() // skip  token

			if !p.expect(lexer.EOL) {
				err := NewParseError(keywordToken, errors.New("'break' is a standalone command"))
				err.Range.End = p.lastToken.Range.End
				breakCommand.Err = err
				return breakCommand, err
			}

			return breakCommand, nil

		} else if bytes.Compare(keywordToken.Value, []byte("continue")) == 0 {

			continueCommand := NewSpecialCommandNode(KIND_CONTINUE, keywordToken, keywordToken.Range)

			p.nextToken() // skip  token

			if !p.expect(lexer.EOL) {
				err := NewParseError(keywordToken, errors.New("'continue' is a standalone command"))
				err.Range.End = p.lastToken.Range.End
				continueCommand.Err = err
				return continueCommand, err
			}

			return continueCommand, nil
		}

	} else if p.accept(lexer.COMMENT) {
		commentExpression := &CommentNode{kind: KIND_COMMENT, Value: p.peek(), rng: p.peek().Range}

		p.nextToken()

		if !p.expect(lexer.EOL) {
			err := NewParseError(p.peek(), errors.New("syntax for comment didn't end properly. extraneous expression"))
			err.Range.End = p.lastToken.Range.End
			commentExpression.Err = err
			return commentExpression, err
		}

		// Check that this comment contains go code to semantically analize
		lookForAndSetGoCodeInComment(commentExpression)

		return commentExpression, nil
	}

	// 4. Default parser whenever no other parser have been enabled
	expression, err := p.expressionStatementParser()

	if err != nil {
		expression.SetError(err)
		return expression, err
	}

	if !p.expect(lexer.EOL) {
		err = NewParseError(p.peek(), errors.New("expected end of statement"))
		err.Range.End = p.lastToken.Range.End
		expression.SetError(err)
		return expression, err
	}

	return expression, nil
}

func (p *Parser) expressionStatementParser() (AstNode, *ParseError) {
	p.incRecursionDepth()
	defer p.decRecursionDepth()

	if multi, err := p.checkRecursionStatus(); err != nil {
		return multi, err
	}

outter_loop:
	for _, token := range p.stream.Tokens[p.indexCurrentToken:] {
		switch token.ID {
		case lexer.DECLARATION_ASSIGNEMENT:
			varDeclarationNode, err := p.declarationAssignmentParser()
			varDeclarationNode.Err = err

			return varDeclarationNode, err

		case lexer.ASSIGNEMENT:
			varInitialization, err := p.initializationAssignmentParser()
			varInitialization.Err = err

			return varInitialization, err

		case lexer.LEFT_PAREN:
			break outter_loop
		}
	}

	multiExpression, err := p.multiExpressionParser()
	multiExpression.Err = err

	return multiExpression, err
}

func (p *Parser) declarationAssignmentParser() (*VariableDeclarationNode, *ParseError) {
	if p.lastToken == nil {
		panic("unexpected empty token found at end of the current instruction")
	}

	varDeclarationNode := NewVariableDeclarationNode(KIND_VARIABLE_DECLARATION, p.peek().Range.Start, p.lastToken.Range.End, nil)

	count := 0
	for {
		count++

		if count > 2 {
			err := NewParseError(p.peek(), errors.New("only one or two variables can be declared at once"))
			err.Range = varDeclarationNode.rng
			varDeclarationNode.Err = err
			return varDeclarationNode, err
		}

		if !p.accept(lexer.DOLLAR_VARIABLE) {
			err := NewParseError(p.peek(), errors.New("variable name must start with '$'"))
			varDeclarationNode.Err = err
			return varDeclarationNode, err
		}

		variable := p.peek()
		varDeclarationNode.VariableNames = append(varDeclarationNode.VariableNames, variable)

		p.nextToken()

		if p.expect(lexer.COMMA) {
			continue
		}

		break
	}

	if !p.expect(lexer.DECLARATION_ASSIGNEMENT) {
		err := NewParseError(p.peek(), errors.New("expected assignment ':='"))
		varDeclarationNode.Err = err
		return varDeclarationNode, err
	}

	node, err := p.expressionStatementParser()
	expression, ok := node.(*MultiExpressionNode)

	if ok == false {
		err := NewParseError(p.peek(), errors.New("expected an expression"))
		err.Range = node.Range()
		return varDeclarationNode, err
	}

	varDeclarationNode.Value = expression
	varDeclarationNode.Err = err

	varDeclarationNode.rng.End = expression.Range().End

	if expression == nil {
		panic("An AST, erroneous or not, must always be non <nil>. Can't be added to ControlFlow\n" + varDeclarationNode.String())
	}

	return varDeclarationNode, err
}

func (p *Parser) initializationAssignmentParser() (*VariableAssignationNode, *ParseError) {
	if p.lastToken == nil {
		panic("unexpected empty token found at end of the current instruction")
	}

	varAssignation := NewVariableAssignmentNode(KIND_VARIABLE_ASSIGNMENT, p.peek().Range.Start, p.lastToken.Range.End, nil)

	count := 0
	for {
		count++

		if count > 2 {
			err := NewParseError(p.peek(), errors.New("only one or two variables can be declared at once"))
			err.Range = varAssignation.rng
			varAssignation.Err = err
			return varAssignation, err
		}

		if !p.accept(lexer.DOLLAR_VARIABLE) {
			err := NewParseError(p.peek(), errors.New("variable name must start with '$'"))
			varAssignation.Err = err
			return varAssignation, err
		}

		variable := p.peek()
		varAssignation.VariableNames = append(varAssignation.VariableNames, variable)

		p.nextToken()

		if p.expect(lexer.COMMA) {
			continue
		}

		break
	}

	if !p.expect(lexer.ASSIGNEMENT) {
		err := NewParseError(p.peek(), errors.New("expected assignment '='"))
		varAssignation.Err = err
		return varAssignation, err
	}

	node, err := p.expressionStatementParser()
	expression, ok := node.(*MultiExpressionNode) // invalidate 'VariableDeclarationNode' and 'VariableAssignationNode'

	if ok == false {
		err := NewParseError(p.peek(), errors.New("expected an expression"))
		err.Range = node.Range()
		return varAssignation, err
	}

	varAssignation.Value = expression
	varAssignation.Err = err

	varAssignation.rng.End = expression.Range().End

	if expression == nil {
		panic("An AST, erroneous or not, must always be non <nil>. Can't be added to ControlFlow\n" + varAssignation.String())
	}

	return varAssignation, err
}

func (p *Parser) multiExpressionParser() (*MultiExpressionNode, *ParseError) {

	if p.lastToken == nil {
		panic("unexpected empty token found at end of the current instruction")
	}

	multiExpression := NewMultiExpressionNode(KIND_MULTI_EXPRESSION, p.peek().Range.Start, p.lastToken.Range.End, nil)

	var expression *ExpressionNode
	var err *ParseError

	for next := true; next; next = p.expect(lexer.PIPE) {

		expression, err = p.expressionParser() // main parsing
		expression.Err = err

		multiExpression.Expressions = append(multiExpression.Expressions, expression)

		if err != nil {
			multiExpression.Err = err
			return multiExpression, err
		}

		if expression == nil { // because if err == nil, then expression != nil
			panic("returned AST was nil although parsing completed succesfully. can't be added to ControlFlow\n" + multiExpression.String())
		}
	}

	multiExpression.rng.End = expression.Range().End

	return multiExpression, nil
}

func (p *Parser) expressionParser() (*ExpressionNode, *ParseError) {
	if p.lastToken == nil {
		panic("unexpected empty token found at end of the current instruction")
	}

	expression := NewExpressionNode(KIND_EXPRESSION, p.peek().Range)
	expression.rng.End = p.lastToken.Range.End

	expression.ExpandedTokens = make([]AstNode, 0, p.sizeStream)
	expression.Symbols = make([]*lexer.Token, 0, p.sizeStream)

	count := 0
	for p.accept(lexer.FUNCTION) || p.accept(lexer.DOT_VARIABLE) || p.accept(lexer.DOLLAR_VARIABLE) || p.accept(lexer.STRING) || p.accept(lexer.CHARACTER) ||
		p.accept(lexer.LEFT_PAREN) || p.accept(lexer.RIGTH_PAREN) || p.accept(lexer.NUMBER) || p.accept(lexer.DECIMAL) || p.accept(lexer.COMPLEX_NUMBER) || p.accept(lexer.BOOLEAN) {

		count++
		if count > 100 {
			panic("possible infinite loop detected while parsing 'expression'")
		}

		if p.accept(lexer.LEFT_PAREN) {
			leftParenthesis := p.peek()
			p.nextToken() // skip '('

			node, err := p.expressionStatementParser() // main processing
			node.SetError(err)

			tokenName := "$__expandable_token_" + strconv.Itoa(GetUniqueNumber())
			group := lexer.NewToken(lexer.EXPANDABLE_GROUP, leftParenthesis.Range, []byte(tokenName))
			group.Range.End = node.Range().End

			expression.ExpandedTokens = append(expression.ExpandedTokens, node)
			expression.Symbols = append(expression.Symbols, group)

			if err != nil {
				return expression, err
			}

			rightParenthesis := p.peek()

			if p.accept(lexer.RIGTH_PAREN) == false {
				err := NewParseError(rightParenthesis, errors.New("missing closing parenthesis ')'"))
				expression.SetError(err)
				return expression, err
			}

			group.Range.End = rightParenthesis.Range.End
			p.nextToken() // skip ')'

			// 2. handle case where: (expression).field VS (expression) .field
			if next := p.peek(); next != nil {
				distance := next.Range.Start.Character - (rightParenthesis.Range.End.Character - 1)

				// check whether the 'next' token is right next to parenthesis
				if distance == 1 && next.ID == lexer.DOT_VARIABLE {
					group.Range.End = next.Range.End
					group.Value = append(group.Value, next.Value...)
					p.nextToken() // skip 'dot var'

					if len(string(next.Value)) == 1 {
						err := NewParseError(group, errors.New("sub-expression cannot end with '.'"))
						expression.SetError(err)
						return expression, err
					}

				} else if distance == 1 && next.ID == lexer.RIGTH_PAREN {
					// do nothing, bc there is no error to report
				} else if distance == 1 {
					err := NewParseError(next, errors.New("need space between argument"))
					expression.SetError(err)
					return expression, err
				}
			}

			continue

		} else if p.accept(lexer.RIGTH_PAREN) {
			break
		}

		expression.ExpandedTokens = append(expression.ExpandedTokens, nil)
		expression.Symbols = append(expression.Symbols, p.peek())
		p.nextToken()
	}

	if len(expression.Symbols) != len(expression.ExpandedTokens) {
		panic(fmt.Sprintf("length mismatch between Symbols (%d) and ExpandedTokens (%d). stream = %s", len(expression.Symbols), len(expression.ExpandedTokens), p.stream.String()))
	}

	if len(expression.Symbols) == 0 {
		err := NewParseError(p.peek(), errors.New("empty expression"))
		err.Range.Start = p.peek().Range.End
		err.Range.Start.Character--
		expression.SetError(err)
		return expression, err
	}

	size := len(expression.Symbols)
	expression.rng.End = expression.Symbols[size-1].Range.End
	// expression.rng.End = p.peek().Range.End

	return expression, nil
}

func lookForAndSetGoCodeInComment(commentExpression *CommentNode) {
	const SEP_COMMENT_GOCODE = "go:code"
	comment := commentExpression.Value.Value

	before, after, found := bytes.Cut(comment, []byte(SEP_COMMENT_GOCODE))
	if !found {
		return
	} else if len(after) == 0 {
		return
	}

	if len(bytes.TrimSpace(before)) > 0 {
		return
	}

	// if bytes.TrimSpace(before) == 0; then execute below
	comment = after

	switch comment[0] {
	case ' ', '\n', '\t', '\r', '\v', '\f': // unicode.IsSpace(rune(comment[0]))
		// continue to next step succesfully
	default:
		return
	}

	initialLength := len(commentExpression.Value.Value)
	finalLength := len(comment)
	indexStartGoCode := initialLength - finalLength

	relativePositionStartGoCode := lexer.ConvertSingleIndexToTextEditorPosition(commentExpression.Value.Value, indexStartGoCode)

	reach := commentExpression.rng
	reach.Start.Line += relativePositionStartGoCode.Line
	reach.Start.Character = relativePositionStartGoCode.Character

	commentExpression.GoCode = &lexer.Token{
		ID:    lexer.COMMENT,
		Range: reach,
		Value: comment,
	}
}

func (p Parser) peek() *lexer.Token {
	index := p.indexCurrentToken

	if index >= p.sizeStream {
		return nil
	}

	return &p.stream.Tokens[index]
}

func (p Parser) peekAt(pos int) *lexer.Token {
	index := p.indexCurrentToken + pos

	if index >= p.sizeStream {
		return nil
	}

	return &p.stream.Tokens[index]
}

func (p *Parser) nextToken() {
	p.indexCurrentToken++
}

func (p Parser) accept(kind lexer.Kind) bool {
	index := p.indexCurrentToken

	if index >= p.sizeStream {
		return false
	}

	return p.stream.Tokens[index].ID == kind
}

func (p Parser) acceptAt(pos int, kind lexer.Kind) bool {
	index := p.indexCurrentToken + pos

	if index >= p.sizeStream {
		return false
	}

	return p.stream.Tokens[index].ID == kind
}

func (p *Parser) expect(kind lexer.Kind) bool {
	if p.accept(kind) {
		p.nextToken()

		return true
	}

	return false
}

func (p *Parser) incRecursionDepth() {
	p.currentRecursionDepth++
}

func (p Parser) checkRecursionStatus() (*MultiExpressionNode, *ParseError) {
	if p.isRecursionMaxDepth() {
		err := NewParseError(p.peek(), errors.New("parser error, reached the max depth authorized"))
		multiExpression := NewMultiExpressionNode(KIND_MULTI_EXPRESSION, p.peek().Range.Start, p.lastToken.Range.End, err)
		return multiExpression, err
	}

	return nil, nil
}

func (p *Parser) decRecursionDepth() {
	p.currentRecursionDepth--
}

func (p Parser) isRecursionMaxDepth() bool {
	return p.currentRecursionDepth >= p.maxRecursionDepth
}

func NewParseError(token *lexer.Token, err error) *ParseError {
	if token == nil {
		panic("token cannot be nil while creating parse error")
	}

	e := &ParseError{
		Err:   err,
		Range: token.Range,
		Token: token,
	}

	return e
}

func getLastElement[E any](arr []E) E {
	var last E

	size := len(arr)
	if size <= 0 {
		return last
	}

	last = arr[size-1]

	return last
}
