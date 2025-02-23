package parser

import (
	"bytes"
	"errors"
	"log"	// TODO: to remove

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

	defaultGroupStatementNode := &GroupStatementNode{
		Kind: KIND_GROUP_STATEMENT,
		isRoot: true,
	}

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

func appendStatementToCurrentScope(scopeStack []*GroupStatementNode, statement AstNode) {
	if statement == nil {
		panic("cannot add empty statement to 'group'")
	}

	if len(scopeStack) == 0 {
		panic("cannot add statement to empty group. Group must be created before hand")
	}

	currentScope := getLastElement(scopeStack)
	currentScope.Statements = append(currentScope.Statements, statement)
}

func (p *Parser) safeStatementGrouping(node AstNode) *ParseError {
	if node == nil {
		return nil
	}

	// change var name to : "openedGroupNodeStack", "activeScopeNodes", "activeScopeStack", "openedScopeStack"
	if len(p.openedNodeStack) == 0 {
		panic("no initial scope available to hold the statements. There must always exist at least one 'scope/group' at any moment")
	}

	if p.openedNodeStack[0].isRoot == false {
		panic("root node has'nt been marked as such. root node, and only the root node, can be marked as 'isRoot'")
	}

	var err *ParseError
	var ROOT_SCOPE *GroupStatementNode = p.openedNodeStack[0]

	newGroup, isScope := node.(*GroupStatementNode)

	if !isScope {
		appendStatementToCurrentScope(p.openedNodeStack, node)
	} else {
		if newGroup.isRoot == true {
			log.Printf("non-root node cannot be flaged as 'root'.\n culprit node = %#v\n", newGroup)
			panic("only the root node, can be marked as 'isRoot', but found it on non-root node")
		}

		stackSize := len(p.openedNodeStack)
		lastInserted := getLastElement(p.openedNodeStack)
		newGroup.parent = lastInserted

		switch newGroup.GetKind() {
		case KIND_IF, KIND_WITH, KIND_RANGE_LOOP, KIND_BLOCK_TEMPLATE, KIND_DEFINE_TEMPLATE:
			newGroup.parent = getLastElement(p.openedNodeStack)
			appendStatementToCurrentScope(p.openedNodeStack, newGroup)

			p.openedNodeStack = append(p.openedNodeStack, newGroup)

		case KIND_ELSE_IF:
			if stackSize > 1 {
				switch lastInserted.GetKind() {
				case KIND_IF, KIND_ELSE_IF:
					// Remove the last element from the stack and switch it with 'KIND_ELSE_IF' scope
					p.openedNodeStack = p.openedNodeStack[:stackSize-1]

					newGroup.parent = getLastElement(p.openedNodeStack)
					appendStatementToCurrentScope(p.openedNodeStack, newGroup)

					p.openedNodeStack = append(p.openedNodeStack, newGroup)
				default:
					err = &ParseError{Range: lastInserted.GetRange(),
						Err: errors.New("'else if' statement is not compatible with '" + lastInserted.GetKind().String() + "'")}
				}
			} else {
				err = &ParseError{Range: newGroup.GetRange(),
					Err: errors.New("extraneous statement '" + newGroup.GetKind().String() + "'")}
			}
		case KIND_ELSE_WITH:
			if stackSize > 1 {
				switch lastInserted.GetKind() {
				case KIND_WITH, KIND_ELSE_WITH:
					// Remove the last element from the stack and switch it with 'KIND_ELSE_WITH' scope
					p.openedNodeStack = p.openedNodeStack[:stackSize-1]

					newGroup.parent = getLastElement(p.openedNodeStack)
					appendStatementToCurrentScope(p.openedNodeStack, newGroup)

					p.openedNodeStack = append(p.openedNodeStack, newGroup)
				default:
					err = &ParseError{Range: lastInserted.GetRange(),
						Err: errors.New("'else with' statement is not compatible with '" + lastInserted.GetKind().String() + "'")}
				}
			} else {
				err = &ParseError{Range: newGroup.GetRange(),
					Err: errors.New("extraneous statement '" + newGroup.GetKind().String() + "'")}
			}
		case KIND_ELSE:
			if stackSize > 1 {
				switch lastInserted.GetKind() {
				case KIND_IF, KIND_ELSE_IF, KIND_WITH, KIND_ELSE_WITH, KIND_RANGE_LOOP:
					// Remove the last element from the stack and switch it with 'KIND_ELSE' scope
					p.openedNodeStack = p.openedNodeStack[:stackSize-1]

					newGroup.parent = getLastElement(p.openedNodeStack)
					appendStatementToCurrentScope(p.openedNodeStack, newGroup)

					p.openedNodeStack = append(p.openedNodeStack, newGroup)
				default:
					err = &ParseError{Range: newGroup.GetRange(),
						Err: errors.New("'else' statement is not compatible with '" + newGroup.GetKind().String() + "'")}
				}
			} else {
				err = &ParseError{Range: newGroup.GetRange(),
					Err: errors.New("extraneous statement '" + newGroup.GetKind().String() + "'")}
			}
		case KIND_END:
			if stackSize > 1 {
				scopeToClose := getLastElement(p.openedNodeStack)
				scopeToClose.Range.End = newGroup.Range.Start

				p.openedNodeStack = p.openedNodeStack[:stackSize-1]

				newGroup.parent = getLastElement(p.openedNodeStack)
				appendStatementToCurrentScope(p.openedNodeStack, newGroup)
			} else {
				err = &ParseError{Range: newGroup.GetRange(), Err: errors.New("extraneous 'end' statement detected")}
			}
		default:
			log.Printf("unhandled scope type error\n scope = %#v\n", newGroup)
			panic("scope type '" + newGroup.GetKind().String() + "' is not yet handled for statement grouping\n" + newGroup.String())
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

	if len(parser.openedNodeStack) > 1 {
		lastInserted := getLastElement(parser.openedNodeStack)
		err := ParseError{Range: lastInserted.GetRange(),
			Err: errors.New("not all group statements ('if/else/define/block/with') have been properly claused")}

		errs = append(errs, err)
	}

	if size := len(defaultGroupStatementNode.Statements); size > 0 {
		defaultGroupStatementNode.Range.Start = defaultGroupStatementNode.Statements[0].GetRange().Start
		defaultGroupStatementNode.Range.End = defaultGroupStatementNode.Statements[size-1].GetRange().End
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
	if p.acceptAt(1, lexer.DECLARATION_ASSIGNEMENT) {
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

	} else if p.acceptAt(1, lexer.ASSIGNEMENT) {
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
		commentExpression := &CommentNode{Kind: KIND_COMMENT, Value: p.peek(), Range: p.peek().Range}

		p.nextToken()

		if !p.expect(lexer.EOL) {
			err := NewParseError(p.peek(), errors.New("syntax for comment didn't end properly. extraneous expression"))
			err.Range.End = lastTokenInInstruction.Range.End
			return commentExpression, err
		}

		// Check that this comment contains go code to semantically analize
		lookForAndSetGoCodeInComment(commentExpression)

		return commentExpression, nil

	} else if p.accept(lexer.KEYWORD) {
		keywordToken := p.peek()

		// INFO: most composite statements (else if xxx, etc) do not check for 'EOL' on purpose

		if bytes.Compare(keywordToken.Value, []byte("if")) == 0 {
			ifExpression := &GroupStatementNode{}
			ifExpression.Kind = KIND_IF
			ifExpression.Range = keywordToken.Range
			ifExpression.Range.End = lastTokenInInstruction.Range.End

			p.nextToken() // skip keyword "if"

			expression, err := p.StatementParser()
			ifExpression.ControlFlow = expression

			if err != nil {
				return ifExpression, err // partial AST are meant for debugging
			}

			if expression == nil { // because if err == nil, then expression != nil
				panic("returned AST was nil although parsing completed succesfully. can't be added to ControlFlow\n" + ifExpression.String())
			}

			if ifExpression.Range.End != expression.GetRange().End {
				panic("ending location mismatch between 'if' statement and its expression\n" + ifExpression.String())
			}

			switch expression.GetKind() {
			case KIND_VARIABLE_ASSIGNMENT, KIND_VARIABLE_DECLARATION, KIND_MULTI_EXPRESSION, KIND_EXPRESSION:

			default:
				err = NewParseError(&lexer.Token{}, errors.New("'if' do not accept this kind of statement"))
				err.Range = expression.GetRange()
				return ifExpression, err
			}

			return ifExpression, nil

		} else if bytes.Compare(keywordToken.Value, []byte("else")) == 0 {
			elseExpression := &GroupStatementNode{}
			elseExpression.Kind = KIND_ELSE
			elseExpression.Range = keywordToken.Range
			elseExpression.Range.End = lastTokenInInstruction.Range.End

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

			if elseCompositeExpression.GetRange().End != lastTokenInInstruction.Range.End {
				panic("ending location mismatch between 'else if/with/...' statement and its expression\n" + elseCompositeExpression.String())
			}

			switch elseCompositeExpression.GetKind() {
			case KIND_IF:
				elseCompositeExpression.SetKind(KIND_ELSE_IF)
			case KIND_WITH:
				elseCompositeExpression.SetKind(KIND_ELSE_WITH)
			default:
				err = NewParseError(keywordToken, errors.New("bad syntax for else statement"))
				err.Range = elseCompositeExpression.GetRange()
				return elseCompositeExpression, err
			}

			return elseCompositeExpression, nil

		} else if bytes.Compare(keywordToken.Value, []byte("end")) == 0 {
			endExpression := &GroupStatementNode{}
			endExpression.Kind = KIND_END
			endExpression.Range = keywordToken.Range
			endExpression.Range.End = lastTokenInInstruction.Range.End

			p.nextToken() // skip 'end' token

			if !p.expect(lexer.EOL) {
				err := NewParseError(keywordToken, errors.New("expression next to 'end' is diasallowed"))
				err.Range.End = lastTokenInInstruction.Range.End
				return endExpression, err
			}

			return endExpression, nil

		} else if bytes.Compare(keywordToken.Value, []byte("range")) == 0 {
			rangeExpression := &GroupStatementNode{}
			rangeExpression.Kind = KIND_RANGE_LOOP
			rangeExpression.Range = keywordToken.Range
			rangeExpression.Range.End = lastTokenInInstruction.Range.End

			p.nextToken()

			var expression AstNode
			var err *ParseError

			if p.acceptAt(1, lexer.COMMA) {
				expression, err = p.doubleDeclarationAssignmentParser()

				if !p.expect(lexer.EOL) {
					err = NewParseError(keywordToken, errors.New("'range' statement has extraneous expression"))
					err.Range.End = lastTokenInInstruction.Range.End
					return rangeExpression, err
				}
			} else {
				expression, err = p.StatementParser()
			}

			rangeExpression.ControlFlow = expression

			if err != nil {
				return rangeExpression, err // partial AST are meant for debugging
			}

			if expression == nil { // because if err == nil, then expression != nil
				panic("returned AST was nil although parsing completed succesfully. can't be added to ControlFlow\n" + rangeExpression.String())
			}

			if rangeExpression.Range.End != expression.GetRange().End {
				panic("ending location mismatch between 'range' statement and its expression\n" + rangeExpression.String())
			}

			switch expression.GetKind() {
			case KIND_VARIABLE_ASSIGNMENT, KIND_VARIABLE_DECLARATION, KIND_MULTI_EXPRESSION, KIND_EXPRESSION:

			default:
				err = NewParseError(lastTokenInInstruction, errors.New("'range' do not accept those type of expression"))
				err.Range = expression.GetRange()
				return rangeExpression, err
			}

			return rangeExpression, nil

		} else if bytes.Compare(keywordToken.Value, []byte("with")) == 0 {
			withExpression := &GroupStatementNode{}
			withExpression.Kind = KIND_WITH
			withExpression.Range = keywordToken.Range
			withExpression.Range.End = lastTokenInInstruction.Range.End

			p.nextToken() // skip 'with' token

			expression, err := p.StatementParser()
			withExpression.ControlFlow = expression

			if err != nil {
				return withExpression, err // partial AST are meant for debugging
			}

			if expression == nil { // because if err == nil, then expression != nil
				panic("returned AST was nil although parsing completed succesfully. 'with ...' parse error")
			}

			if withExpression.Range.End != expression.GetRange().End {
				panic("ending location mismatch between 'range' statement and its expression\n" + withExpression.String())
			}

			switch expression.GetKind() {
			case KIND_VARIABLE_ASSIGNMENT, KIND_VARIABLE_DECLARATION, KIND_MULTI_EXPRESSION, KIND_EXPRESSION:

			default:
				err = NewParseError(lastTokenInInstruction, errors.New("'with' do not accept those type of expression"))
				err.Range = expression.GetRange()
				return withExpression, err
			}

			return withExpression, nil

		} else if bytes.Compare(keywordToken.Value, []byte("block")) == 0 {
			blockExpression := &GroupStatementNode{}
			blockExpression.Kind = KIND_BLOCK_TEMPLATE
			blockExpression.Range = keywordToken.Range
			blockExpression.Range.End = lastTokenInInstruction.Range.End

			p.nextToken() // skip 'block' token

			if !p.accept(lexer.STRING) {
				err := NewParseError(p.peek(), errors.New("'block' expect a string next to it"))
				return blockExpression, err
			}

			templateExpression := &TemplateStatementNode{Kind: KIND_BLOCK_TEMPLATE, TemplateName: p.peek(), Range: p.peek().Range}
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

			if blockExpression.Range.End != expression.GetRange().End {
				panic("ending location mismatch between 'range' statement and its expression\n" + blockExpression.String())
			}

			switch expression.GetKind() {
			case KIND_VARIABLE_ASSIGNMENT, KIND_VARIABLE_DECLARATION, KIND_MULTI_EXPRESSION, KIND_EXPRESSION:

			default:
				err = NewParseError(templateExpression.TemplateName, errors.New("'block' do not accept those type of expression"))
				err.Range = expression.GetRange()
				return blockExpression, err
			}

			return blockExpression, nil

		} else if bytes.Compare(keywordToken.Value, []byte("define")) == 0 {
			defineExpression := &GroupStatementNode{}
			defineExpression.Kind = KIND_DEFINE_TEMPLATE
			defineExpression.Range = keywordToken.Range
			defineExpression.Range.End = lastTokenInInstruction.Range.End

			p.nextToken() // skip 'define' token

			if !p.accept(lexer.STRING) {
				err := NewParseError(p.peek(), errors.New("'define' expect a string next to it"))
				return defineExpression, err
			}

			templateExpression := &TemplateStatementNode{Kind: KIND_DEFINE_TEMPLATE, TemplateName: p.peek(), Range: p.peek().Range}
			templateExpression.parent = defineExpression
			defineExpression.ControlFlow = templateExpression

			p.nextToken()

			if !p.expect(lexer.EOL) {
				err := NewParseError(p.peek(), errors.New("'define' do not accept any expression after its name"))
				err.Range.End = lastTokenInInstruction.Range.End
				return defineExpression, err
			}

			return defineExpression, nil

		} else if bytes.Compare(keywordToken.Value, []byte("template")) == 0 {
			templateExpression := &TemplateStatementNode{}
			templateExpression.Kind = KIND_USE_TEMPLATE
			templateExpression.Range = keywordToken.Range
			templateExpression.Range.End = lastTokenInInstruction.Range.End
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

			if templateExpression.Range.End != expression.GetRange().End {
				panic("ending location mismatch between 'range' statement and its expression\n" + templateExpression.String())
			}

			switch expression.GetKind() {
			case KIND_VARIABLE_ASSIGNMENT, KIND_VARIABLE_DECLARATION, KIND_MULTI_EXPRESSION, KIND_EXPRESSION:

			default:
				err = NewParseError(templateExpression.TemplateName, errors.New("'template' do not accept those type of expression"))
				err.Range = expression.GetRange()
				return templateExpression, err
			}

			return templateExpression, nil
		}
	}

	// 4. Default parser whenever no other parser have been enabled

	multiExpression, err := p.multiExpressionParser()
	if err != nil {
		return multiExpression, err // partial AST are meant for debugging
	}

	if !p.expect(lexer.EOL) {
		err = NewParseError(p.peek(), errors.New("expected end of expression, but got extraneous expression"))
		err.Range.End = lastTokenInInstruction.Range.End
		return multiExpression, err
	}

	return multiExpression, nil
}

func (p *Parser) declarationAssignmentParser() (*VariableDeclarationNode, *ParseError) {
	if !p.accept(lexer.DOLLAR_VARIABLE) {
		err := NewParseError(p.peek(), errors.New("variable name must start by '$' and contains alphanumerical char"))
		return nil, err
	}

	variable := p.peek()
	p.nextToken()

	if !p.expect(lexer.DECLARATION_ASSIGNEMENT) {
		err := NewParseError(p.peek(), errors.New("expected assignmement '=', but found something else"))
		return nil, err
	}

	lastTokenInInstruction := p.peekAtEndCurrentInstruction() // token before 'EOL'
	if lastTokenInInstruction == nil {
		panic("unexpected empty token found at end of the current instruction")
	}

	varDeclarationNode := &VariableDeclarationNode{}
	varDeclarationNode.Kind = KIND_VARIABLE_DECLARATION
	varDeclarationNode.Range = variable.Range
	varDeclarationNode.Range.End = lastTokenInInstruction.Range.End
	varDeclarationNode.VariableNames = append(varDeclarationNode.VariableNames, variable)

	expression, err := p.multiExpressionParser()
	varDeclarationNode.Value = expression

	if err != nil {
		return varDeclarationNode, err // partial AST are meant for debugging
	}

	if expression == nil { // because if err == nil, then expression != nil
		panic("returned AST was nil although parsing completed succesfully. can't be added to ControlFlow\n" + varDeclarationNode.String())
	}

	if varDeclarationNode.Range.End != expression.GetRange().End {
		panic("ending location mismatch between 'if' statement and its expression\n" + varDeclarationNode.String())
	}

	return varDeclarationNode, nil
}

func (p *Parser) doubleDeclarationAssignmentParser() (*VariableDeclarationNode, *ParseError) {
	if !p.accept(lexer.DOLLAR_VARIABLE) {
		err := NewParseError(p.peek(), errors.New("expected variable begining with '$' but got something else"))
		return nil, err
	}

	firstVariable := p.peek()
	p.nextToken()

	if !p.expect(lexer.COMMA) {
		err := NewParseError(p.peek(), errors.New("expected ',' to separate variable while declaring them"))
		return nil, err
	}

	varDeclarationNode, err := p.declarationAssignmentParser()
	if err != nil {
		return varDeclarationNode, err // partial AST are meant for debugging
	}

	if varDeclarationNode == nil { // because if err == nil, then expression != nil
		panic("returned AST was nil although parsing completed succesfully. can't be added to ControlFlow\n" + varDeclarationNode.String())
	}

	if len(varDeclarationNode.VariableNames) != 1 {
		err = NewParseError(firstVariable, errors.New("expected one variable after ',' but got something else"))
		err.Range = varDeclarationNode.Range
		return varDeclarationNode, err
	}

	secondVariable := varDeclarationNode.VariableNames[0]
	varDeclarationNode.VariableNames = nil
	varDeclarationNode.VariableNames = append(varDeclarationNode.VariableNames, firstVariable, secondVariable)
	varDeclarationNode.Range.Start = firstVariable.Range.Start

	return varDeclarationNode, nil
}

func (p *Parser) initializationAssignmentParser() (*VariableAssignationNode, *ParseError) {
	if !p.accept(lexer.DOLLAR_VARIABLE) {
		err := NewParseError(p.peek(), errors.New("the variable name must start by '$' or '.', and only contains alphanumerical char"))
		return nil, err
	}

	variable := p.peek()
	p.nextToken()

	if !p.expect(lexer.ASSIGNEMENT) {
		err := NewParseError(p.peek(), errors.New("expected assignmement '=', but found something else"))
		return nil, err
	}

	lastTokenInInstruction := p.peekAtEndCurrentInstruction() // token before 'EOL'
	if lastTokenInInstruction == nil {
		panic("unexpected empty token found at end of the current instruction")
	}

	varAssignation := &VariableAssignationNode{}
	varAssignation.Kind = KIND_VARIABLE_ASSIGNMENT
	varAssignation.VariableName = variable
	varAssignation.Range = variable.Range
	varAssignation.Range.End = lastTokenInInstruction.Range.End

	expression, err := p.multiExpressionParser()
	varAssignation.Value = expression
	if err != nil {
		return varAssignation, err // partial AST are meant for debugging
	}

	if varAssignation == nil { // because if err == nil, then expression != nil
		panic("returned AST was nil although parsing completed succesfully. can't be added to ControlFlow\n" + varAssignation.String())
	}

	varAssignation.Range.End = expression.Range.End

	return varAssignation, nil
}

func (p *Parser) multiExpressionParser() (*MultiExpressionNode, *ParseError) {
	lastTokenInInstruction := p.peekAtEndCurrentInstruction() // token before 'EOL'
	if lastTokenInInstruction == nil {
		panic("unexpected empty token found at end of the current instruction")
	}

	multiExpression := &MultiExpressionNode{}
	multiExpression.Kind = KIND_MULTI_EXPRESSION
	multiExpression.Range.Start = p.peek().Range.Start
	multiExpression.Range.End = lastTokenInInstruction.Range.End

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

	for next := true ; next ; next = p.expect(lexer.PIPE) {
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

	multiExpression.Range.End = expression.Range.End

	return multiExpression, nil
}

func (p *Parser) expressionParser() (*ExpressionNode, *ParseError) {
	lastTokenInInstruction := p.peekAtEndCurrentInstruction() // token before 'EOL'
	if lastTokenInInstruction == nil {
		panic("unexpected empty token found at end of the current instruction")
	}

	expression := &ExpressionNode{}
	expression.Kind = KIND_EXPRESSION
	expression.Range.Start = p.peek().Range.Start
	expression.Range.End = lastTokenInInstruction.Range.End

	// var currentSymbol *lexer.Token
	currentSymbol := p.peek()
	depth := 0

	depth, err := p.symbolParser(expression, depth)
	if err != nil {
		return expression, err
	}

	if depth < 0 {
		currentSymbol = getLastElement(expression.Symbols)		// since depth != 0 then len(expression.Symboles) > 0
		err = NewParseError(currentSymbol, errors.New("no matching opening bracket '('"))
		return expression, err
	} else if depth > 0 {
		err = NewParseError(currentSymbol, errors.New("no matching closing bracket ')'"))
		return expression, err
	}


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
	expression.Range.End = currentSymbol.Range.End

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
			newDepth, err = p.symbolParser(expression, depth + 1)

			if newDepth != depth {	// In this case this is always true, newDepth >= depth
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
	comment := commentExpression.Value.Value
	comment = bytes.TrimSpace(comment)

	const SEP_COMMENT_GOCODE = "go:code"
	if ! bytes.HasPrefix(comment, []byte(SEP_COMMENT_GOCODE)) {
		log.Printf("SEP not found :: sep = %s :: comment = %q\n", SEP_COMMENT_GOCODE, comment)
		return
	}

	start := len(SEP_COMMENT_GOCODE)
	comment = comment[start:]

	if len(comment) == 0 {
		log.Printf("after SEP, comment too short comment = %q\n", comment)
		return
	}

	first := comment[0]
	if ! (first == ' ' || first == '\n' || first == '\t' || first == '\r') {
		log.Printf("after SEP, no separation 'space' = %q\n", comment)
		return
	}

	comment = bytes.TrimLeft(comment, SEP_COMMENT_GOCODE)
	commentExpression.GoCode = comment
	log.Printf("\nHourray, go:code found : %q\n\n", comment)
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

func (p Parser) isEOL() bool {
	if len(p.input) == 0 {
		return false
	}

	return p.input[0].ID == lexer.EOL
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
