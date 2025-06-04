// TODO: features to add
//
// [ ] Unused variable detection
// [ ] Check that 'keyword' and 'types' are not used as parameter names (functions)
// [x] Function declaration in comment have invalid 'Range'
// [ ] Dectection of cyclical import
// [ ] Advanced type-system
// [ ] Go-To Definition
// [ ] Testing for major stable features

package analyzer

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	goParser "go/parser"
	"go/scanner"
	"go/token"
	"go/types"
	"log"
	"maps"
	"reflect"
	"strings"

	"github.com/yayolande/gota/lexer"
	"github.com/yayolande/gota/parser"
)

var (
	TYPE_ANY   = types.Universe.Lookup("any")
	TYPE_ERROR = types.Universe.Lookup("error")
)

func init() {
	if TYPE_ANY == nil {
		panic("initialization of 'any' type failed")
	}

	if TYPE_ERROR == nil {
		panic("initialization of 'error' type failed")
	}
}

const (
	TYPE_INT   string = "int"
	TYPE_FLOAT        = "float"
	TYPE_BOOL
	TYPE_CHAR
	TYPE_STRING = "string"
	// TYPE_ANY = "any"
	// TYPE_ERROR = "error"
	TYPE_VOID    = "void"
	TYPE_MAP     = "map"
	TYPE_ARRAY   = "array"
	TYPE_STRUCT  = "struct"
	TYPE_POINTER = "pointer"
	TYPE_ALIAS   = "alias"
	TYPE_INVALID = "invalid type"
)

// -------------------------
// Analyzer Types definition
// -------------------------

// TODO: add 'Stringer' method for FunctionDefinition, VariableDefinition, DataStructureDefinition

type FunctionDefinition struct {
	Node     parser.AstNode
	Range    lexer.Range
	FileName string

	// New comer to keep
	Name string
	Type *types.Signature
}

type VariableDefinition struct {
	Node           parser.AstNode // direct node containing info about this variable
	Range          lexer.Range    // variable lifetime
	FileName       string
	Name           string
	Type           types.Type
	ImplicitType   map[string]types.Type // Guessed tree type through 'var' usage
	IsValid        bool
	IsUsedOnce     bool // Only useful to detect whether or not a variable have never been used in the program
	UsageFrequency int
}

type TemplateDefinition struct {
	Node      parser.AstNode
	Range     lexer.Range
	FileName  string
	Name      string
	InputType types.Type
	IsValid   bool
}

type FileDefinition struct {
	Root *parser.GroupStatementNode
	Name string

	// TODO: do the same for 'ScopeToVariables'
	TypeHints        map[*parser.GroupStatementNode]types.Type
	ScopeToVariables map[*parser.GroupStatementNode][]*VariableDefinition // TODO: CHANGE THIS TO A 'MAP' !

	Functions map[string]*FunctionDefinition
	Templates map[string]*TemplateDefinition
	// WorkspaceTemplates	map[string]*TemplateDefinition
}

// func NewFileDefinition(fileName string, root *parser.GroupStatementNode, outterTemplate []*TemplateDefinition) (*FileDefinition, map[string]*VariableDefinition, map[string]*VariableDefinition) {
func NewFileDefinition(fileName string, root *parser.GroupStatementNode, outterTemplate map[*parser.GroupStatementNode]*TemplateDefinition) (*FileDefinition, map[string]*VariableDefinition, map[string]*VariableDefinition) {
	fileInfo := new(FileDefinition)

	fileInfo.Name = fileName
	fileInfo.Root = root
	fileInfo.TypeHints = make(map[*parser.GroupStatementNode]types.Type)
	fileInfo.Templates = make(map[string]*TemplateDefinition)

	// 2. build external templates available for the current file
	foundOnce := make(map[string]bool)

	for templateNode, templateDef := range outterTemplate {
		templateName := string(templateNode.ControlFlow.(*parser.TemplateStatementNode).TemplateName.Value)

		def := fileInfo.Templates[templateName]

		if def != nil {

			if !foundOnce[templateName] {
				defAny := &TemplateDefinition{
					InputType: TYPE_ANY.Type(),
					FileName:  "",
					Node:      nil,
				}

				fileInfo.Templates[templateName] = defAny
			}

			foundOnce[templateName] = true
			continue
		}

		fileInfo.Templates[templateName] = templateDef
	}

	// fileInfo.Templates = make(map[string]*TemplateDefinition)
	fileInfo.Functions = getBuiltinFunctionDefinition()
	fileInfo.ScopeToVariables = make(map[*parser.GroupStatementNode][]*VariableDefinition)

	globalVariables := make(map[string]*VariableDefinition)
	localVariables := make(map[string]*VariableDefinition)

	globalVariables["."] = NewVariableDefinition(".", nil, fileName)
	globalVariables["$"] = NewVariableDefinition("$", nil, fileName)

	return fileInfo, globalVariables, localVariables
}

func (f FileDefinition) GetScopedVariables(scope *parser.GroupStatementNode) map[string]*VariableDefinition {
	scopedVariables := make(map[string]*VariableDefinition)

	if scope == nil || f.ScopeToVariables == nil {
		return scopedVariables
	}

	listVariables, found := f.ScopeToVariables[scope]
	if !found {
		return scopedVariables
	}

	for _, variable := range listVariables {
		scopedVariables[variable.Name] = variable
	}

	return scopedVariables
}

func CloneTemplateDefinition(src *TemplateDefinition) *TemplateDefinition {
	if src == nil {
		return nil
	}

	dst := &TemplateDefinition{
		Node:      src.Node,
		Range:     src.Range,
		FileName:  src.FileName,
		Name:      src.Name,
		InputType: src.InputType,
		IsValid:   src.IsValid,
	}

	return dst
}

// TODO: alter return value to 'map[string]*TemplateDefinition'
/*
func (f FileDefinition) GetFileScopedTemplates() map[string]parser.AstNode {
	scopedTemplates := make(map[string]parser.AstNode)

	if f.Templates == nil {
		return scopedTemplates
	}

	var name string
	var node parser.AstNode

	for _, variable := range f.Templates {
		name = variable.Name
		node = variable.Node

		scopedTemplates[name] = node
	}

	return scopedTemplates
}
*/

/*
// TODO: to remove when 'Template' field is properly altered
// TODO: replace 'currentTemplate' with 'templateName'
func (f FileDefinition) GetSingleTemplateDefinition(template parser.AstNode) *TemplateDefinition {

	for _, def := range f.Templates {
		if def.Node == template {
			return def
		}
	}

	for _, def := range f.WorkspaceTemplates {
		if def.Node == template {
			return def
		}
	}

	return nil
}

// TODO: to remove when 'Template' field is properly altered
func (f FileDefinition) GetSingleTemplateDefinitionByName(templateName string) *TemplateDefinition {
	for _, def := range f.Templates {
		if def.Name == templateName {
			return def
		}
	}

	for _, def := range f.WorkspaceTemplates {
		if def.Name == templateName {
			return def
		}
	}

	return nil
}
*/

// ------------
// Start Here -
// ------------

// TODO: this need some more work to be usable universally by all files within need to be recreated each time
// TODO:	add types to every builtin functions
func getBuiltinFunctionDefinition() map[string]*FunctionDefinition {
	dict := parser.SymbolDefinition{
		"and":      nil,
		"call":     nil,
		"html":     nil,
		"index":    nil,
		"slice":    nil,
		"js":       nil,
		"len":      nil,
		"not":      nil,
		"or":       nil,
		"print":    nil,
		"printf":   nil,
		"println":  nil,
		"urlquery": nil,
		"eq":       nil,
		"ne":       nil,
		"lt":       nil,
		"le":       nil,
		"gt":       nil,
		"ge":       nil,
		"true":     nil, // unsure about this
		"false":    nil, // unsure about this
		"continue": nil, // unsure about this
		"break":    nil, // uncertain about this
	}

	var def *FunctionDefinition

	builtinFunctionDefinition := make(map[string]*FunctionDefinition)

	for key, val := range dict {
		def = &FunctionDefinition{}
		def.Name = key
		def.Node = val
		def.FileName = "builtin"

		builtinFunctionDefinition[key] = def
	}

	return builtinFunctionDefinition
}

func NewVariableDefinition(variableName string, node parser.AstNode, fileName string) *VariableDefinition {
	def := &VariableDefinition{}

	def.Name = variableName
	def.FileName = fileName

	def.ImplicitType = make(map[string]types.Type)
	def.Type = TYPE_ANY.Type()
	def.IsValid = true

	if node != nil {
		def.Node = node
		def.Range = node.GetRange()
	}

	return def
}

func DefinitionAnalysis(fileName string, node *parser.GroupStatementNode, outterTemplate map[*parser.GroupStatementNode]*TemplateDefinition) (*FileDefinition, []lexer.Error) {
	if node == nil {
		return nil, nil
	}

	fileInfo, globalVariables, localVariables := NewFileDefinition(fileName, node, outterTemplate)

	_, errs := definitionAnalysisRecursive(node, nil, fileInfo, globalVariables, localVariables)

	return fileInfo, errs
}

func definitionAnalysisRecursive(node parser.AstNode, parent *parser.GroupStatementNode, file *FileDefinition, globalVariables, localVariables map[string]*VariableDefinition) ([2]types.Type, []lexer.Error) {
	if globalVariables == nil || localVariables == nil {
		panic("arguments global/local variable defintion for 'definitionAnalysis()' shouldn't be 'nil'")
	}

	var errs []lexer.Error
	var statementType [2]types.Type

	switch n := node.(type) {
	case *parser.GroupStatementNode:

		// only useful to set 'file.Functions' and 'file.TypeHints[node]'
		if n.ShortCut.CommentGoCode != nil {
			definitionAnalysisComment(n.ShortCut.CommentGoCode, n, file, globalVariables, localVariables)
		}

		statementType, errs = definitionAnalysisGroupStatement(n, parent, file, globalVariables, localVariables)

	case *parser.TemplateStatementNode:
		statementType, errs = definitionAnalysisTemplatateStatement(n, parent, file, globalVariables, localVariables)

	case *parser.CommentNode:
		statementType, errs = definitionAnalysisComment(n, parent, file, globalVariables, localVariables)

	case *parser.VariableDeclarationNode:
		statementType, errs = definitionAnalysisVariableDeclaration(n, parent, file, globalVariables, localVariables)

	case *parser.VariableAssignationNode:
		statementType, errs = definitionAnalysisVariableAssignment(n, parent, file, globalVariables, localVariables)

	case *parser.MultiExpressionNode:
		statementType, errs = definitionAnalysisMultiExpression(n, parent, file, globalVariables, localVariables)

	case *parser.ExpressionNode:
		statementType, errs = definitionAnalysisExpression(n, parent, file, globalVariables, localVariables)

	default:
	}

	return statementType, errs
}

