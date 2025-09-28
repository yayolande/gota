package parser

import (
	"bytes"
	"errors"
	"log" // TODO: to remove

	"github.com/yayolande/gota/lexer"
)

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
	input                 []lexer.Token
	openedNodeStack       []*GroupStatementNode
	maxRecursionDepth     int
	currentRecursionDepth int
}

func createParser(tokens []lexer.Token) *Parser {
	if tokens == nil {
		panic("cannot create a parser containing empty tokens")
	}

	input := make([]lexer.Token, len(tokens))
	copy(input, tokens)

	/*
		defaultGroupStatementNode := &GroupStatementNode{
			Kind: KIND_GROUP_STATEMENT,
			isRoot: true,
		}
	*/

	defaultGroupStatementNode := NewGroupStatementNode(KIND_GROUP_STATEMENT, lexer.Range{})
	defaultGroupStatementNode.isRoot = true

	groupNodeStack := []*GroupStatementNode{}
	groupNodeStack = append(groupNodeStack, defaultGroupStatementNode)

	parser := &Parser{
		input:                 input,
		openedNodeStack:       groupNodeStack,
		maxRecursionDepth:     3,
		currentRecursionDepth: 0,
	}

	return parser
}

func appendStatementToScopeShortcut(scope *GroupStatementNode, statement AstNode) *ParseError {
	switch stmt := statement.(type) {
	case *GroupStatementNode:

		if !stmt.IsTemplate() {
			// exit the current scope
			return nil
		}

		//	WIP
		// TODO: when initializing the scope, create 'ShortCut' fields
		// (necessary since they are pointer, otherwise they are 'nil' when they reach this point)
		templateNode := stmt.ControlFlow.(*TemplateStatementNode)
		templateName := string(templateNode.TemplateName.Value)

		if scope.ShortCut.TemplateDefined[templateName] != nil {
			err := NewParseError(templateNode.TemplateName, errors.New("template already defined"))
			return err
		}

		scope.ShortCut.TemplateDefined[templateName] = stmt

	case *TemplateStatementNode:

		// templateName := string(stmt.TemplateName.Value)
		// scope.ShortCut.TemplateCallUsed[templateName] = stmt
		scope.ShortCut.TemplateCallUsed = append(scope.ShortCut.TemplateCallUsed, stmt)

	case *CommentNode:

		if stmt.GoCode == nil {
			// exit the current scope
			return nil
		}

		if scope.ShortCut.CommentGoCode != nil {
			// WIP
			// ignore the new comment ;
			// and return an error to user (no 2 'go:code' allowed in the same scope)
			stmt.GoCode = nil

			err := errors.New("cannot redeclare 'go:code' in the same scope")
			return NewParseError(stmt.Value, err)
		}

		scope.ShortCut.CommentGoCode = stmt

	case *VariableDeclarationNode:

		if len(stmt.VariableNames) == 0 {
			// exit the current scope
			return nil
		}

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
}

func (p *Parser) safeStatementGrouping(node AstNode) *ParseError {
	if node == nil {
		return nil
	}

	// TODO: change var name to : "openedGroupNodeStack", "activeScopeNodes", "activeScopeStack", "openedScopeStack"
	if len(p.openedNodeStack) == 0 {
		panic("no initial scope available to hold the statements. There must always exist at least one 'scope/group' at any moment")
	}

	if p.openedNodeStack[0].isRoot == false {
		panic("root node has'nt been marked as such. root node, and only the root node, can be marked as 'isRoot'")
	}

	var err *ParseError
	var ROOT_SCOPE *GroupStatementNode = p.openedNodeStack[0]

	stackSize := len(p.openedNodeStack)
	currentScope := p.openedNodeStack[stackSize-1]

	newScope, isScope := node.(*GroupStatementNode)

	if !isScope {

		appendStatementToCurrentScope(currentScope, node)
		err = appendStatementToScopeShortcut(currentScope, node)

	} else {
		if newScope.IsRoot() {
			log.Printf("non-root node cannot be flaged as 'root'.\n culprit node = %#v\n", newScope)
			panic("only the root node, can be marked as 'isRoot', but found it on non-root node")
		}

		newScope.parent = currentScope

		switch newScope.Kind() {
		case KIND_IF, KIND_WITH, KIND_RANGE_LOOP, KIND_BLOCK_TEMPLATE, KIND_DEFINE_TEMPLATE:
			newScope.parent = currentScope
			appendStatementToCurrentScope(currentScope, newScope)
			err = appendStatementToScopeShortcut(currentScope, newScope)

			p.openedNodeStack = append(p.openedNodeStack, newScope)

		case KIND_ELSE_IF:
			if stackSize >= 2 {
				switch currentScope.Kind() {
				case KIND_IF, KIND_ELSE_IF: // Remove the last element from the stack and switch it with 'KIND_ELSE_IF' scope
					scopeToClose := currentScope
					scopeToClose.rng.End = newScope.Range().Start

					parentScope := p.openedNodeStack[stackSize-2]
					p.openedNodeStack = p.openedNodeStack[:stackSize-1]

					newScope.parent = parentScope
					appendStatementToCurrentScope(parentScope, newScope)

					p.openedNodeStack = append(p.openedNodeStack, newScope)
				default:
					err = &ParseError{Range: currentScope.Range(),
						Err: errors.New("'else if' statement is not compatible with '" + currentScope.Kind().String() + "'")}
				}
			} else {
				err = &ParseError{Range: newScope.Range(),
					Err: errors.New("extraneous statement '" + newScope.Kind().String() + "'")}
			}
		case KIND_ELSE_WITH:
			if stackSize >= 2 {
				switch currentScope.Kind() {
				case KIND_WITH, KIND_ELSE_WITH:
					scopeToClose := currentScope
					scopeToClose.rng.End = newScope.Range().Start

					// Remove the last element from the stack and switch it with 'KIND_ELSE_WITH' scope
					parentScope := p.openedNodeStack[stackSize-2]
					p.openedNodeStack = p.openedNodeStack[:stackSize-1] // fold current scope

					newScope.parent = parentScope
					appendStatementToCurrentScope(parentScope, newScope)

					p.openedNodeStack = append(p.openedNodeStack, newScope)
				default:
					err = &ParseError{Range: currentScope.Range(),
						Err: errors.New("'else with' statement is not compatible with '" + currentScope.Kind().String() + "'")}
				}
			} else {
				err = &ParseError{Range: newScope.Range(),
					Err: errors.New("extraneous statement '" + newScope.Kind().String() + "'")}
			}
		case KIND_ELSE:
			if stackSize >= 2 {
				switch currentScope.Kind() {
				case KIND_IF, KIND_ELSE_IF, KIND_WITH, KIND_ELSE_WITH, KIND_RANGE_LOOP:
					// Remove the last element from the stack and switch it with 'KIND_ELSE' scope

					scopeToClose := currentScope
					scopeToClose.rng.End = newScope.Range().Start

					parentScope := p.openedNodeStack[stackSize-2]
					p.openedNodeStack = p.openedNodeStack[:stackSize-1] // fold previous scope

					newScope.parent = parentScope
					appendStatementToCurrentScope(parentScope, newScope)

					p.openedNodeStack = append(p.openedNodeStack, newScope)

				default:
					err = &ParseError{Range: newScope.Range(),
						Err: errors.New("'else' statement is not compatible with '" + currentScope.Kind().String() + "'")}
				}
			} else {
				err = &ParseError{Range: newScope.Range(),
					Err: errors.New("extraneous statement '" + newScope.Kind().String() + "'")}
			}
		case KIND_END:
			if stackSize >= 2 {
				scopeToClose := currentScope
				scopeToClose.rng.End = newScope.Range().Start

				parentScope := p.openedNodeStack[stackSize-2]
				p.openedNodeStack = p.openedNodeStack[:stackSize-1] // fold/close current scope

				newScope.parent = parentScope
				appendStatementToCurrentScope(parentScope, newScope)

			} else {
				err = &ParseError{Range: newScope.Range(), Err: errors.New("extraneous 'end' statement detected")}
			}
		default:
			log.Printf("unhandled scope type error\n scope = %#v\n", newScope)
			panic("scope type '" + newScope.Kind().String() + "' is not yet handled for statement grouping\n" + newScope.String())
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
func Parse(tokens []lexer.Token) (*GroupStatementNode, []lexer.Error) {
	if tokens == nil {
		return nil, nil
	}

	parser := createParser(tokens)

	var errs []lexer.Error
	var err *ParseError
	var node AstNode

	for !parser.isEOF() {
		node, err = parser.StatementParser()

		if err != nil {
			errs = append(errs, err)
			parser.flushInputUntilNextStatement()
		} else {
			err = parser.safeStatementGrouping(node)

			if err != nil {
				errs = append(errs, err)
			}
		}
	}

	if len(parser.openedNodeStack) == 0 {
		panic("fatal error while building the parse tree. Expected at least one scope/group but found nothing")
	}

	defaultGroupStatementNode := parser.openedNodeStack[0]

	if size := len(parser.openedNodeStack); size > 1 {
		// currentScope := getLastElement(parser.openedNodeStack)
		currentScope := parser.openedNodeStack[size-1]
		err := ParseError{Range: currentScope.Range(),
			// Err: errors.New("group statements '"+ currentScope.kind.String() + "' not claused")}
			Err: errors.New("missing matching '{{ end }}' statement")}

		errs = append(errs, err)
	}

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
func (p *Parser) StatementParser() (ast AstNode, er *ParseError) {
	if len(p.input) == 0 {
		return nil, nil
	}

	// 1. Escape infinite recursion
	if p.isRecursionMaxDepth() {
		err := NewParseError(p.peek(), errors.New("parser error, reached the max depth authorized"))
		return nil, err
	}

	p.incRecursionDepth()
	defer p.decRecursionDepth()

	// 2. Helper variable mainly used to easily get end location of the instruction (token.Range.End)
	lastTokenInInstruction := p.peekAtEndCurrentInstruction() // token before 'EOL'
	if lastTokenInInstruction == nil {
		panic("unexpected empty token found at end of the current instruction")
	}

	// 3. Syntax Parser for the Go Template language
	if p.accept(lexer.KEYWORD) {
		keywordToken := p.peek()

		// INFO: most composite statements (else if xxx, etc) do not check for 'EOL' on purpose

		if bytes.Compare(keywordToken.Value, []byte("if")) == 0 {

			ifExpression := NewGroupStatementNode(KIND_IF, keywordToken.Range)
			ifExpression.rng.End = lastTokenInInstruction.Range.End

			p.nextToken() // skip keyword "if"

			// BUG: Change 'StatementParser()' to something more specific that only handle variable declaration, assignment, multi expression, expression
			expression, err := p.StatementParser()
			ifExpression.ControlFlow = expression

			if err != nil {
				return ifExpression, err // partial AST are meant for debugging
			}

			if expression == nil { // because if err == nil, then expression != nil
				panic("returned AST was nil although parsing completed succesfully. can't be added to ControlFlow\n" + ifExpression.String())
			}

			if ifExpression.rng.End != expression.Range().End {
				panic("ending location mismatch between 'if' statement and its expression\n" + ifExpression.String())
			}

			switch expression.Kind() {
			case KIND_VARIABLE_ASSIGNMENT, KIND_VARIABLE_DECLARATION, KIND_MULTI_EXPRESSION, KIND_EXPRESSION:

			default:
				err = NewParseError(&lexer.Token{}, errors.New("'if' do not accept this kind of statement"))
				err.Range = expression.Range()
				return ifExpression, err
			}

			return ifExpression, nil

		} else if bytes.Compare(keywordToken.Value, []byte("else")) == 0 {

			elseExpression := NewGroupStatementNode(KIND_ELSE, keywordToken.Range)
			elseExpression.rng.End = lastTokenInInstruction.Range.End

			p.nextToken() // skip 'else' token

			if p.expect(lexer.EOL) {
				return elseExpression, nil
			}

			// past this, 'elseExpression' is not useful anymore, and will be replaced by 'elseCompositeExpression'
			elseExpression = nil
			elseCompositeExpression, err := p.StatementParser()

			if err != nil {
				return elseCompositeExpression, err // partial AST are meant for debugging
			}

			if elseCompositeExpression == nil { // because if err == nil, then expression != nil
				panic("returned AST was nil although parsing completed succesfully. 'else ...' parse error")
			}

			if elseCompositeExpression.Range().End != lastTokenInInstruction.Range.End {
				panic("ending location mismatch between 'else if/with/...' statement and its expression\n" + elseCompositeExpression.String())
			}

			switch elseCompositeExpression.Kind() {
			case KIND_IF:
				elseCompositeExpression.SetKind(KIND_ELSE_IF)
			case KIND_WITH:
				elseCompositeExpression.SetKind(KIND_ELSE_WITH)
			default:
				err = NewParseError(keywordToken, errors.New("else statement expect either 'if' or 'with' or nothing"))
				err.Range = elseCompositeExpression.Range()
				return elseCompositeExpression, err
			}

			return elseCompositeExpression, nil

		} else if bytes.Compare(keywordToken.Value, []byte("end")) == 0 {

			endExpression := NewGroupStatementNode(KIND_END, keywordToken.Range)
			endExpression.rng.End = lastTokenInInstruction.Range.End

			p.nextToken() // skip 'end' token

			if !p.expect(lexer.EOL) {
				err := NewParseError(keywordToken, errors.New("expression next to 'end' is diasallowed"))
				err.Range.End = lastTokenInInstruction.Range.End
				return endExpression, err
			}

			return endExpression, nil

		} else if bytes.Compare(keywordToken.Value, []byte("range")) == 0 {

			rangeExpression := NewGroupStatementNode(KIND_RANGE_LOOP, keywordToken.Range)
			rangeExpression.rng.End = lastTokenInInstruction.Range.End

			p.nextToken()

			expression, err := p.StatementParser()
			rangeExpression.ControlFlow = expression

			if err != nil {
				return rangeExpression, err // partial AST are meant for debugging
			}

			if expression == nil { // because if err == nil, then expression != nil
				panic("returned AST was nil although parsing completed succesfully. can't be added to ControlFlow\n" + rangeExpression.String())
			}

			if rangeExpression.Range().End != expression.Range().End {
				panic("ending location mismatch between 'range' statement and its expression\n" + rangeExpression.String())
			}

			switch expression.Kind() {
			case KIND_VARIABLE_ASSIGNMENT, KIND_VARIABLE_DECLARATION, KIND_MULTI_EXPRESSION, KIND_EXPRESSION:

			default:
				err = NewParseError(lastTokenInInstruction, errors.New("'range' do not accept those type of expression"))
				err.Range = expression.Range()
				return rangeExpression, err
			}

			return rangeExpression, nil

		} else if bytes.Compare(keywordToken.Value, []byte("with")) == 0 {
			/*
				withExpression := &GroupStatementNode{}
				withExpression.Kind = KIND_WITH
				withExpression.Range = keywordToken.Range
			*/

			withExpression := NewGroupStatementNode(KIND_WITH, keywordToken.Range)
			withExpression.rng.End = lastTokenInInstruction.Range.End

			p.nextToken() // skip 'with' token

			expression, err := p.StatementParser()
			withExpression.ControlFlow = expression

			if err != nil {
				return withExpression, err // partial AST are meant for debugging
			}

			if expression == nil { // because if err == nil, then expression != nil
				panic("returned AST was nil although parsing completed succesfully. 'with ...' parse error")
			}

			if withExpression.Range().End != expression.Range().End {
				panic("ending location mismatch between 'range' statement and its expression\n" + withExpression.String())
			}

			switch expression.Kind() {
			case KIND_VARIABLE_ASSIGNMENT, KIND_VARIABLE_DECLARATION, KIND_MULTI_EXPRESSION, KIND_EXPRESSION:

			default:
				err = NewParseError(lastTokenInInstruction, errors.New("'with' do not accept those type of expression"))
				err.Range = expression.Range()
				return withExpression, err
			}

			return withExpression, nil

		} else if bytes.Compare(keywordToken.Value, []byte("block")) == 0 {
			/*
				blockExpression := &GroupStatementNode{}
				blockExpression.Kind = KIND_BLOCK_TEMPLATE
				blockExpression.Range = keywordToken.Range
			*/

			blockExpression := NewGroupStatementNode(KIND_BLOCK_TEMPLATE, keywordToken.Range)
			blockExpression.rng.End = lastTokenInInstruction.Range.End

			p.nextToken() // skip 'block' token

			if !p.accept(lexer.STRING) {
				err := NewParseError(p.peek(), errors.New("'block' expect a string next to it"))
				return blockExpression, err
			}

			templateExpression := &TemplateStatementNode{kind: KIND_BLOCK_TEMPLATE, TemplateName: p.peek(), rng: p.peek().Range}
			templateExpression.parent = blockExpression
			blockExpression.ControlFlow = templateExpression

			p.nextToken()

			expression, err := p.StatementParser()
			templateExpression.Expression = expression

			if err != nil {
				return blockExpression, err // partial AST are meant for debugging
			}

			if expression == nil { // because if err == nil, then expression != nil
				panic("returned AST was nil although parsing completed succesfully. can't add expression to ControlFlow\n" + blockExpression.String())
			}

			if blockExpression.Range().End != expression.Range().End {
				panic("ending location mismatch between 'range' statement and its expression\n" + blockExpression.String())
			}

			switch expression.Kind() {
			case KIND_VARIABLE_ASSIGNMENT, KIND_VARIABLE_DECLARATION, KIND_MULTI_EXPRESSION, KIND_EXPRESSION:

			default:
				err = NewParseError(templateExpression.TemplateName, errors.New("'block' do not accept those type of expression"))
				err.Range = expression.Range()
				return blockExpression, err
			}

			return blockExpression, nil

		} else if bytes.Compare(keywordToken.Value, []byte("define")) == 0 {
			/*
				defineExpression := &GroupStatementNode{}
				defineExpression.Kind = KIND_DEFINE_TEMPLATE
				defineExpression.Range = keywordToken.Range
			*/

			defineExpression := NewGroupStatementNode(KIND_DEFINE_TEMPLATE, keywordToken.Range)
			defineExpression.rng.End = lastTokenInInstruction.Range.End

			p.nextToken() // skip 'define' token

			if !p.accept(lexer.STRING) {
				err := NewParseError(p.peek(), errors.New("'define' expect a string next to it"))
				return defineExpression, err
			}

			templateExpression := &TemplateStatementNode{kind: KIND_DEFINE_TEMPLATE, TemplateName: p.peek(), rng: p.peek().Range}
			templateExpression.parent = defineExpression
			defineExpression.ControlFlow = templateExpression

			p.nextToken()

			if !p.expect(lexer.EOL) {
				err := NewParseError(p.peek(), errors.New("'define' do not accept any expression"))
				err.Range.End = lastTokenInInstruction.Range.End
				return defineExpression, err
			}

			return defineExpression, nil

		} else if bytes.Compare(keywordToken.Value, []byte("template")) == 0 {
			templateExpression := &TemplateStatementNode{}
			templateExpression.kind = KIND_USE_TEMPLATE
			templateExpression.rng = keywordToken.Range
			templateExpression.rng.End = lastTokenInInstruction.Range.End
			templateExpression.parent = nil

			p.nextToken() // skip 'template' tokens

			if !p.accept(lexer.STRING) {
				err := NewParseError(p.peek(), errors.New("'template' expect a string next to it"))
				return templateExpression, err
			}

			templateExpression.TemplateName = p.peek()
			p.nextToken()

			expression, err := p.StatementParser()
			templateExpression.Expression = expression

			if err != nil {
				return templateExpression, err // partial AST are meant for debugging
			}

			if expression == nil { // because if err == nil, then expression != nil
				panic("returned AST was nil although parsing completed succesfully. can't add expression to ControlFlow\n" + templateExpression.String())
			}

			if templateExpression.Range().End != expression.Range().End {
				panic("ending location mismatch between 'range' statement and its expression\n" + templateExpression.String())
			}

			switch expression.Kind() {
			case KIND_VARIABLE_ASSIGNMENT, KIND_VARIABLE_DECLARATION, KIND_MULTI_EXPRESSION, KIND_EXPRESSION:

			default:
				err = NewParseError(templateExpression.TemplateName, errors.New("'template' do not accept those type of expression"))
				err.Range = expression.Range()
				return templateExpression, err
			}

			return templateExpression, nil
		}

	} else if p.acceptAt(1, lexer.DECLARATION_ASSIGNEMENT) || p.acceptAt(3, lexer.DECLARATION_ASSIGNEMENT) {
		varDeclarationNode, err := p.declarationAssignmentParser()

		if err != nil {
			return varDeclarationNode, err // partial AST are meant for debugging
		}

		if !p.expect(lexer.EOL) {
			err = NewParseError(p.peek(), errors.New("syntax for assignment delcaration didn't end properly. extraneous expression"))
			err.Range.End = lastTokenInInstruction.Range.End
			return varDeclarationNode, err
		}

		return varDeclarationNode, nil

	} else if p.acceptAt(1, lexer.ASSIGNEMENT) || p.acceptAt(3, lexer.ASSIGNEMENT) {
		varInitialization, err := p.initializationAssignmentParser()

		if err != nil {
			return varInitialization, err // partial AST are meant for debugging
		}

		if !p.expect(lexer.EOL) {
			err = NewParseError(p.peek(), errors.New("syntax for assignment didn't end properly. extraneous expression"))
			err.Range.End = lastTokenInInstruction.Range.End
			return varInitialization, err
		}

		return varInitialization, nil

	} else if p.accept(lexer.COMMENT) {
		commentExpression := &CommentNode{kind: KIND_COMMENT, Value: p.peek(), rng: p.peek().Range}

		p.nextToken()

		if !p.expect(lexer.EOL) {
			err := NewParseError(p.peek(), errors.New("syntax for comment didn't end properly. extraneous expression"))
			err.Range.End = lastTokenInInstruction.Range.End
			return commentExpression, err
		}

		// Check that this comment contains go code to semantically analize
		lookForAndSetGoCodeInComment(commentExpression)

		return commentExpression, nil
	}

	// 4. Default parser whenever no other parser have been enabled

	multiExpression, err := p.multiExpressionParser()
	if err != nil {
		return multiExpression, err // partial AST are meant for debugging
	}

	if !p.expect(lexer.EOL) {

		var errMsg error
		if p.expect(lexer.COMMA) {
			errMsg = errors.New("more than 2 variables detected for declaration/assignment")
		} else {
			errMsg = errors.New("expected end of expression, but got extraneous tokens")
		}

		err = NewParseError(p.peek(), errMsg)
		err.Range.End = lastTokenInInstruction.Range.End

		return multiExpression, err
	}

	return multiExpression, nil
}

func (p *Parser) declarationAssignmentParser() (*VariableDeclarationNode, *ParseError) {

	lastTokenInInstruction := p.peekAtEndCurrentInstruction() // token before 'EOL'
	if lastTokenInInstruction == nil {
		panic("unexpected empty token found at end of the current instruction")
	}

	varDeclarationNode := &VariableDeclarationNode{}
	varDeclarationNode.kind = KIND_VARIABLE_DECLARATION
	varDeclarationNode.rng = p.peek().Range
	varDeclarationNode.rng.End = lastTokenInInstruction.Range.End

	count := 0
	for {
		count++

		if count > 2 {
			err := NewParseError(p.peek(), errors.New("only one or two variables can be declared at once"))
			err.Range = varDeclarationNode.rng
			return nil, err
		}

		if !p.accept(lexer.DOLLAR_VARIABLE) {
			err := NewParseError(p.peek(), errors.New("variable name must start with '$'"))
			return nil, err
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
		err := NewParseError(p.peek(), errors.New("expected assignment ':=', but found something else"))
		return nil, err
	}

	expression, err := p.multiExpressionParser()
	varDeclarationNode.Value = expression

	if err != nil {
		return varDeclarationNode, err // partial AST are meant for debugging
	}

	if expression == nil { // because if err == nil, then expression != nil
		panic("returned AST was nil although parsing completed succesfully. can't be added to ControlFlow\n" + varDeclarationNode.String())
	}

	if varDeclarationNode.Range().End != expression.Range().End {
		panic("ending location mismatch between 'if' statement and its expression\n" + varDeclarationNode.String())
	}

	return varDeclarationNode, nil
}

func (p *Parser) initializationAssignmentParser() (*VariableAssignationNode, *ParseError) {

	lastTokenInInstruction := p.peekAtEndCurrentInstruction() // token before 'EOL'
	if lastTokenInInstruction == nil {
		panic("unexpected empty token found at end of the current instruction")
	}

	varAssignation := &VariableAssignationNode{}
	varAssignation.kind = KIND_VARIABLE_ASSIGNMENT
	varAssignation.rng = p.peek().Range
	varAssignation.rng.End = lastTokenInInstruction.Range.End

	// varAssignation.VariableName = variable
	count := 0
	for {
		count++

		if count > 2 {
			err := NewParseError(p.peek(), errors.New("only one or two variables can be declared at once"))
			err.Range = varAssignation.rng
			return nil, err
		}

		if !p.accept(lexer.DOLLAR_VARIABLE) {
			// err := NewParseError(p.peek(), errors.New("variable name must start with '$'"))
			// BUG: replace the code below by the one above
			err := NewParseError(p.peek(), errors.New("variable name must start with '$'; assign ; "+string(p.peek().Value)))
			return nil, err
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
		err := NewParseError(p.peek(), errors.New("expected assignment '=', but found something else"))
		return nil, err
	}

	expression, err := p.multiExpressionParser()
	varAssignation.Value = expression
	if err != nil {
		return varAssignation, err // partial AST are meant for debugging
	}

	return varAssignation, nil
}

func (p *Parser) multiExpressionParser() (*MultiExpressionNode, *ParseError) {
	lastTokenInInstruction := p.peekAtEndCurrentInstruction() // token before 'EOL'
	if lastTokenInInstruction == nil {
		panic("unexpected empty token found at end of the current instruction")
	}

	multiExpression := &MultiExpressionNode{}
	multiExpression.kind = KIND_MULTI_EXPRESSION
	multiExpression.rng.Start = p.peek().Range.Start
	multiExpression.rng.End = lastTokenInInstruction.Range.End

	var expression *ExpressionNode
	var err *ParseError

	/*
		expression, err := p.expressionParser()
		multiExpression.Expressions = append(multiExpression.Expressions, expression)

		if err != nil {
			err.Range = lastTokenInInstruction.Range
			err.Range.Start = lastTokenInInstruction.Range.End
			err.Range.Start.Character--
			return multiExpression, err
		}

		if expression == nil { // because if err == nil, then expression != nil
			panic("returned AST was nil although parsing completed succesfully. can't be added to ControlFlow\n" + multiExpression.String())
		}
	*/

	for next := true; next; next = p.expect(lexer.PIPE) {
		expression, err = p.expressionParser()
		multiExpression.Expressions = append(multiExpression.Expressions, expression)

		if err != nil {
			/*
				err.Range = lastTokenInInstruction.Range
				err.Range.Start = lastTokenInInstruction.Range.End
				err.Range.Start.Character--
			*/
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
	lastTokenInInstruction := p.peekAtEndCurrentInstruction() // token before 'EOL'
	if lastTokenInInstruction == nil {
		panic("unexpected empty token found at end of the current instruction")
	}

	expression := &ExpressionNode{}
	expression.kind = KIND_EXPRESSION
	expression.rng.Start = p.peek().Range.Start
	expression.rng.End = lastTokenInInstruction.Range.End

	// var currentSymbol *lexer.Token
	currentSymbol := p.peek()
	depth := 0

	depth, err := p.symbolParser(expression, depth)
	if err != nil {
		return expression, err
	}

	if depth < 0 {
		currentSymbol = getLastElement(expression.Symbols) // since depth != 0 then len(expression.Symboles) > 0
		err = NewParseError(currentSymbol, errors.New("no matching opening bracket '('"))
		return expression, err
	} else if depth > 0 {
		err = NewParseError(currentSymbol, errors.New("no matching closing bracket ')'"))
		return expression, err
	}

	// TODO: Remove this comment below whenever useless
	/*
		for p.accept(lexer.FUNCTION) || p.accept(lexer.DOT_VARIABLE) || p.accept(lexer.DOLLAR_VARIABLE) || p.accept(lexer.STRING) || p.accept(lexer.NUMBER) ||
			p.accept(lexer.LEFT_PAREN) || p.accept(lexer.RIGTH_PAREN) {
			if p.peek().ID == lexer.LEFT_PAREN {
				return p.symbolParser(depth + 1)
			} else if p.peek().ID == lexer.RIGTH_PAREN {
				return depth - 1
			}

			currentSymbol = p.peek()
			expression.Symbols = append(expression.Symbols, currentSymbol)

			p.nextToken()
		}
	*/

	if p.isEOF() {
		panic("'expression' parser reached the end of instructions unexpectedly\n" + expression.String())
	}

	if len(expression.Symbols) == 0 {
		err := NewParseError(p.peek(), errors.New("expected an expression but got nothing"))
		err.Range.Start = p.peek().Range.End
		err.Range.Start.Character--

		return expression, err
	}

	currentSymbol = getLastElement(expression.Symbols)
	expression.rng.End = currentSymbol.Range.End

	return expression, nil
}

func (p *Parser) symbolParser(expression *ExpressionNode, depth int) (int, *ParseError) {
	var currentSymbol *lexer.Token
	var err *ParseError

	for p.accept(lexer.FUNCTION) || p.accept(lexer.DOT_VARIABLE) || p.accept(lexer.DOLLAR_VARIABLE) || p.accept(lexer.STRING) || p.accept(lexer.NUMBER) ||
		p.accept(lexer.LEFT_PAREN) || p.accept(lexer.RIGTH_PAREN) {

		currentSymbol = p.peek()
		expression.Symbols = append(expression.Symbols, currentSymbol)

		p.nextToken()

		if currentSymbol.ID == lexer.LEFT_PAREN {
			var newDepth int
			newDepth, err = p.symbolParser(expression, depth+1)

			if newDepth != depth { // In this case this is always true, newDepth >= depth
				// unclosed left paren
				err = NewParseError(currentSymbol, errors.New("parenthesis not closed"))
			}

			if p.peek().ID == lexer.RIGTH_PAREN {
				err = NewParseError(currentSymbol, errors.New("empty sub-expression"))
			}

			// return depth - 1, err
		} else if currentSymbol.ID == lexer.RIGTH_PAREN {
			return depth - 1, nil
		}
	}

	return depth, err
}

func lookForAndSetGoCodeInComment(commentExpression *CommentNode) {
	const SEP_COMMENT_GOCODE = "go:code"
	comment := commentExpression.Value.Value

	before, after, found := bytes.Cut(comment, []byte(SEP_COMMENT_GOCODE))
	if !found {
		// log.Printf("SEP not found :: sep = %s :: comment = %q\n", SEP_COMMENT_GOCODE, comment)
		return
	} else if len(after) == 0 {
		return
	}

	if len(bytes.TrimSpace(before)) > 0 {
		// log.Printf("SEP obfuscated by garbage :: before sep = %q :: comment = %q\n", before, comment)
		return
	}

	// if bytes.TrimSpace(before) == 0; then execute below
	comment = after

	switch comment[0] {
	case ' ', '\n', '\t', '\r', '\v', '\f': // unicode.IsSpace(rune(comment[0]))
		// continue to next step succesfully
	default:
		// TODO: report this error to user instead of only printing it in the log ?
		// log.Printf("after SEP, no separation 'space' = %q\n", comment)
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
	if len(p.input) == 0 {
		return nil
	}

	return &p.input[0]
}

func (p Parser) peekAt(pos int) *lexer.Token {
	if pos < len(p.input) {
		return &p.input[pos]
	}

	return nil
}

// Return the last token before 'EOL' from the current instruction
func (p Parser) peekAtEndCurrentInstruction() *lexer.Token {
	if len(p.input) == 0 {
		return nil
	}

	last := &p.input[0]

	for _, tok := range p.input {
		if tok.ID == lexer.EOL {
			break
		}
		last = &tok
	}

	return last
}

func (p *Parser) nextToken() {
	if len(p.input) == 0 {
		return
	}

	p.input = p.input[1:]
}

// Whenever a parse error happen for a ligne (series of tokens ending with token 'EOL'),
// the tokens of the statements (line) are not fully read.
// Thus to accurate parse the next statement/line, you must flush tokens of the erroneous statement
func (p *Parser) flushInputUntilNextStatement() {
	if len(p.input) == 0 {
		return
	}

	for index, el := range p.input {
		if el.ID == lexer.EOL {
			index++
			p.input = p.input[index:]

			return
		}
	}

	panic("tokens malformated used during parsing. missing 'EOL' at the end of instruction tokens")
}

func (p Parser) accept(kind lexer.Kind) bool {
	if len(p.input) == 0 {
		return false
	}
	return p.input[0].ID == kind
}

func (p Parser) acceptAt(pos int, kind lexer.Kind) bool {
	if len(p.input) == 0 {
		return false
	}

	if pos >= len(p.input) {
		return false
	}

	return p.input[pos].ID == kind
}

func (p *Parser) expect(kind lexer.Kind) bool {
	if p.accept(kind) {
		p.nextToken()

		return true
	}

	return false
}

func (p Parser) isEOF() bool {
	return len(p.input) == 0
}

func (p *Parser) incRecursionDepth() {
	p.currentRecursionDepth++
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