func definitionAnalysisGroupStatement(node *parser.GroupStatementNode, parent *parser.GroupStatementNode, file *FileDefinition, globalVariables, localVariables map[string]*VariableDefinition) ([2]types.Type, []lexer.Error) {
	if globalVariables == nil || localVariables == nil {
		panic("arguments global/local/function/template defintion for 'DefinitionAnalysis()' shouldn't be 'nil' for 'GroupStatementNode'")
	}

	if node.IsRoot() == true && node.Parent() != nil {
		panic("only root node can be flaged as 'root' and with 'parent == nil'")
	}

	// NOTE: However non-root element could have 'v.parent == nil' when an error occurs

	// 1. Variables Init
	scopedGlobalVariables := map[string]*VariableDefinition{}
	// scopedTemplateDefinitionLocal := map[string]parser.AstNode{}

	maps.Copy(scopedGlobalVariables, globalVariables)
	maps.Copy(scopedGlobalVariables, localVariables)
	// maps.Copy(scopedTemplateDefinitionLocal, templateDefinitionsLocal)

	localVariables = map[string]*VariableDefinition{} // 'localVariables' lost reference to the parent 'map', so no need to worry using it

	var errs []lexer.Error
	var localErrs []lexer.Error
	var controlFlowType [2]types.Type

	controlFlowType[0] = TYPE_ANY.Type()
	controlFlowType[1] = TYPE_ERROR.Type()

	// 2. ControlFlow analysis
	switch node.Kind {
	case parser.KIND_IF, parser.KIND_ELSE_IF, parser.KIND_RANGE_LOOP, parser.KIND_WITH,
		parser.KIND_ELSE_WITH, parser.KIND_DEFINE_TEMPLATE, parser.KIND_BLOCK_TEMPLATE:

		if node.ControlFlow == nil {
			log.Printf("fatal, 'controlFlow' not found for 'GroupStatementNode'. \n %s \n", node)
			panic("this 'GroupStatementNode' expect a non-nil 'controlFlow' based on its type ('Kind') " + node.GetKind().String())
		}

		controlFlowType, errs = definitionAnalysisRecursive(node.ControlFlow, node, file, scopedGlobalVariables, localVariables)
	}

	// 3. Variables Scope
	switch node.Kind {
	case parser.KIND_IF, parser.KIND_ELSE, parser.KIND_ELSE_IF, parser.KIND_END:

	case parser.KIND_RANGE_LOOP, parser.KIND_WITH, parser.KIND_ELSE_WITH:
		// TODO: I do believe those variable '$' and '.' should at some point be saved into the 'FileDefinition'

		scopedGlobalVariables["."] = NewVariableDefinition(".", node.ControlFlow, file.Name)
		scopedGlobalVariables["."].Type = controlFlowType[0]

		file.ScopeToVariables[node] = append(file.ScopeToVariables[node], scopedGlobalVariables["."])

	case parser.KIND_DEFINE_TEMPLATE, parser.KIND_BLOCK_TEMPLATE, parser.KIND_GROUP_STATEMENT:

		// TODO: I sould probably add 'parser.KIND_GROUP_STATEMENT' to this use case, it seems appropriate here
		scopedGlobalVariables = make(map[string]*VariableDefinition)
		localVariables = make(map[string]*VariableDefinition)

		scopedGlobalVariables["."] = NewVariableDefinition(".", node, file.Name)
		scopedGlobalVariables["$"] = NewVariableDefinition("$", node, file.Name)

		scopedGlobalVariables["."].Type = TYPE_ANY.Type()
		scopedGlobalVariables["$"].Type = TYPE_ANY.Type()

		scopedGlobalVariables["."].IsUsedOnce = true
		scopedGlobalVariables["$"].IsUsedOnce = true

		// TODO: not sure about this one, perhaps I should use 'map' instead of 'slice' ???????
		file.ScopeToVariables[node] = append(file.ScopeToVariables[node], scopedGlobalVariables["."])
		file.ScopeToVariables[node] = append(file.ScopeToVariables[node], scopedGlobalVariables["$"])

		goCode := node.ShortCut.CommentGoCode
		if goCode != nil {
			typ, _ := definitionAnalysisComment(goCode, node, file, scopedGlobalVariables, localVariables)

			scopedGlobalVariables["."].Type = typ[0]
			scopedGlobalVariables["$"].Type = typ[0]
		}

		if node.GetKind() == parser.KIND_BLOCK_TEMPLATE {
			expressionType := controlFlowType[0].Underlying()
			templateType := scopedGlobalVariables["."].Type.Underlying()

			// TODO: change this to allow 'expressionType' to contains more than 'templateType' field
			// eg. "expressionType = struct { name string, age int }" and "templateType = struct { name string }"
			// should yield 'true' because 'expressionType' contains all element of 'templateType'
			// hint: look at 'types.Implement()', 'types.Satisfies()' and the like first	[those are not useful after all]
			if !types.Identical(expressionType, templateType) {
				err := parser.NewParseError(&lexer.Token{}, errTypeMismatch)
				err.Range = node.ControlFlow.GetRange()

				errs = append(errs, err)
			}

		}

	default:
		panic("found unexpected 'Kind' for 'GroupStatementNode' during 'DefinitionAnalysis()'\n node = " + node.String())
	}

	// 4. Statements analysis
	var statementType [2]types.Type

	for _, statement := range node.Statements {
		if statement == nil {
			panic("statement within 'GroupStatementNode' cannot be nil. make to find where this nil value has been introduced and rectify it")
		}

		// Make DefinitionAnalysis for every children
		statementType, localErrs = definitionAnalysisRecursive(statement, node, file, scopedGlobalVariables, localVariables)
		errs = append(errs, localErrs...)

		// TODO: is this code below really necessary ????
		if statementType[1] == nil || types.Identical(statementType[1], TYPE_ERROR.Type()) {
			continue
		}

		err := parser.NewParseError(&lexer.Token{}, errTypeMismatch)
		err.Range = statement.GetRange()
		errs = append(errs, err)
	}

	// Verify that all 'localVariables' have been used at least once
	for _, def := range localVariables {
		if def.IsUsedOnce {
			continue
		}

		msg := "variable is never used"
		err := parser.NewParseError(&lexer.Token{}, errors.New(msg))
		err.Range = def.Node.GetRange()

		errs = append(errs, err)
	}

	// Implicitly guess the type of var '.' if no type is found (empty or 'any')
	if node.Kind == parser.KIND_DEFINE_TEMPLATE || node.Kind == parser.KIND_BLOCK_TEMPLATE {
		typ := guessVariableTypeFromImplicitType(scopedGlobalVariables["."])
		scopedGlobalVariables["."].Type = typ
	}

	// file.TypeHints[node] = controlFlowType[0]
	file.TypeHints[node] = scopedGlobalVariables["."].Type

	return controlFlowType, errs
}

func definitionAnalysisTemplatateStatement(node *parser.TemplateStatementNode, parent *parser.GroupStatementNode, file *FileDefinition, globalVariables, localVariables map[string]*VariableDefinition) ([2]types.Type, []lexer.Error) {
	if parent == nil {
		panic("template cannot be parentless, it shoud be contain in at least 1 one scope")
	}

	if node.TemplateName == nil {
		panic("the template name should never be empty for a template expression. make sure the template has been parsed correctly.\n" + node.String())
	}

	var errs, localErrs []lexer.Error
	var expressionType [2]types.Type

	// 1. Expression analysis, if any
	if node.Expression != nil {
		expressionType, localErrs = definitionAnalysisRecursive(node.Expression, parent, file, globalVariables, localVariables)
		errs = append(errs, localErrs...)
	}

	invalidTypes := [2]types.Type{
		types.Typ[types.Invalid],
		TYPE_ERROR.Type(),
	}

	// 2. template name analysis
	switch node.Kind {
	case parser.KIND_USE_TEMPLATE:
		templateName := string(node.TemplateName.Value)

		templateDef, found := file.Templates[templateName]
		if !found {
			err := parser.NewParseError(node.TemplateName, errTemplateUndefined)
			errs = append(errs, err)

			return invalidTypes, errs
		}

		if templateDef == nil {
			log.Printf("found nil 'TemplateDefinition' for an existing template.\n file def = %#v\n", file)
			panic("'TemplateDefinition' cannnot be nil for an existing template")
		} else if templateDef.InputType == nil {
			log.Printf("defined template cannot have 'nil' InputType\n def = %#v", templateDef)
			panic("defined template cannot have 'nil' InputType")
		}

		// TODO:	what to do about this condition ? Should I panic ? Or should a normal error returned (what message) ?
		if expressionType[0] == nil {
			err := parser.NewParseError(node.TemplateName, errors.New(""))
			err.Range.Start = node.TemplateName.Range.End
			err.Range.End = node.Range.End
			errs = append(errs, err)

			return invalidTypes, errs
		}

		if !types.Identical(templateDef.InputType.Underlying(), expressionType[0].Underlying()) {
			err := parser.NewParseError(node.TemplateName, errTypeMismatch)
			errs = append(errs, err)

			return invalidTypes, errs
		}

	case parser.KIND_DEFINE_TEMPLATE, parser.KIND_BLOCK_TEMPLATE:
		// WIP
		// TODO: the template definition has already been done in a previous phase
		// "builtinFunctionDefinition()"
		// I am not sure this one is still needed

		if parent.Parent().IsRoot() == false {
			err := parser.NewParseError(node.TemplateName, errors.New("template cannot be defined in local scope"))
			errs = append(errs, err)
		}

		if node.Expression != nil {
			err := parser.NewParseError(&lexer.Token{}, errors.New("template define/block didn't expect an expression"))
			err.Range = node.Expression.GetRange()
			errs = append(errs, err)
		}

		/*
			templateName := string(node.TemplateName.Value)

			// Make sure that the template haven't already be defined in the local scope (root scope)
			// found := templateDefinitionsLocal[templateName]

			_, found := file.Templates[templateName]
			if found {
				err := parser.NewParseError(node.TemplateName, errors.New("template already defined"))
				errs = append(errs, err)
			}

			templateDefinitionsLocal[templateName] = node.Parent()	// Not necessary since this variable is short lived

			def := &TemplateDefinition{}
			def.Name = templateName
			def.Node = node
			def.FileName = file.Name
			def.Range = node.TemplateName.Range
			def.InputType = TYPE_ANY.Type()

			// file.Templates = append(file.Templates, def)
			file.Templates[templateName] = def
		*/

		if node.Parent() == nil {
			log.Printf("fatal, parent not found on template definition. template = \n %s \n", node)
			panic("'TemplateStatementNode' with unexpected empty parent node. " +
				"the template definition and block template must have a parent which will contain all the statements inside the template")
		}

		if node.Parent().Kind != node.Kind {
			panic("value mismatch. 'TemplateStatementNode.Kind' and 'TemplateStatementNode.parent.Kind' must similar")
		}
	default:
		panic("'TemplateStatementNode' do not accept any other type than 'KIND_DEFINE_TEMPLATE, KIND_BLOCK_TEMPLATE, KIND_USE_TEMPLATE'")
	}

	return expressionType, errs
}

// WIP
// TODO: refactor this function, too many ugly code in here
// Only one 'go:code' allowed by files, the other one will be reported as error and discarded in the subsequent computation
// During this phase 'file' argument is modified (file.Functions)
func definitionAnalysisComment(comment *parser.CommentNode, parentScope *parser.GroupStatementNode, file *FileDefinition, globalVariables, localVariables map[string]*VariableDefinition) ([2]types.Type, []lexer.Error) {
	commentType := [2]types.Type{
		TYPE_ANY.Type(),
		TYPE_ERROR.Type(),
	}

	if comment == nil {
		return commentType, nil
		// panic("'CommentNode' cannot be 'nil' while its definition analysis in ongoing")
	}

	if parentScope == nil {
		panic("'CommentNode' cannot be parentless, it shoud be contain in at least 1 one scope")
	}

	if comment.Kind != parser.KIND_COMMENT {
		panic("found value mismatch for 'CommentNode.Kind' during DefinitionAnalysis().\n " + comment.String())
	}

	if comment.GoCode == nil {
		return commentType, nil
	}

	// 1. Find and store all functions and struct definitions
	const virtualFileName = "comment_for_go_template_virtual_file.go"
	const virtualHeader = "package main\n"

	fileSet := token.NewFileSet()
	source := append([]byte(virtualHeader), comment.GoCode.Value...)

	goNode, err := goParser.ParseFile(fileSet, virtualFileName, source, goParser.AllErrors)

	var errsType []types.Error

	config := &types.Config{
		Importer:         importer.Default(),
		IgnoreFuncBodies: true,
		Error: func(err error) {
			errsType = append(errsType, err.(types.Error))
		},
	}

	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}

	pkg, _ := config.Check("", fileSet, []*ast.File{goNode}, info)

	for _, name := range pkg.Scope().Names() {
		obj := pkg.Scope().Lookup(name)

		switch typ := obj.Type().(type) {
		case *types.Signature:
			function := &FunctionDefinition{}
			function.Node = comment
			function.Name = obj.Name()
			function.FileName = file.Name
			function.Type = typ

			startPos := fileSet.Position(obj.Pos())
			endPos := fileSet.Position(obj.Pos())

			// TODO: this one is not good
			endPos.Column += endPos.Offset

			relativeRangeFunction := goAstPositionToRange(startPos, endPos)
			function.Range = remapRangeFromCommentGoCodeToSource(virtualHeader, comment.GoCode.Range, relativeRangeFunction)

			// function.Range.Start.Line -= 2
			// function.Range.End.Line -= 2

			file.Functions[function.Name] = function
			// WIP
			// DEBUG
			log.Printf("go:code fun :: fn = %s ::: range = %s", function.Name, function.Range)

		case *types.Named, *types.Struct:
			if obj.Name() != "Input" {
				continue
			}

		default:
			continue
		}
	}

	// TODO: is this one necessary. I think I do it the same thing above
	/*
		for _, def := range info.Defs {
			log.Println("def = ", def)

			if def == nil {
				continue
			}

			if def.Name() == "Input" {
				structType, ok := def.Type().Underlying().(*types.Struct)

				if !ok {
					continue
				}

				varDef := NewVariableDefinition(def.Name(), comment, file.Name)
				varDef.Type = structType

				commentType[0] = structType

				file.ScopeToVariables[parentScope] = append(file.ScopeToVariables[parentScope], varDef)

				continue
			}

			log.Println("reflect def type = ", reflect.TypeOf(def.Type()))

			l, ok := def.Type().(*types.Signature)

			if !ok {
				continue
			}

			log.Println("isMethod = ", l.Recv())
			if l.Recv() != nil {
				continue
			}

			function := &FunctionDefinition{}
			function.Node = comment
			function.Name = def.Name()
			function.FileName = file.Name
			function.Type = l

			// posn := fileSet.Position(def.Pos())
			// function.Range =

			startPos := fileSet.Position(def.Pos())
			endPos := fileSet.Position(def.Pos())
			endPos.Column += endPos.Offset

			distance := goAstPositionToRange(startPos, endPos)
			function.Range = remapRangeFromCommentGoCodeToSource(virtualHeader, comment.GoCode.Range, distance)

			file.Functions[function.Name] = function

			log.Printf("go:code fun alt :: fn = %s ::: range = %s", function.Name, function.Range)
		}

	*/

	errs := convertThirdPartiesParseErrorToLocalError(err, errsType, file, comment, virtualHeader)

	return commentType, errs
}

func definitionAnalysisVariableDeclaration(node *parser.VariableDeclarationNode, parentScope *parser.GroupStatementNode, file *FileDefinition, globalVariables, localVariables map[string]*VariableDefinition) ([2]types.Type, []lexer.Error) {
	if parentScope == nil {
		panic("'variable declaration' cannot be parentless, it shoud be contain in at least 1 one scope")
	}

	if node.Kind != parser.KIND_VARIABLE_DECLARATION {
		panic("found value mismatch for 'VariableDeclarationNode.Kind' during DefinitionAnalysis()")
	}

	if localVariables == nil {
		panic("'localVariables' shouldn't be nil for 'VariableDeclarationNode.DefinitionAnalysis()'")
	}

	var errs []lexer.Error
	var expressionType [2]types.Type

	invalidTypes := [2]types.Type{
		types.Typ[types.Invalid],
		TYPE_ERROR.Type(),
	}

	// TODO: disally reassignment of '$' and '.' ---> I think it is already implicitly done though

	// 0. Check that 'expression' is valid
	if node.Value != nil {
		var localErrs []lexer.Error
		expressionType, localErrs = definitionAnalysisMultiExpression(node.Value, parentScope, file, globalVariables, localVariables)

		errs = append(errs, localErrs...)
	} else {
		localErr := parser.NewParseError(&lexer.Token{}, errors.New("assignment expression cannot be empty"))
		localErr.Range = node.Range
		errs = append(errs, localErr)
	}

	// 1. Check at least var is declared
	variableSize := len(node.VariableNames)
	if variableSize == 0 {
		errLocal := parser.ParseError{Err: errors.New("variable name is empty for the declaration"), Range: node.Range}
		errs = append(errs, errLocal)

		return invalidTypes, errs
	} else if variableSize > 2 {
		err := parser.NewParseError(node.VariableNames[0],
			errors.New("can't have more than 2 variables declaration on same statement"))

		err.Range.End = node.VariableNames[variableSize-1].Range.End
		errs = append(errs, err)

		return invalidTypes, errs
	}

	// 2. Check existance of variable and process without error if 'var' is unique
	for count, variable := range node.VariableNames {
		if bytes.ContainsAny(variable.Value, ".") {
			err := parser.NewParseError(variable,
				errors.New("variable name cannot include the special character '.' while declaring"))

			errs = append(errs, err)
			continue
		}

		// Check if this variable name exist in the current scope. No need to check for function since 'var' always start with $
		key := string(variable.Value)

		if _, found := localVariables[key]; found {
			err := parser.NewParseError(variable, errVariableRedeclaration)
			errs = append(errs, err)

			continue
		}

		// 2. Insert definition into dictionary, since there is no error whether the variable is already declared or not
		def := NewVariableDefinition(key, node, file.Name)

		// def := &VariableDefinition{}
		// def.Node = node
		// def.Name = key

		// TODO: bring back enumeration to this file, other file is deprecated

		// TODO: Double variable declaration only work with "range" keyword,
		// so later take it into consideration
		def.Type = expressionType[count] // TODO: this code is so wrong

		// def.FileName = file.Name
		// def.IsValid = true

		def.Range = variable.Range
		def.Range.End = parentScope.Range.End

		file.ScopeToVariables[parentScope] = append(file.ScopeToVariables[parentScope], def)

		localVariables[key] = def
	}

	return expressionType, errs
}

func definitionAnalysisVariableAssignment(node *parser.VariableAssignationNode, parent *parser.GroupStatementNode, file *FileDefinition, globalVariables, localVariables map[string]*VariableDefinition) ([2]types.Type, []lexer.Error) {
	if node.Kind != parser.KIND_VARIABLE_ASSIGNMENT {
		panic("found value mismatch for 'VariableAssignationNode.Kind' during DefinitionAnalysis()\n" + node.String())
	}

	if globalVariables == nil || localVariables == nil {
		panic("'localVariables' or 'globalVariables' shouldn't be empty for 'VariableAssignationNode.DefinitionAnalysis()'")
	}

	var errs []lexer.Error
	var assignmentType [2]types.Type

	invalidTypes := [2]types.Type{
		types.Typ[types.Invalid],
		TYPE_ERROR.Type(),
	}

	// TODO: disallow assignment to var '$' and '.'

	// 0. Check that 'expression' is valid
	if node.Value == nil {
		errLocal := parser.NewParseError(nil, errors.New("assignment value cannot be empty"))
		errLocal.Range = node.Range
		errs = append(errs, errLocal)

		return invalidTypes, errs
	}

	expressionType, localErrs := definitionAnalysisMultiExpression(node.Value, parent, file, globalVariables, localVariables)
	errs = append(errs, localErrs...)

	// 1. Check at least var is declared
	if node.VariableName == nil {
		err := fmt.Errorf("%w. syntax should be 'variable = value'", errEmptyVariableName)
		errLocal := parser.ParseError{Err: err, Range: node.Range}
		errLocal.Range = node.Range

		errs = append(errs, errLocal)
		return expressionType, errs
	}

	if bytes.ContainsAny(node.VariableName.Value, ".") {
		err := parser.NewParseError(node.VariableName,
			errors.New("variable name cannot contains any special character such '.' while assigning"))

		errs = append(errs, err)
		return expressionType, errs
	}

	// 2. Check if variable is defined, if not report error
	name := string(node.VariableName.Value)
	defLocal, isLocal := localVariables[name]
	defGlobal, isGlobal := globalVariables[name]

	var def *VariableDefinition

	if isLocal {
		def = defLocal
	} else if isGlobal {
		def = defGlobal
	} else {
		err := parser.NewParseError(node.VariableName, errVariableUndefined)
		errs = append(errs, err)

		return invalidTypes, errs
	}

	if !types.Identical(def.Type, expressionType[0]) {
		errMsg := fmt.Errorf("%w, between var '%s' and expr '%s'", errTypeMismatch, def.Type, expressionType[0])
		err := parser.NewParseError(node.VariableName, errMsg)
		errs = append(errs, err)

		return invalidTypes, errs
	}

	assignmentType[0] = def.Type
	assignmentType[1] = expressionType[1]

	return assignmentType, errs
}

func definitionAnalysisMultiExpression(node *parser.MultiExpressionNode, parent *parser.GroupStatementNode, file *FileDefinition, globalVariables, localVariables map[string]*VariableDefinition) ([2]types.Type, []lexer.Error) {
	if node.Kind != parser.KIND_MULTI_EXPRESSION {
		panic("found value mismatch for 'MultiExpressionNode.Kind' during DefinitionAnalysis()\n" + node.String())
	}

	var errs, localErrs []lexer.Error
	var expressionType [2]types.Type
	var count int = 0

	for _, expression := range node.Expressions {
		if expression == nil {
			log.Printf("fatal, nil element within expression list for MultiExpressionNode. \n %s \n", node.String())
			panic("element within expression list cannot be 'nil' for MultiExpressionNode. instead of inserting the nil value, omit it")
		}

		// normal processing when this is the first expression
		if count == 0 {
			expressionType, localErrs = definitionAnalysisExpression(expression, parent, file, globalVariables, localVariables)
			errs = append(errs, localErrs...)

			// TODO: replace 'count' variable with a var of bool type (eg. isFirstExpression)
			count++
			continue
		}

		// when piping, you pass the result of the previous expression to the end position of the current expression
		//
		// create a token group and insert it to the end of the expression
		groupName := "$__TMP_GROUP"

		tokenGroup := &lexer.Token{ID: lexer.GROUP, Value: []byte(groupName), Range: expression.Range}
		expression.Symbols = append(expression.Symbols, tokenGroup)

		// TODO: Introduce the concept of reserved variable name
		// In my case, variable that start with "$__" are reserved for parser internal use

		// then insert that token as variable within the file
		def := NewVariableDefinition(groupName, nil, file.Name)
		def.Range = tokenGroup.Range
		def.IsValid = true

		def.Type = expressionType[0]
		// TODO: bring back enumeration to this file, other file is deprecated

		// file.ScopeToVariables[parent] = append(file.ScopeToVariables[parent], def)
		localVariables[def.Name] = def

		expressionType, localErrs = definitionAnalysisExpression(expression, parent, file, globalVariables, localVariables)
		errs = append(errs, localErrs...)

		// once, processing is over, remove the group created from the expression
		size := len(expression.Symbols)
		expression.Symbols = expression.Symbols[:size-1]

		// remove temporary variable definition from file
		// TODO: do as stated above, and do a proper lookup
		// size = len(file.ScopeToVariables[parent])
		// defToRemove := file.ScopeToVariables[parent][size - 1]

		delete(localVariables, def.Name)
		// file.ScopeToVariables[parent] = file.ScopeToVariables[parent][:size - 1] // this is not good, nothing garanty that 'def' will this be at the end. look for it propertly

		// never forget about this one
		count++
	}

	return expressionType, errs
}

func definitionAnalysisExpression(node *parser.ExpressionNode, parent *parser.GroupStatementNode, file *FileDefinition, globalVariables, localVariables map[string]*VariableDefinition) ([2]types.Type, []lexer.Error) {
	if node.Kind != parser.KIND_EXPRESSION {
		panic("found value mismatch for 'ExpressionNode.Kind'. expected 'KIND_EXPRESSION' instead. current node \n" + node.String())
	}

	if globalVariables == nil || localVariables == nil {
		panic("'globalVariables' or 'localVariables' or shouldn't be empty for 'ExpressionNode.DefinitionAnalysis()'")
	}

	// 1. Check FunctionName is legit (var or func)
	var expressionType [2]types.Type = [2]types.Type{types.Typ[types.Invalid], TYPE_ERROR.Type()}
	var errs []lexer.Error

	if len(node.Symbols) == 0 {
		err := parser.NewParseError(nil, errEmptyExpression)
		err.Range = node.Range
		errs = append(errs, err)

		return expressionType, errs
	}

	// ------------
	// New area
	// ------------

	defChecker := NewDefinitionAnalyzer(node.Symbols, file, node.Range)
	expressionType, errs = defChecker.makeSymboleDefinitionAnalysis(localVariables, globalVariables)

	return expressionType, errs
}

// first make definition analysis to find all existing reference
// then make the type analysis
type definitionAnalyzer struct {
	symbols         []*lexer.Token
	index           int
	isEOF           bool
	file            *FileDefinition
	rangeExpression lexer.Range
}

func (a definitionAnalyzer) String() string {
	str := fmt.Sprintf(`{ "symbols": %s, "index": %d, "file": %v, "rangeExpression": %s }`, lexer.PrettyFormater(a.symbols), a.index, a.file, a.rangeExpression.String())
	return str
}

func (a *definitionAnalyzer) peek() *lexer.Token {
	if a.index >= len(a.symbols) {
		panic("index out of bound for 'definitionAnalyzer'; check that you only use the provide method to move between tokens, like 'analyzer.nextToken()'")
	}

	return a.symbols[a.index]
}

func (a *definitionAnalyzer) peekAt(index int) *lexer.Token {
	if index < 0 {
		panic("negative index is not allowed")
	}

	if index >= len(a.symbols) {
		panic("index out of bound for 'definitionAnalyzer'; check that you only the index is comming from property 'analyzer.index'")
	}

	return a.symbols[index]
}

func (a *definitionAnalyzer) nextToken() {
	if a.index+1 >= len(a.symbols) {
		a.isEOF = true
		return
	}

	a.index++
}

func (a definitionAnalyzer) isTokenAvailable() bool {
	if a.isEOF {
		return false
	}

	return a.index < len(a.symbols)
}

func NewDefinitionAnalyzer(symbols []*lexer.Token, file *FileDefinition, rangeExpr lexer.Range) *definitionAnalyzer {
	ana := &definitionAnalyzer{
		symbols:         symbols,
		index:           0,
		file:            file,
		rangeExpression: rangeExpr,
	}

	return ana
}

// fetch all tokens and sort them
func (p *definitionAnalyzer) makeSymboleDefinitionAnalysis(localVariables, globalVariables map[string]*VariableDefinition) ([2]types.Type, []lexer.Error) {
	var errs []lexer.Error
	var err *parser.ParseError

	listOfUsedFunctions := map[string]*FunctionDefinition{}
	processedToken := []*lexer.Token{}
	processedTypes := []types.Type{}
	// usedVariables := map[string]bool{}

	var symbol *lexer.Token
	var symbolType types.Type

	// startIndex := p.index

	count := 0
	for p.isTokenAvailable() {
		if count > 100 {
			log.Printf("too many loop.\n analyzer = %s\n", p)
			panic("loop lasted more than expected on 'expression definition analysis'")
		}

		count++
		symbol = p.peek()

		switch symbol.ID {
		case lexer.STRING:
			processedToken = append(processedToken, symbol)
			processedTypes = append(processedTypes, types.Typ[types.String])

			p.nextToken()
		case lexer.NUMBER:
			// TODO: what about floot ??
			processedToken = append(processedToken, symbol)
			processedTypes = append(processedTypes, types.Typ[types.Int])

			p.nextToken()
		case lexer.FUNCTION:

			functionName := string(symbol.Value)
			def := p.file.Functions[functionName]

			if def == nil {
				err = parser.NewParseError(symbol, errFunctionUndefined)
				errs = append(errs, err)
			} else {
				symbolType = def.Type
				listOfUsedFunctions[functionName] = def
			}

			processedToken = append(processedToken, symbol)
			processedTypes = append(processedTypes, symbolType)

			p.nextToken()
		case lexer.DOLLAR_VARIABLE, lexer.GROUP:
			// TODO: check 'DOLLAR_VARIABLE' for 'method' when type system will be better established
			// do the var/method exist ?
			//
			// is it defined ?? (can be a variable, or a method)

			// symbolType := TYPE_INVALID

			varDef := getVariableDefinitionForRootField(symbol, localVariables, globalVariables)
			symbolType, err = getTypeOfDollarVariableWithinFile(symbol, varDef)

			markVariableAsUsed(varDef)

			// symbolType, err = getTypeOfDollarVariableWithinFile(symbol, localVariables, globalVariables)
			// markVariableAsUsed(symbol, localVariables, globalVariables)
			// varDef.IsUsedOnce = true

			if err != nil {
				errs = append(errs, err)
			}

			processedToken = append(processedToken, symbol)
			processedTypes = append(processedTypes, symbolType)

			p.nextToken()
		case lexer.DOT_VARIABLE:
			// TODO: check 'DOT_VARIABLE' when type system will be better established
			// No check for variable definition
			// but check for variable type when type system is ready
			// but if method, make sure the type-check is going

			varDef := getVariableDefinitionForRootField(symbol, localVariables, globalVariables)
			symbolType, err = getTypeOfDollarVariableWithinFile(symbol, varDef)
			if err != nil {
				errs = append(errs, err)
			}

			markVariableAsUsed(varDef)
			setVariableImplicitType(varDef, symbol, symbolType)

			processedToken = append(processedToken, symbol)
			processedTypes = append(processedTypes, symbolType)

			p.nextToken()
		case lexer.LEFT_PAREN:

			// get 'Range' before skipping the token
			startRange := p.peek().Range
			p.nextToken()

			groupType, localErrs := p.makeSymboleDefinitionAnalysis(localVariables, globalVariables)
			errs = append(errs, localErrs...)

			// skip closing token
			symbol = p.peek()
			if symbol.ID != lexer.RIGTH_PAREN {
				err = parser.NewParseError(symbol, errors.New("expected ')'"))
				errs = append(errs, err)
			}

			endRange := p.peek().Range

			p.nextToken()

			// Create a new 'token' and types that represent the returned value, so that analysis continue as if that group was in fact a mere token
			newSymbol := &lexer.Token{ID: lexer.GROUP, Value: []byte("(PARENT_GROUP)"), Range: startRange}
			newSymbol.Range.End = endRange.End

			processedToken = append(processedToken, newSymbol)
			processedTypes = append(processedTypes, groupType[0])

		case lexer.RIGTH_PAREN:
			// do not skip this token now
			//
			// all token have been found for this sub-expression
			// exprRange := p.peek().Range
			// exprRange.Start = p.peekAt(startIndex).Range.Start

			// groupType, localErrs := checkSymbolType(processedToken, processedTypes, exprRange, listOfUsedFunctions)
			/*
				groupType, localErrs := checkSymbolType(processedToken, nil, exprRange, listOfUsedFunctions)
				errs = append(errs, localErrs...)
			*/

			// var groupType [2]types.Type

			/*
				invalidTypes := [2]types.Type{
					types.Typ[types.Invalid],
					TYPE_ERROR.Type(),
				}
			*/

			groupType, inferedTypes, err := makeTypeInference(processedToken, processedTypes)
			if err != nil {
				errs = append(errs, err)
			}

			for sym, inferedTyp := range inferedTypes {
				varDef := getVariableDefinitionForRootField(sym, localVariables, globalVariables)
				setVariableImplicitType(varDef, sym, inferedTyp)
			}

			// token ')' will be skipped by the caller

			// return invalidTypes, errs
			return groupType, errs
		default:
			log.Printf("unexpectd token type. token = %#v\n", symbol.String())
			panic("unexpected token type to parse during 'definition analysis' on Expression. tok type = " + symbol.ID.String())
		}
	}

	groupType, inferedTypes, err := makeTypeInference(processedToken, processedTypes)
	if err != nil {
		errs = append(errs, err)
	}

	for sym, inferedTyp := range inferedTypes {
		varDef := getVariableDefinitionForRootField(sym, localVariables, globalVariables)
		setVariableImplicitType(varDef, sym, inferedTyp)
	}

	// return invalidTypes, errs
	return groupType, errs
}

var errEmptyVariableName error = errors.New("empty variable name")
var errVariableEndWithDot error = errors.New("variable cannot end with '.'")
var errVariableWithConsecutiveDot error = errors.New("consecutive '.' found in variable name")
var errVariableUndefined error = errors.New("variable undefined")
var errVariableRedeclaration error = errors.New("variable redeclaration")
var errFieldNotFound error = errors.New("field not found")
var errMethodNotInEndPosition error = errors.New("method call can only be at the end")
var errMapDontHaveChildren error = errors.New("map cannot have children")
var errChanArrSliceDontHaveChildren = errors.New("channel/array/slice cannot have children")

func splitVariableNameFields(variable *lexer.Token) ([]string, []int, *parser.ParseError) {
	var err *parser.ParseError
	var index, characterCount, counter int
	var fields []string
	var fieldsLocalPosition []int

	// TODO: later on handle the case where the variable is a 'DOT_VARIABLE', meaning it start with '.'
	// which will make the code below to behave unexpectly

	variableName := string(variable.Value)

	if variableName == "" {
		return nil, nil, parser.NewParseError(variable, errEmptyVariableName)
	}

	if variableName[0] == '.' { // Handle the case of a 'DOT_VARIABLE', which start with '.'
		fields = append(fields, ".")
		fieldsLocalPosition = append(fieldsLocalPosition, characterCount)

		characterCount++

		variableName = variableName[1:]

		if variableName == "" {
			return fields, fieldsLocalPosition, nil
		}
	}

	for {
		counter++
		if counter > 1000 {
			log.Printf("current variableName = %s \n fields = %q \n err = %#v \n index = %d",
				variableName, fields, err, index)
			panic("infinite loop detected while spliting variableName")
		}

		index = strings.IndexByte(variableName, '.')

		if index < 0 {
			if variableName == "" {
				err = parser.NewParseError(variable, errors.New("variable cannot end with '.'"))
				err.Range.Start.Character += characterCount
				err.Range.End = err.Range.Start
				err.Range.End.Character += 1

				return fields, fieldsLocalPosition, err
			}

			index = len(variableName)
		} else if index == 0 {
			err = parser.NewParseError(variable, errors.New("consecutive '.' found in variable name"))

			err.Range.Start.Character += characterCount
			err.Range.End = err.Range.Start
			err.Range.End.Character += 1

			return fields, fieldsLocalPosition, err
		}

		fields = append(fields, variableName[:index])
		fieldsLocalPosition = append(fieldsLocalPosition, characterCount)

		characterCount += index + 1

		/*
			if len(variableName) <= index + 1 {
				break
			}
		*/

		if index >= len(variableName) {
			break
		}

		variableName = variableName[index+1:]
	}

	if len(fields) == 0 {
		return nil, nil, parser.NewParseError(variable, errEmptyVariableName)
	}

	return fields, fieldsLocalPosition, nil
}

func joinVariableNameFields(fields []string) (string, *parser.ParseError) {
	length := len(fields)
	if length == 0 {
		err := parser.NewParseError(&lexer.Token{}, errEmptyVariableName)
		return "", err
	}

	// Check for malformated 'field' that will compose the variable name
	for i := 0; i < length; i++ {
		field := fields[i]

		if i == 0 && field == "." {
			continue
		}

		if field == "" {
			err := parser.NewParseError(&lexer.Token{}, errEmptyVariableName)
			return "", err
		} else if field == "." {
			msg := errors.New("only root field can be equal to '.'")
			err := parser.NewParseError(&lexer.Token{}, msg)
			return "", err
		} else if strings.Contains(field, ".") {
			msg := errors.New("field of a variable cannot contain '.'")
			err := parser.NewParseError(&lexer.Token{}, msg)
			return "", err
		}
	}

	suffix := strings.Join(fields[1:], ".")
	if fields[0] == "." {
		return "." + suffix, nil
	}

	return fields[0] + "." + suffix, nil

	// WIP
	// TODO: remove the code below after a few tests
	// Join field to obtain the final variable name
	if length == 1 {
		return fields[0], nil
	}

	if fields[0] == "." {
		suffix := strings.Join(fields[1:], ".")

		return "." + suffix, nil
	}

	variableName := strings.Join(fields, ".")

	return variableName, nil
}

// compute type of variable
func getTypeOfDollarVariableWithinFile(variable *lexer.Token, varDef *VariableDefinition) (types.Type, *parser.ParseError) {
	invalidType := types.Typ[types.Invalid]

	if varDef == nil {
		err := parser.NewParseError(variable, errVariableUndefined)
		return invalidType, err
	}

	// 1. Extract names/fields from variable (separated by '.')
	fields, fieldsLocalPosition, err := splitVariableNameFields(variable)
	if err != nil {
		return invalidType, err
	}

	// TODO: Make sure that 'varDef.Name == fields[0]'

	parentType := varDef.Type
	if parentType == nil {
		return TYPE_ANY.Type(), nil
	} else if types.Identical(parentType, TYPE_ANY.Type()) {
		return TYPE_ANY.Type(), nil
	}

	// 2. Now go down the struct to find out the final variable or method
	// TODO: Should I 'Unalias()' before returning the type ?

	if len(fields) == 0 {
		err := parser.NewParseError(variable, errEmptyVariableName)
		return invalidType, err
	} else if len(fields) == 1 {
		return parentType, nil
	}

	var count int
	var fieldName string
	var fieldPos int
	var fieldSize int = len(fields)

	for i := 1; i < len(fields); i++ {
		count++
		if count > 100 {
			log.Printf("infinite loop detected while analyzing fields.\n fields = %q", fields)
			panic("infinite loop detected while analyzing fields")
		}

		fieldName = fields[i]
		fieldPos = fieldsLocalPosition[i]

		// TODO: range error reporting is not accurate, solve it later on

		// Always check the parent type but always return the last field
		// of the variable without a check
		switch t := parentType.(type) {
		default:
			log.Printf("parentType = %#v \n reflec.TypeOf(parentType) = %s\n"+
				" fields.index = %d ::: fields = %q",
				parentType, reflect.TypeOf(parentType), i, fields,
			)
			panic("field name not recognized")

		case *types.Basic:
			isLastIndex := func(index, size int) bool {
				return index == size-1
			}

			// TODO: remove this check since it will always be true (hint: parent vs child)
			if !isLastIndex(i, fieldSize) {
				err = parser.NewParseError(variable, errTypeMismatch)
				return invalidType, err
			}

			return t, nil

		case *types.Named:
			// a. Check that fieldName match the method name
			var method *types.Func
			var foundMethod bool

			for index := range t.NumMethods() {
				method = t.Method(index)

				if method.Name() != fieldName {
					continue
				}

				foundMethod = true
				break
			}

			if foundMethod {
				parentType = method.Signature()

				continue
			}

			// b. Unpack the underlying type and restart the loop at the current index
			parentType = t.Underlying()

			i--
			continue

		case *types.Struct:
			parentType = nil

			for index := range t.NumFields() {
				field := t.Field(index)

				if field.Name() != fieldName {
					continue
				}

				parentType = field.Type()

				break
			}

			if parentType == nil {
				err = parser.NewParseError(variable, errFieldNotFound)
				err.Range.Start.Character += fieldsLocalPosition[i]
				err.Range.End.Character = err.Range.Start.Character + len(fieldName)

				return invalidType, err
			}

			continue

		case *types.Alias:
			parentType = types.Unalias(t)

			i--
			continue

		case *types.Pointer:
			parentType = t.Elem()

			i--
			continue

		case *types.Interface:
			/*
				for index := range t.NumEmbeddeds() {
					fmt.Println("interface embeded = ", t.EmbeddedType(index))
				}
			*/

			parentType = nil

			for index := range t.NumMethods() {
				field := t.Method(index)

				if field.Name() != fieldName {
					continue
				}

				parentType = field.Type()

				break
			}

			// TODO: handle the case where the type is the empty interface 'interface{}' (known as 'any' type)
			if parentType == nil && t.NumMethods() == 0 {
				return TYPE_ANY.Type(), nil
			}

			if parentType == nil {
				err = parser.NewParseError(variable, errFieldNotFound)
				err.Range.Start.Character += fieldsLocalPosition[i]
				err.Range.End.Character = err.Range.Start.Character + len(fieldName)

				return invalidType, err
			}

			continue

		case *types.Signature:
			err = parser.NewParseError(variable, errMethodNotInEndPosition)
			err.Range.Start.Character += fieldPos
			err.Range.End.Character = err.Range.Start.Character + len(fieldName)

			return invalidType, err

		case *types.Map:
			err = parser.NewParseError(variable, errMapDontHaveChildren)
			err.Range.Start.Character += fieldPos
			err.Range.End.Character = err.Range.Start.Character + len(fieldName)

			return invalidType, err

		case *types.Chan, *types.Array, *types.Slice:
			// those cannot be parent type for go template
			err = parser.NewParseError(variable, errChanArrSliceDontHaveChildren)
			err.Range.Start.Character += fieldPos
			err.Range.End.Character = err.Range.Start.Character + len(fieldName)

			return invalidType, err
			/*
				case *types.Alias:
					fmt.Println("-----> Alias type found = ", t.Underlying())
				case *types.Interface:
					fmt.Println("-----> Interface type found = ", t.Method(0))
			*/

			// array, slice, pointer, channel, map, nil, typeParams, Named, Union, tuple, signature, func
			// alias, interface
		}
	}

	if parentType == nil {
		log.Printf("parent type not found (parentType == nil).\n variable = %s", string(variable.Value))
		panic("parent type not found")
	}

	return parentType, nil
}

func unTuple(typ types.Type) [2]types.Type {
	if typ == nil {
		return [2]types.Type{nil, nil}
	}

	tuple, ok := typ.(*types.Tuple)
	if !ok {
		return [2]types.Type{typ, nil}
	}

	if tuple.Len() == 0 {
		return [2]types.Type{nil, nil}
	} else if tuple.Len() == 1 {
		return [2]types.Type{tuple.At(0).Type(), nil}
	} else if tuple.Len() == 2 {
		return [2]types.Type{tuple.At(0).Type(), tuple.At(1).Type()}
	}

	return [2]types.Type{types.Typ[types.Invalid], types.Typ[types.Invalid]}
}

func makeFunctionTypeInference(funcType *types.Signature, funcSymbol *lexer.Token, argTypes []types.Type, argSymbols []*lexer.Token) (resultType types.Type, inferedTypes map[*lexer.Token]types.Type, err *parser.ParseError) {
	if funcType == nil {
		// TODO: temporary fix, I should delete this code blow and uncomment the one above
		err := parser.NewParseError(funcSymbol, errFunctionUndefined)
		return types.Typ[types.Invalid], nil, err
	}

	// 1. Check Parameter VS Argument validity
	invalidReturnType := types.Typ[types.Invalid]
	inferedTypes = make(map[*lexer.Token]types.Type)

	paramSize := funcType.Params().Len()
	argumentSize := len(argSymbols)

	if paramSize != argumentSize {
		err := parser.NewParseError(funcSymbol, errFunctionParameterSizeMismatch)

		return invalidReturnType, nil, err
	}

	for i := range len(argSymbols) {
		paramType := funcType.Params().At(i).Type()
		argumentType := argTypes[i]

		if argFuncType, ok := argumentType.(*types.Signature); ok {
			retVals, _, err := makeFunctionTypeInference(argFuncType, argSymbols[i], []types.Type{}, []*lexer.Token{})

			if err != nil {
				err.Err = fmt.Errorf("%w, function used as argument cannot have parameter", err.Err)

				return invalidReturnType, nil, err
			}

			argumentType = unTuple(retVals)[0]
		}

		// type inference processing for argument of type 'any'
		if types.Identical(argumentType, TYPE_ANY.Type()) {
			symbol := argSymbols[i]
			symbolType := paramType

			// WIP
			for readOnlyVariable, readOnlyVariableType := range inferedTypes {
				rootVariable := symbol
				fullVariable := readOnlyVariable
				fullVariableType := readOnlyVariableType
				rootVariableType := symbolType

				if len(readOnlyVariable.Value) < len(symbol.Value) {
					rootVariable = readOnlyVariable
					fullVariable = symbol
					fullVariableType = symbolType
					rootVariableType = readOnlyVariableType
				}

				prefixFound := bytes.HasPrefix(fullVariable.Value, rootVariable.Value)
				if !prefixFound {
					continue
				}

				variable := lexer.CloneToken(fullVariable)

				// replace useless root by a single 'fake' root that do not
				fakeFileName := "fake_file_name"
				varName := "fake_root"
				length := len(variable.Value)

				variable.Value = append([]byte(varName), variable.Value[length:]...)

				varDef := NewVariableDefinition(varName, nil, fakeFileName)
				varDef.Type = rootVariableType // new variable type (for the root only)

				typ, err := getTypeOfDollarVariableWithinFile(variable, varDef)
				if err != nil {
					return invalidReturnType, nil, err
				}

				if types.Identical(typ, TYPE_ANY.Type()) {
					continue
				}

				if !types.Identical(typ, fullVariableType) {
					errMsg := fmt.Errorf("%w between expected type '%s' and current type '%s'", errTypeMismatch, typ, fullVariableType)
					err := parser.NewParseError(symbol, errMsg)

					return invalidReturnType, nil, err
				}
			}

			if inferedTypes[symbol] == nil {
				inferedTypes[symbol] = symbolType
			}

			argumentType = inferedTypes[symbol]
		}

		if types.Identical(paramType, TYPE_ANY.Type()) {
			continue
		}

		if !types.Identical(paramType, argumentType) {
			errMsg := fmt.Errorf("%w between param '%s' and arg '%s'", errTypeMismatch, paramType, argumentType)
			err := parser.NewParseError(argSymbols[i], errMsg)

			return invalidReturnType, nil, err
		}
	}

	// 2. Check Validity for Return Type
	returnSize := funcType.Results().Len()
	if returnSize > 2 {
		err := parser.NewParseError(funcSymbol, errFunctionMaxReturn)

		return invalidReturnType, nil, err
	} else if returnSize == 2 {
		secondReturnType := funcType.Results().At(1).Type()
		errorType := TYPE_ERROR.Type()

		if !types.Identical(secondReturnType, errorType) {
			err := parser.NewParseError(funcSymbol, errFunctionSecondReturnNotError)

			return invalidReturnType, nil, err
		}
	} else if returnSize == 0 {
		err := parser.NewParseError(funcSymbol, errFunctionVoidReturn)

		return invalidReturnType, nil, err
	}

	return funcType.Results(), inferedTypes, nil
}

// Return the resulting type of a given expression (symbols)
// TODO: rename func to 'makeExpressionTypeInference'
var errTemplateUndefined error = errors.New("template undefined")
var errEmptyExpression error = errors.New("empty expression")
var errArgumentsOnlyForFunction error = errors.New("only function and method accepts arguments")
var errFunctionUndefined error = errors.New("function undefined")
var errFunctionParameterSizeMismatch error = errors.New("function 'parameter' and 'argument' size mismatch")
var errFunctionMaxReturn error = errors.New("function can't return more than 2 values")
var errFunctionVoidReturn error = errors.New("function can't have 'void' return value")
var errFunctionSecondReturnNotError error = errors.New("function's second return value must be an 'error' only")
var errTypeMismatch error = errors.New("type mismatch")

func makeTypeInference(symbols []*lexer.Token, typs []types.Type) (resultType [2]types.Type, inferedTypes map[*lexer.Token]types.Type, err *parser.ParseError) {
	if len(symbols) != len(typs) {
		log.Printf("every symbol should must have a single type."+
			"\n symbols = %q\n typs = %q", symbols, typs)
		panic("every symbol should must have a single type")
	}

	// 1. len(symbols) == 0 && len == 1
	if len(symbols) == 0 {
		err := parser.NewParseError(nil, errEmptyExpression)

		return unTuple(types.Typ[types.Invalid]), nil, err
	} else if len(symbols) == 1 {
		symbol := symbols[0]
		typ := typs[0]

		funcType, ok := typ.(*types.Signature)
		if ok {
			returnType, _, err := makeFunctionTypeInference(funcType, symbol, typs[1:], symbols[1:])

			return unTuple(returnType), nil, err
		}

		return unTuple(typ), nil, nil
	}

	// 2. len(symbols) >= 2 :: Always true if this section is reached
	funcType, ok := typs[0].(*types.Signature)
	if !ok {
		err := parser.NewParseError(symbols[0], errArgumentsOnlyForFunction)

		return unTuple(types.Typ[types.Invalid]), nil, err
	}

	returnType, inferedTypes, err := makeFunctionTypeInference(funcType, symbols[0], typs[1:], symbols[1:])
	if err != nil {
		return unTuple(returnType), nil, err
	}

	return unTuple(returnType), inferedTypes, nil
}

func getVariableDefinitionForRootField(variable *lexer.Token, localVariables, globalVariables map[string]*VariableDefinition) *VariableDefinition {
	fields, _, _ := splitVariableNameFields(variable)

	if len(fields) == 0 {
		return nil
	}

	// 2. Find whether the root variable exists or not
	variableName := fields[0]

	defLocal, foundLocal := localVariables[variableName]
	defGlobal, foundGlobal := globalVariables[variableName]

	var varDef *VariableDefinition

	if foundLocal {
		varDef = defLocal
	} else if foundGlobal {
		varDef = defGlobal
	} else {
		return nil
	}

	return varDef
}

func markVariableAsUsed(varDef *VariableDefinition) {
	if varDef == nil {
		return
	}

	varDef.IsUsedOnce = true
}

func setVariableImplicitType(varDef *VariableDefinition, symbol *lexer.Token, symbolType types.Type) {
	if symbol == nil {
		log.Printf("cannot set implicit type of not existing symbol.\n varDef = %#v", varDef)
		panic("cannot set implicit type of not existing symbol")
	}

	if varDef == nil {
		return
	}

	if !types.Identical(varDef.Type, TYPE_ANY.Type()) {
		return
	}

	if symbolType == nil {
		symbolType = TYPE_ANY.Type()
	}

	// if 'var' type is 'any', then build the implicit type

	variablePath := string(symbol.Value)
	typeFound := varDef.ImplicitType[variablePath]

	// only insert type for 'variablePath' whenever is
	// either the first time that is 'variablePath is found
	// or when the type found is 'any'
	// in that both case above, we can alter the tye 'type' of 'variablePath'
	if typeFound != nil && !types.Identical(typeFound, TYPE_ANY.Type()) {
		// TODO: return an error, bc user trying to use var with 2 or more different
		// type receiver
		// eg. using '.name' as a 'string' and then later using again '.name' as a 'int'
		// the second usage should report an error since, '.name' has been register as 'string'
		// as provided by inference rule on the first usage of the 'variablePath'
		return
	}

	// TODO: make check for other related variable field
	// look at 'makeTypeInference()' and copy that implementation
	varDef.ImplicitType[variablePath] = symbolType
}

func guessVariableTypeFromImplicitType(varDef *VariableDefinition) types.Type {
	if varDef == nil {
		return types.Typ[types.Invalid]
	}

	if !types.Identical(varDef.Type, TYPE_ANY.Type()) {
		return TYPE_ANY.Type()
	}

	// reach here only if variable of type 'any'
	tree := buildTreeFromImplicitVariableType(varDef.ImplicitType)
	inferedType := buildTypeFromTreeOfType(tree)

	return inferedType
}

type nodeImplicitType struct {
	isRoot    bool
	fieldName string
	fieldType types.Type
	children  map[string]*nodeImplicitType
}

func newNodeImplicitType(fieldName string, fieldType types.Type) *nodeImplicitType {
	if fieldType == nil {
		fieldType = TYPE_ANY.Type()
	}

	node := &nodeImplicitType{
		isRoot:    false,
		fieldName: fieldName,
		fieldType: fieldType,
		children:  make(map[string]*nodeImplicitType),
	}

	return node
}

func buildTreeFromImplicitVariableType(implicitType map[string]types.Type) *nodeImplicitType {
	if len(implicitType) == 0 {
		return nil
	}

	allVariableFields := make([][]string, 0, len(implicitType))
	maxLength := 0
	rootName := ""

	// copy the data into a better DS format for processing
	for propertyChainPath := range implicitType {
		variable := &lexer.Token{Value: []byte(propertyChainPath)}

		fields, _, err := splitVariableNameFields(variable)
		if err != nil {
			log.Printf("variable syntax is incorrect.\n err = %s\n fields = %q",
				err.Err.Error(), implicitType)
			panic("variable syntax is incorrect. " + err.Err.Error())
		}

		length := len(fields)
		if length > maxLength {
			maxLength = length
			rootName = fields[0]
		} else if length == 0 {
			log.Printf("variable length cannot be 'zero'.\n fiels = %q \n implicitType = %#v", fields, implicitType)
			panic("variable length cannot be 'zero'")
		}

		allVariableFields = append(allVariableFields, fields)
	}

	// make sure all fields chain share the same root (variable)
	for _, fields := range allVariableFields {
		variableName := fields[0]

		if variableName != rootName {
			log.Printf("fields can only have a unique root variable name\n implicitType = %#v\n", implicitType)
			panic("fields can only have a unique root variable name")
		}
	}

	// build the root node and insert it into the parent pool
	tree := newNodeImplicitType(rootName, nil)
	tree.isRoot = true

	parentPool := make(map[string]*nodeImplicitType)
	currentPool := make(map[string]*nodeImplicitType)

	parentPool[rootName] = tree

	// Now inspect all fields and build the full tree
	for index := 1; index < maxLength; index++ {

		for _, fields := range allVariableFields {
			if index >= len(fields) {
				continue
			}

			field := fields[index]
			// WIP

			// parentFieldPath := strings.Join(fields[:index], ".")
			parentFieldPath, err := joinVariableNameFields(fields[:index])
			if err != nil {
				log.Printf("error while building implicit type tree ::: \n err = %s\n fields = %q",
					err.Err, fields[:index])
				panic("error while building implicit type tree ::: " + err.GetError())
			}

			parent := parentPool[parentFieldPath]

			if parent == nil {
				log.Printf("unable to find the parent of the current field within the tree"+
					" field = %#v\n fields = %q\n", field, fields)
				panic("unable to find the parent of the current field within the tree")
			}

			if parent.children[field] != nil {
				continue
			}

			child := newNodeImplicitType(field, nil)
			if index == len(fields)-1 { // last element in the slice
				// path := strings.Join(fields, ".")
				path, err := joinVariableNameFields(fields)
				if err != nil {
					log.Printf("error while joining full var in implicit type tree ::: \n err = %s\n fields = %q",
						err.Err, fields)
					panic("error while joining full var in implicit type tree ::: " + err.GetError())
				}

				typ := implicitType[path]

				if typ == nil {
					log.Printf("infered field type cannot be of 'nil' type.\n path = %s\n fields = %q\n implicitType = %v\n",
						path, fields, implicitType)
					panic("infered field type cannot be of 'nil' type")
				}

				child.fieldType = typ
			}

			// currentFieldPath := strings.Join(fields[:index+1], ".")
			currentFieldPath, err := joinVariableNameFields(fields[:index+1])
			if err != nil {
				log.Printf("error while joining full var in implicit type tree ::: \n err = %s\n fields = %q",
					err.Err, fields)
				panic("error while joining full var in implicit type tree ::: " + err.GetError())
			}

			currentPool[currentFieldPath] = child
			parent.children[field] = child
		}

		// Node sure about this !!!
		parentPool = currentPool
		currentPool = make(map[string]*nodeImplicitType)
	}

	return tree
}

func buildTypeFromTreeOfType(tree *nodeImplicitType) types.Type {
	if tree == nil {
		return types.Typ[types.Invalid]
	}

	if len(tree.children) == 0 {
		if tree.fieldType == nil {
			tree.fieldType = TYPE_ANY.Type()
		}

		return tree.fieldType
	}

	var varFields []*types.Var
	for _, node := range tree.children {
		fieldType := buildTypeFromTreeOfType(node)
		field := types.NewVar(token.NoPos, nil, node.fieldName, fieldType)

		varFields = append(varFields, field)
	}

	structType := types.NewStruct(varFields, nil)

	return structType
}

// This function assume every tokens have been processed correctly
// Thus if the first token of an expression have been omitted because it was not define,
// it is not the responsibility of this function to handle it
// In fact, the parent function should have not called this, and handle the problem otherwise
// Best results is when the definition analysis worked out properly
//
// 1. Never call this function if the first token (function or varialbe) of the expression is invalid (definition analysis failed)
// 2. If tokens in the middle failed instead (the definition analysis), send the token over anyway, but with 'invalid type',
// the rest will be handled by this function; the hope to always have an accurate depiction of the length of arguments for function/method
// and handle properly the type mismatch
// 3. Send over the file data or at least definition for all function that will be used (WIP for documentation of this behavior)
//
// In conclusion, this function must always be called with all the token present in the expression, otherwise you will get inconsistant result
// If there is token that are not valid, just alter its type to 'invalid type'
// But as stated earlier, if the first token is rotten, dont even bother calling this function
//

// it removes the added header from the 'position' count
func remapRangeFromCommentGoCodeToSource(header string, boundary, target lexer.Range) lexer.Range {
	maxLineInHeader := 0

	for _, char := range []byte(header) {
		if char == '\n' {
			maxLineInHeader++
		}
	}

	rangeRemaped := lexer.Range{}

	// NOTE: because of go/parser, 'target.Start.Line' always start at '1'
	rangeRemaped.Start.Line = boundary.Start.Line + target.Start.Line - 1 - maxLineInHeader
	rangeRemaped.End.Line = boundary.Start.Line + target.End.Line - 1 - maxLineInHeader

	rangeRemaped.Start.Character = target.Start.Character - 1
	rangeRemaped.End.Character = target.End.Character - 1

	if target.Start.Line-maxLineInHeader == 1 {
		rangeRemaped.Start.Character = boundary.Start.Character + len(header) + target.Start.Character
	}

	if target.End.Line-maxLineInHeader == 1 {
		rangeRemaped.End.Character = boundary.Start.Character + len(header) + target.End.Character
	}

	if rangeRemaped.End.Line > boundary.End.Line {
		msg := "boundary.End.Line = %d ::: rangeRemaped.End.Line = %d\n"
		log.Printf(msg, boundary.End.Line, rangeRemaped.End.Line)

		panic("remaped range cannot excede the comment GoCode boundary")
	}

	return rangeRemaped
}

func goAstPositionToRange(startPos, endPos token.Position) lexer.Range {
	distance := lexer.Range{
		Start: lexer.Position{
			Line:      startPos.Line,
			Character: startPos.Column,
		},
		End: lexer.Position{
			Line:      endPos.Line,
			Character: endPos.Column,
		},
	}

	return distance
}

func NewParseErrorFromErrorList(err *scanner.Error, randomColumnOffset int) *parser.ParseError {
	if err == nil {
		return nil
	}

	if randomColumnOffset < 0 {
		randomColumnOffset = 10
	}

	parseErr := &parser.ParseError{
		Err: errors.New(err.Msg),
		Range: lexer.Range{
			Start: lexer.Position{
				Line:      err.Pos.Line,
				Character: err.Pos.Column,
			},
			End: lexer.Position{
				Line:      err.Pos.Line,
				Character: err.Pos.Column + randomColumnOffset - 1,
			},
		},
	}

	return parseErr
}

func convertThirdPartiesParseErrorToLocalError(parseError error, errsType []types.Error, file *FileDefinition, comment *parser.CommentNode, virtualHeader string) []lexer.Error {
	if file == nil {
		panic("file definition cannot be 'nil' while convert std error to project error")
	}

	if comment == nil {
		panic("comment node cannot be 'nil' while convert std error to project error")
	}

	var errs []lexer.Error

	// 1. convert parse error from go/ast.Error to lexer.Error, and adjust the 'Range'

	if parseError != nil {
		log.Println("comment scanner error found, ", parseError)

		errorList, ok := parseError.(scanner.ErrorList)
		if !ok {
			panic("unexpected error, error obtained by go code parsing did not return expected type ('scanner.ErrorList')")
		}

		const randomColumnOffset int = 7

		for _, errScanner := range errorList {
			// A. Build diagnostic errors
			parseErr := NewParseErrorFromErrorList(errScanner, randomColumnOffset)
			// WIP
			// parseErr.Range = remapRangeFromCommentGoCodeToSource(virtualHeader, comment.Range, parseErr.Range)
			parseErr.Range = remapRangeFromCommentGoCodeToSource(virtualHeader, comment.GoCode.Range, parseErr.Range)
			log.Println("comment scanner error :: ", parseErr)

			errs = append(errs, parseErr)

			// TODO: this for loop must should be removed in the future
			// B. Tag the functions that are erroneous
			for _, function := range file.Functions {
				if function == nil {
					log.Printf("nil function inserted into file definition\n FileDefinition = %#v\n", file)
					panic("nil function inserted into FileDefinition")
				}

				errorLine := parseErr.Range.Start.Line
				errorColumn := parseErr.Range.Start.Character

				if function.Range.Start.Line > errorLine {
					continue
				}

				if function.Range.End.Line < errorLine {
					continue
				}

				if function.Range.Start.Character > errorColumn {
					continue
				}
			}
		}
	}

	// 2. convert type error to lexer.Error

	for _, err := range errsType {
		fset := err.Fset
		pos := fset.Position(err.Pos)

		parseErr := parser.ParseError{
			Err: errors.New(err.Msg),
			Range: lexer.Range{
				Start: lexer.Position{
					Line:      pos.Line,
					Character: pos.Column,
				},
				End: lexer.Position{
					Line:      pos.Line,
					Character: pos.Column + pos.Offset - 1,
				},
			},
		}

		// WIP
		// parseErr.Range = remapRangeFromCommentGoCodeToSource(virtualHeader, comment.Range, parseErr.Range)
		parseErr.Range = remapRangeFromCommentGoCodeToSource(virtualHeader, comment.GoCode.Range, parseErr.Range)

		log.Println("==> 1122")
		log.Printf("%s\n ==> rangeError : %s\n", comment, parseErr.Range)

		errs = append(errs, parseErr)
	}

	return errs
}

//
//
//
// LSP-like helpter functions
//
//
//

// TODO: rename to 'findTargetNodeInfo()',
// TODO: not yet implemented, this code is buggy and not tested
func findLeadWithinAst(root parser.AstNode, position lexer.Position) (node parser.AstNode, name string, isTemplate bool) {
	if root == nil {
		return nil, "", false
	}

	if !root.GetRange().Contains(position) {
		return nil, "", false
	}

	switch r := root.(type) {
	case *parser.GroupStatementNode:
		if r.ControlFlow.GetRange().Contains(position) {

			node, name, isTemplate = findLeadWithinAst(r.ControlFlow, position)
			if _, ok := node.(*parser.GroupStatementNode); !ok {
				node = r
			}

			return node, name, isTemplate
		}

		for _, statement := range r.Statements {
			if !statement.GetRange().Contains(position) {
				continue
			}

			node, name, isTemplate = findLeadWithinAst(statement, position)
			if _, ok := node.(*parser.GroupStatementNode); !ok {
				node = r
			}

			return node, name, isTemplate
		}

		return nil, "", false
	case *parser.TemplateStatementNode:
		if r.Kind == parser.KIND_DEFINE_TEMPLATE || r.Kind == parser.KIND_BLOCK_TEMPLATE {
			return nil, "", false
		}

		if r.TemplateName == nil {
			return nil, "", false
		}

		if r.TemplateName.Range.Contains(position) {
			return r.Parent(), string(r.TemplateName.Value), true
		}

		if r.Expression == nil {
			return nil, "", false
		}

		if r.Expression.GetRange().Contains(position) {
			return findLeadWithinAst(r.Expression, position)
		}

		return nil, "", false

	case *parser.VariableDeclarationNode:
		for _, variable := range r.VariableNames {
			if !variable.Range.Contains(position) {
				continue
			}

			return r, string(variable.Value), false
		}

		if r.Value.GetRange().Contains(position) {
			return findLeadWithinAst(r.Value, position)
		}

		return nil, "", false
	case *parser.VariableAssignationNode:
		if r.VariableName.Range.Contains(position) {
			return r, string(r.VariableName.Value), false
		}

		if r.Value.GetRange().Contains(position) {
			return findLeadWithinAst(r.Value, position)
		}

		return nil, "", false
	case *parser.MultiExpressionNode:
		for _, expression := range r.Expressions {
			if !expression.GetRange().Contains(position) {
				continue
			}

			return findLeadWithinAst(expression, position)
		}

		return nil, "", false
	case *parser.ExpressionNode:
		for _, symbol := range r.Symbols {
			if !symbol.Range.Contains(position) {
				continue
			}

			return r, string(symbol.Value), false
		}

		return nil, "", false
	case *parser.CommentNode:
		return nil, "", false
	default:
		panic("unexpected AstNode type while finding corresponding node with a particular position")
	}
}

func FindAstNodeRelatedToPosition(root *parser.GroupStatementNode, position lexer.Position) (*lexer.Token, parser.AstNode, *parser.GroupStatementNode, bool) {
	seeker := &findAstNodeRelatedToPosition{Position: position}

	log.Println("position before walker: ", position)
	parser.Walk(seeker, root)
	log.Printf("seeker after walker : %#v\n", seeker)

	return seeker.TokenFound, seeker.NodeFound, seeker.LastParent, seeker.IsTemplate
}

type findAstNodeRelatedToPosition struct {
	Position   lexer.Position
	TokenFound *lexer.Token
	LastParent *parser.GroupStatementNode
	NodeFound  parser.AstNode // nodeStatement
	IsTemplate bool
}

func (v *findAstNodeRelatedToPosition) Visit(node parser.AstNode) parser.Visitor {
	// TODO: the token in question can be: 'variable', 'function', 'template name', 'keyword(no-op)', 'garbase(no-op)'

	if node == nil {
		return nil
	}

	// 1. Going down the node tree
	if v.TokenFound != nil {
		return nil
	}

	if !node.GetRange().Contains(v.Position) {
		log.Println("NOPE, POSITION OUT OF RANGE, GO ELSEWHERE. nodeRange = ", node.GetRange())
		return nil
	}

	log.Println("Hourray, POSITION IS IN RANGE. node = ", node)

	switch n := node.(type) {
	case *parser.GroupStatementNode:
		v.LastParent = n
		return v
	case *parser.TemplateStatementNode:
		if n.TemplateName.Range.Contains(v.Position) {
			v.TokenFound = n.TemplateName
			v.NodeFound = n
			v.IsTemplate = true
		}

		v.NodeFound = n

		return v
	case *parser.VariableAssignationNode:
		if n.VariableName.Range.Contains(v.Position) {
			v.TokenFound = n.VariableName
			v.NodeFound = n
			v.IsTemplate = false

			return nil
		}

		v.NodeFound = n

		return v
	case *parser.VariableDeclarationNode:
		for _, variable := range n.VariableNames {
			if variable.Range.Contains(v.Position) {
				v.TokenFound = variable
				v.NodeFound = n
				v.IsTemplate = false

				return nil
			}
		}

		v.NodeFound = n

		return v
	case *parser.MultiExpressionNode:
		for _, expression := range n.Expressions {
			if expression.Range.Contains(v.Position) {
				if v.NodeFound == nil {
					v.NodeFound = n
				}

				return v
			}
		}

		return nil
	case *parser.ExpressionNode:
		for _, symbol := range n.Symbols {
			if symbol.Range.Contains(v.Position) {
				v.TokenFound = symbol
				v.IsTemplate = false

				if v.NodeFound == nil {
					v.NodeFound = n
				}

				return nil
			}
		}

		return nil
	}

	return nil
}

// TODO: Template should be able to return many definition locations in case many templates in the worksapce have a similar name
// However with the way I designed my Data Structure, it is almost impossible to anything beside one result for template
// All in all, I really need a make hover for definition analysis. But this will wait after I have released the first version
// and learend some theory and Data Structure related to 'Type Theory'
func GoToDefinition(from *lexer.Token, parentNodeStatement parser.AstNode, parentScope *parser.GroupStatementNode, file *FileDefinition, isTemplate bool) (fileName string, defFound parser.AstNode, reach lexer.Range) {
	if file == nil {
		log.Printf("File definition not found to compute Go-To Definition. Thus cannot find definition of node."+
			"parentScope = %s\n parentNodeStatement = %s \n from = %s\n", parentScope, parentNodeStatement, from)
		panic("File definition not found to compute Go-To Definition. Thus cannot find definition of node, " + from.String())
	}
	// 1. Try to find the template, if appropriate
	if isTemplate {
		name := string(from.Value)
		// templateFound := file.GetSingleTemplateDefinitionByName(name)
		templateFound := file.Templates[name]

		if templateFound == nil {
			return file.Name, nil, lexer.Range{}
		}

		if templateFound.FileName == "" {
			return file.Name, nil, lexer.Range{}
		}

		return templateFound.FileName, templateFound.Node, templateFound.Range
	}

	name := string(from.Value)

	// 2. Try to find the function
	functionFound := file.Functions[name]
	if functionFound != nil {
		log.Println("---> 7788")
		log.Printf("function -- name = %s :: range = %s\n", functionFound.Name, functionFound.Range.String())

		return functionFound.FileName, functionFound.Node, functionFound.Range
	}

	// 3. Found the multi-scope varialbe
	const MAX_LOOP_REPETITION int = 20
	var count int = 0

	// Bubble up until you find the scope where the variable is defined
	for parentScope != nil {
		count++
		if count > MAX_LOOP_REPETITION {
			panic("possible infinite loop detected while processing 'goToDefinition()'")
		}

		scopedVariables := file.GetScopedVariables(parentScope)

		variableFound, ok := scopedVariables[name]
		if ok {
			return variableFound.FileName, variableFound.Node, variableFound.Range
		}

		parentScope = parentScope.Parent()
	}

	return file.Name, nil, lexer.Range{}
}

/*
func reverseFindDefinitionWithinFile(leave *parser.GroupStatementNode, nameToFind string, isTemplate bool) (foundNode parser.AstNode, reach lexer.Range) {
	panic("not implemented yet")

	if leave == nil {
		return nil, getZeroRangeValue()
	}

	// 0. look if user defined template
	if isTemplate {
		// TODO: there is a function that find all top level 'template' from a node, use it here (but it is at a higher module/gota)
	}

	// 1. look if variable
	for _, varialbe := range leave.Variables {
		if varialbe.Name == nameToFind {
			return varialbe.Node, varialbe.Range
		}
	}

	// 2. look if function (comment)
	// ....

	return reverseFindDefinitionWithinFile(leave.Parent(), nameToFind, isTemplate)
}
*/

func goToDefinitionForFileNodeOnly(position lexer.Range) (node parser.AstNode, reach lexer.Range) {
	panic("not implemented yet")
}

// WIP
// This function is here to replace 'types.Identical()' because if the receiver var is 'any' type and new var is 'int'
// there should be no issue since 'any' type include 'int' type. However, with 'types.Identical()' that's not the case
func TypeMatch(receiver, target types.Type) bool {
	panic("not implemented yet")
}
