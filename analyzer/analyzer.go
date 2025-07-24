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
	TYPE_ANY                                   = types.Universe.Lookup("any")
	TYPE_ERROR                                 = types.Universe.Lookup("error")
	TEMPLATE_MANAGER *WorkspaceTemplateManager = nil
)

func init() {
	if TYPE_ANY == nil {
		panic("initialization of 'any' type failed")
	}

	if TYPE_ERROR == nil {
		panic("initialization of 'error' type failed")
	}

	TEMPLATE_MANAGER = NewWorkspaceTemplateManager()
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
type NodeDefinition interface {
	Name() string
	FileName() string
	Node() parser.AstNode
	Range() lexer.Range
	Type() types.Type
	TypeString() string
}

type FunctionDefinition struct {
	node parser.AstNode
	// Range    lexer.Range
	rng      lexer.Range
	fileName string

	// New comer to keep
	name string
	typ  *types.Signature
}

func (f FunctionDefinition) Name() string {
	return f.name
}

func (f FunctionDefinition) FileName() string {
	return f.fileName
}

func (f FunctionDefinition) Type() types.Type {
	return f.typ
}

func (f FunctionDefinition) Node() parser.AstNode {
	return f.node
}

func (f FunctionDefinition) Range() lexer.Range {
	return f.rng
}

func (f FunctionDefinition) TypeString() string {
	buf := new(bytes.Buffer)
	types.WriteSignature(buf, f.typ, nil)

	str := "func " + f.name + " " + buf.String()

	return str
}

type VariableDefinition struct {
	node     parser.AstNode // direct node containing info about this variable
	rng      lexer.Range    // variable lifetime
	fileName string
	name     string
	typ      types.Type

	ImplicitType   map[string]types.Type // Guessed tree type through 'var' usage
	IsValid        bool
	IsUsedOnce     bool // Only useful to detect whether or not a variable have never been used in the program
	UsageFrequency int
}

func (v VariableDefinition) Name() string {
	return v.name
}

func (v VariableDefinition) FileName() string {
	return v.fileName
}

func (v VariableDefinition) Type() types.Type {
	return v.typ
}

func (v VariableDefinition) Node() parser.AstNode {
	return v.node
}

func (v VariableDefinition) Range() lexer.Range {
	return v.rng
}

func (v VariableDefinition) TypeString() string {
	// WIP
	// 2 use cases: 'Named' & Other type
	//

	str := v.typ.String()

	switch v.typ.(type) {
	case *types.Named:
		str = v.typ.Underlying().String()

	default:
	}

	str = "var " + v.name + " " + str

	return str
}

type TemplateDefinition struct {
	node      parser.AstNode
	rng       lexer.Range
	fileName  string
	name      string
	inputType types.Type
	isValid   bool
}

func (t TemplateDefinition) Name() string {
	return t.name
}

func (t TemplateDefinition) FileName() string {
	return t.fileName
}

func (t TemplateDefinition) Type() types.Type {
	return t.inputType
}

func (t TemplateDefinition) Node() parser.AstNode {
	return t.node
}

func (t TemplateDefinition) Range() lexer.Range {
	return t.rng
}

// TODO: refactor to take into account method name as well
// type 'named' vs 'basic'
func (t TemplateDefinition) TypeString() string {
	str := "var _ " + t.inputType.Underlying().String()

	return str
}

type FileDefinition struct {
	root                         *parser.GroupStatementNode
	name                         string
	typeHints                    map[*parser.GroupStatementNode]types.Type // ???
	scopeToVariables             map[*parser.GroupStatementNode](map[string]*VariableDefinition)
	functions                    map[string]*FunctionDefinition
	templates                    map[string]*TemplateDefinition
	isTemplateDependencyAnalyzed bool
	// WorkspaceTemplates	map[string]*TemplateDefinition
}

func (f FileDefinition) Name() string {
	return f.name
}

func (f FileDefinition) FileName() string {
	return f.name
}

func (f FileDefinition) Type() types.Type {
	return f.typeHints[f.root]
}

func (f FileDefinition) Node() parser.AstNode {
	return f.root
}

func (f FileDefinition) Range() lexer.Range {
	return f.root.Range()
}

func (f FileDefinition) TypeString() string {
	return f.Type().String()
}

func (f FileDefinition) Root() *parser.GroupStatementNode {
	return f.root
}

func NewFileDefinition(fileName string, root *parser.GroupStatementNode, outterTemplate map[*parser.GroupStatementNode]*TemplateDefinition) (*FileDefinition, map[string]*VariableDefinition, map[string]*VariableDefinition) {
	file := new(FileDefinition)

	file.name = fileName
	file.root = root
	file.isTemplateDependencyAnalyzed = false

	file.typeHints = make(map[*parser.GroupStatementNode]types.Type)
	file.templates = make(map[string]*TemplateDefinition)

	// 2. build external templates available for the current file
	foundMoreThanOnce := make(map[string]bool)

	for templateNode, templateDef := range outterTemplate {
		templateName := templateNode.TemplateName()

		def := file.templates[templateName]

		if def != nil {

			if !foundMoreThanOnce[templateName] {
				defAny := &TemplateDefinition{
					inputType: TYPE_ANY.Type(),
					fileName:  "",
					node:      nil,
				}

				file.templates[templateName] = defAny
			}

			foundMoreThanOnce[templateName] = true
			continue
		}

		file.templates[templateName] = templateDef
		foundMoreThanOnce[templateName] = false
	}

	file.functions = getBuiltinFunctionDefinition()
	file.scopeToVariables = make(map[*parser.GroupStatementNode]map[string]*VariableDefinition)

	globalVariables, localVariables := NewGlobalAndLocalVariableDefinition(nil, fileName)

	return file, globalVariables, localVariables
}

func NewFileDefinitionFromPartialFile(partialFile *FileDefinition, outterTemplate map[*parser.GroupStatementNode]*TemplateDefinition) (*FileDefinition, map[string]*VariableDefinition, map[string]*VariableDefinition) {
	if partialFile == nil {
		log.Printf("got a <nil> partial File\n")
		panic("got a <nil> partial File")
	}

	if partialFile.root == nil {
		log.Printf("partial file without root parse tree found at start definition analysis"+
			"\n fileName = %s\n", partialFile.FileName())
		panic("partial file without root parse tree found at start definition analysis")
	}

	if partialFile.name == "" {
		panic("partial file cannot have empty file name")
	}

	file, globalVariables, localVariables := NewFileDefinition(partialFile.FileName(), partialFile.root, outterTemplate)

	// TODO: should I do a deep clone instead of a shallow one ???
	file.root = partialFile.root
	file.functions = partialFile.functions

	file.scopeToVariables = partialFile.scopeToVariables

	return file, globalVariables, localVariables
}

func NewTemplateDefinition(name string, fileName string, node parser.AstNode, rng lexer.Range, typ types.Type, isValid bool) *TemplateDefinition {
	def := &TemplateDefinition{}
	def.name = name
	def.fileName = fileName
	def.node = node
	def.rng = rng
	def.isValid = isValid
	def.inputType = typ

	return def
}

// TODO: remove this function and use *parser.GroupStatementNode.ShortCut.VariableDeclarations
// But seriously, I am sure sure about it since 'VariableDeclarationNode' is defined in 'parser' package
// but the 'VariableDefinition' is instead defined in 'analyzer'
// Mixing both DS will create a cyclical import issue
func (f FileDefinition) GetScopedVariables(scope *parser.GroupStatementNode) map[string]*VariableDefinition {
	scopedVariables := f.scopeToVariables[scope]
	if scopedVariables == nil {
		scopedVariables = make(map[string]*VariableDefinition)
	}

	return scopedVariables
}

func CloneTemplateDefinition(src *TemplateDefinition) *TemplateDefinition {
	if src == nil {
		return nil
	}

	dst := &TemplateDefinition{
		node:      src.Node(),
		rng:       src.Range(),
		fileName:  src.fileName,
		name:      src.Name(),
		inputType: src.inputType,
		isValid:   src.isValid,
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
// WIP
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
		def.name = key
		def.node = val
		def.fileName = "builtin"

		builtinFunctionDefinition[key] = def
	}

	return builtinFunctionDefinition
}

func NewGlobalAndLocalVariableDefinition(node parser.AstNode, fileName string) (map[string]*VariableDefinition, map[string]*VariableDefinition) {
	globalVariables := make(map[string]*VariableDefinition)
	localVariables := make(map[string]*VariableDefinition)

	localVariables["."] = NewVariableDefinition(".", nil, fileName)
	localVariables["$"] = NewVariableDefinition("$", nil, fileName)

	return globalVariables, localVariables
}

func NewVariableDefinition(variableName string, node parser.AstNode, fileName string) *VariableDefinition {
	def := &VariableDefinition{}

	def.name = variableName
	def.fileName = fileName

	def.ImplicitType = make(map[string]types.Type)
	def.typ = TYPE_ANY.Type()
	def.IsValid = true

	if node != nil {
		def.node = node
		def.rng = node.Range()
	}

	return def
}

func DefinitionAnalysisFromPartialFile(partialFile *FileDefinition, outterTemplate map[*parser.GroupStatementNode]*TemplateDefinition) (*FileDefinition, []lexer.Error) {
	if partialFile == nil {
		log.Printf("expected a partial file but got <nil> for 'DefinitionAnalysisFromPartialFile()'")
		panic("expected a partial file but got <nil> for 'DefinitionAnalysisFromPartialFile()'")
	}

	file, globalVariables, localVariables := NewFileDefinitionFromPartialFile(partialFile, outterTemplate)
	file.isTemplateDependencyAnalyzed = true

	typ, errs := definitionAnalysisRecursive(file.root, nil, file, globalVariables, localVariables)

	_ = typ
	// file.typeHints[file.root] = typ[0]

	return file, errs
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

func definitionAnalysisGroupStatement(node *parser.GroupStatementNode, _ *parser.GroupStatementNode, file *FileDefinition, globalVariables, localVariables map[string]*VariableDefinition) ([2]types.Type, []lexer.Error) {
	if globalVariables == nil || localVariables == nil {
		panic("arguments global/local/function/template defintion for 'DefinitionAnalysis()' shouldn't be 'nil' for 'GroupStatementNode'")
	}

	if node.IsRoot() == true && node.Parent() != nil {
		panic("only root node can be flaged as 'root' and with 'parent == nil'")
	}

	// 1. Variables Init
	scopedGlobalVariables := map[string]*VariableDefinition{}

	maps.Copy(scopedGlobalVariables, globalVariables)
	maps.Copy(scopedGlobalVariables, localVariables)

	localVariables = map[string]*VariableDefinition{} // 'localVariables' lost reference to the parent 'map', so no need to worry using it

	var errs []lexer.Error
	var localErrs []lexer.Error

	var controlFlowType [2]types.Type
	var scopeType [2]types.Type

	controlFlowType[0] = TYPE_ANY.Type()
	controlFlowType[1] = TYPE_ERROR.Type()

	scopeType[0] = TYPE_ANY.Type()
	scopeType[1] = TYPE_ERROR.Type()

	// 2. ControlFlow analysis
	switch node.Kind() {
	case parser.KIND_IF, parser.KIND_ELSE_IF, parser.KIND_RANGE_LOOP, parser.KIND_WITH,
		parser.KIND_ELSE_WITH, parser.KIND_DEFINE_TEMPLATE, parser.KIND_BLOCK_TEMPLATE:

		if node.ControlFlow == nil {
			log.Printf("fatal, 'controlFlow' not found for 'GroupStatementNode'. \n %s \n", node)
			panic("this 'GroupStatementNode' expect a non-nil 'controlFlow' based on its type ('Kind') " + node.Kind().String())
		}

		controlFlowType, errs = definitionAnalysisRecursive(node.ControlFlow, node, file, scopedGlobalVariables, localVariables)
	}

	// 3. Set Variables Scope
	// TODO: Use the new 'node.IsGroupWithNoVariableReset()' and the like
	switch node.Kind() {
	case parser.KIND_IF, parser.KIND_ELSE, parser.KIND_ELSE_IF, parser.KIND_END:

	case parser.KIND_RANGE_LOOP, parser.KIND_WITH, parser.KIND_ELSE_WITH:

		localVariables["."] = NewVariableDefinition(".", node.ControlFlow, file.Name())

		localVariables["."].typ = controlFlowType[0]

	case parser.KIND_DEFINE_TEMPLATE, parser.KIND_BLOCK_TEMPLATE, parser.KIND_GROUP_STATEMENT:

		scopedGlobalVariables = make(map[string]*VariableDefinition)
		localVariables = make(map[string]*VariableDefinition)

		localVariables["."] = NewVariableDefinition(".", node, file.name)
		localVariables["$"] = NewVariableDefinition("$", node, file.name)

		localVariables["."].IsUsedOnce = true
		localVariables["$"].IsUsedOnce = true

		commentGoCode := node.ShortCut.CommentGoCode
		if commentGoCode != nil {
			_, localErrs := definitionAnalysisComment(commentGoCode, node, file, scopedGlobalVariables, localVariables)

			localVariables["$"].rng = localVariables["."].Range()
			localVariables["$"].typ = localVariables["."].Type()

			errs = append(errs, localErrs...)
		}

		if node.Kind() == parser.KIND_BLOCK_TEMPLATE {
			expressionType := controlFlowType[0].Underlying()
			templateType := localVariables["."].Type().Underlying()

			// TODO: change this to allow 'expressionType' to contains more than 'templateType' field
			// eg. "expressionType = struct { name string, age int }" and "templateType = struct { name string }"
			// should yield 'true' because 'expressionType' contains all element of 'templateType'
			// hint: look at 'types.Implement()', 'types.Satisfies()' and the like first	[those are not useful after all]
			if !types.Identical(expressionType, templateType) {
				err := parser.NewParseError(&lexer.Token{}, errTypeMismatch)
				err.Range = node.ControlFlow.Range()

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

		// skip already analyzed 'goCode' (done above)
		if statement == node.ShortCut.CommentGoCode {
			continue
		}

		// skip template scope analysis if already done before during template dependencies analysis
		scope, isScope := statement.(*parser.GroupStatementNode)
		if isScope && scope.IsTemplate() && file.isTemplateDependencyAnalyzed {
			continue
		}

		// Make DefinitionAnalysis for every children
		statementType, localErrs = definitionAnalysisRecursive(statement, node, file, scopedGlobalVariables, localVariables)
		errs = append(errs, localErrs...)

		// TODO: is this code below really necessary ????
		if statementType[1] == nil || types.Identical(statementType[1], TYPE_ERROR.Type()) {
			continue
		}

		err := parser.NewParseError(&lexer.Token{}, errTypeMismatch)
		err.Range = statement.Range()
		errs = append(errs, err)
	}

	// Verify that all 'localVariables' have been used at least once
	for _, def := range localVariables {
		if def.IsUsedOnce {
			continue
		}

		msg := "variable is never used"
		err := parser.NewParseError(&lexer.Token{}, errors.New(msg))
		err.Range = def.Node().Range()

		errs = append(errs, err)
	}

	// TODO: the idea is correct, but the way I am going about it is dubious at best
	// Implicitly guess the type of var '.' if no type is found (empty or 'any')
	// WARNING: only apply this if the current '.' and '$' are of type 'any'
	// if node.Kind() == parser.KIND_DEFINE_TEMPLATE || node.Kind() == parser.KIND_BLOCK_TEMPLATE {
	if node.IsGroupWithDollarDotVariableReset() {
		// What about '$'. It is nice and all to guess for '.', but what about '$' variable ?
		typ := guessVariableTypeFromImplicitType(localVariables["."])
		localVariables["."].typ = typ
		localVariables["$"].typ = typ
	}

	// set type of the scope
	/*
		switch node.Kind() {
		case parser.KIND_IF, parser.KIND_ELSE, parser.KIND_ELSE_IF, parser.KIND_END:
		default:
			file.typeHints[node] = localVariables["."].typ
		}
	*/

	if node.IsGroupWithDotVariableReset() || node.IsGroupWithDollarDotVariableReset() {
		file.typeHints[node] = localVariables["."].typ
	}

	// WIP
	if node.IsTemplate() {
		// name := node
		// file.templates[name].inputType = localVariables["."].typ
		file.typeHints[node] = localVariables["."].typ
	}

	if localVariables["."] != nil {
		scopeType[0] = localVariables["."].typ
	}

	// save all local variables for the current scope; and set the type hint
	file.scopeToVariables[node] = localVariables

	return scopeType, errs
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
	switch node.Kind() {
	case parser.KIND_USE_TEMPLATE:
		templateName := string(node.TemplateName.Value)

		templateDef, found := file.templates[templateName]
		if !found {
			err := parser.NewParseError(node.TemplateName, errTemplateUndefined)
			errs = append(errs, err)

			return invalidTypes, errs
		}

		if templateDef == nil {
			log.Printf("found nil 'TemplateDefinition' for an existing template.\n file def = %#v\n", file)
			panic("'TemplateDefinition' cannnot be nil for an existing template")
		} else if templateDef.inputType == nil {
			log.Printf("defined template cannot have 'nil' InputType\n def = %#v", templateDef)
			panic("defined template cannot have 'nil' InputType")
		}

		// TODO:	what to do about this condition ? Should I panic ? Or should a normal error returned (what message) ?
		if expressionType[0] == nil {
			err := parser.NewParseError(node.TemplateName, errors.New(""))
			err.Range.Start = node.TemplateName.Range.End
			err.Range.End = node.Range().End
			errs = append(errs, err)

			return invalidTypes, errs
		}

		if !types.Identical(templateDef.inputType.Underlying(), expressionType[0].Underlying()) {
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
			err.Range = node.Expression.Range()
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

		if node.Parent().Kind() != node.Kind() {
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
func definitionAnalysisComment(comment *parser.CommentNode, parentScope *parser.GroupStatementNode, file *FileDefinition, _, localVariables map[string]*VariableDefinition) ([2]types.Type, []lexer.Error) {
	commentType := [2]types.Type{
		TYPE_ANY.Type(),
		TYPE_ERROR.Type(),
	}

	if comment == nil {
		return commentType, nil
	}

	if parentScope == nil {
		panic("'CommentNode' cannot be parentless, it shoud be contain in at least 1 one scope")
	}

	if comment.Kind() != parser.KIND_COMMENT {
		panic("found value mismatch for 'CommentNode.Kind' during DefinitionAnalysis().\n " + comment.String())
	}

	if comment.GoCode == nil {
		return commentType, nil
	}

	// Do not analyze orphaned 'GoCode'
	// Correct 'GoCode' is available in parent scope
	if comment != parentScope.ShortCut.CommentGoCode {
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
	var errComments []lexer.Error

	for _, name := range pkg.Scope().Names() {
		obj := pkg.Scope().Lookup(name)

		switch typ := obj.Type().(type) {
		case *types.Signature:
			function := &FunctionDefinition{}
			function.node = comment
			function.name = obj.Name()
			function.fileName = file.Name()
			function.typ = typ

			startPos := fileSet.Position(obj.Pos())
			endPos := fileSet.Position(obj.Pos())
			endPos.Column += endPos.Offset

			relativeRangeFunction := goAstPositionToRange(startPos, endPos)
			function.rng = remapRangeFromCommentGoCodeToSource(virtualHeader, comment.GoCode.Range, relativeRangeFunction)

			if !parentScope.IsRoot() {
				err := parser.NewParseError(comment.GoCode, errors.New("function cannot be declared outside root scope"))
				err.Range = function.Range()
				errComments = append(errComments, err)

				continue
			}

			file.functions[function.Name()] = function

			// DEBUG
			log.Printf("go:code fun :: fn = %s ::: range = %s", function.Name(), function.Range())

		case *types.Named, *types.Struct:
			/*
				log.Println("2255 named")
				log.Println("obj found = ", obj, " ---> typ found = ", typ.Underlying())

				named := typ.(*types.Named)
				log.Println("named method count = ", named.NumMethods())
				for i := 0; i < named.NumMethods(); i++ {
					log.Println("------> method ", i, " = ", named.Method(i))
					log.Println("------> method ", i, " = ", named.Method(i).Signature())
					log.Println("------> method ", i, " = ", named.Method(i).Type().Underlying())
				}
			*/

			if obj.Name() != "Input" {
				continue
			}

			if localVariables["."] == nil {
				continue
			}

			commentType[0] = typ

			startPos := fileSet.Position(obj.Pos())
			endPos := fileSet.Position(obj.Pos())
			endPos.Column += endPos.Offset

			relativeRangeFunction := goAstPositionToRange(startPos, endPos)

			localVariables["."].rng = remapRangeFromCommentGoCodeToSource(virtualHeader, comment.GoCode.Range, relativeRangeFunction)
			localVariables["."].typ = typ

			if localVariables["$"] == nil {
				continue
			}

			localVariables["$"].rng = localVariables["."].rng
			localVariables["$"].typ = localVariables["."].typ

		default:
			continue
		}
	}

	errs := convertThirdPartiesParseErrorToLocalError(err, errsType, file, comment, virtualHeader)
	errs = append(errs, errComments...)

	return commentType, errs
}

func definitionAnalysisVariableDeclaration(node *parser.VariableDeclarationNode, parentScope *parser.GroupStatementNode, file *FileDefinition, globalVariables, localVariables map[string]*VariableDefinition) ([2]types.Type, []lexer.Error) {
	if parentScope == nil {
		panic("'variable declaration' cannot be parentless, it shoud be contain in at least 1 one scope")
	}

	if node.Kind() != parser.KIND_VARIABLE_DECLARATION {
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
		localErr.Range = node.Range()
		errs = append(errs, localErr)
	}

	// 1. Check at least var is declared
	variableSize := len(node.VariableNames)
	if variableSize == 0 {
		errLocal := parser.ParseError{Err: errors.New("variable name is empty for the declaration"), Range: node.Range()}
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
		def := NewVariableDefinition(key, node, file.Name())

		// def := &VariableDefinition{}
		// def.Node = node
		// def.Name = key

		// TODO: bring back enumeration to this file, other file is deprecated

		// TODO: Double variable declaration only work with "range" keyword,
		// so later take it into consideration
		def.typ = expressionType[count] // TODO: this code is so wrong

		// def.FileName = file.Name
		// def.IsValid = true

		def.rng.Start = variable.Range.Start
		// def.rng.End = parentScope.Range().End

		// file.scopeToVariables[parentScope] = append(file.scopeToVariables[parentScope], def)

		localVariables[key] = def
	}

	return expressionType, errs
}

func definitionAnalysisVariableAssignment(node *parser.VariableAssignationNode, parent *parser.GroupStatementNode, file *FileDefinition, globalVariables, localVariables map[string]*VariableDefinition) ([2]types.Type, []lexer.Error) {
	if node.Kind() != parser.KIND_VARIABLE_ASSIGNMENT {
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
		errLocal.Range = node.Range()
		errs = append(errs, errLocal)

		return invalidTypes, errs
	}

	expressionType, localErrs := definitionAnalysisMultiExpression(node.Value, parent, file, globalVariables, localVariables)
	errs = append(errs, localErrs...)

	// 1. Check at least var is declared
	if node.VariableName == nil {
		err := fmt.Errorf("%w. syntax should be 'variable = value'", errEmptyVariableName)
		errLocal := parser.ParseError{Err: err, Range: node.Range()}
		errLocal.Range = node.Range()

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

	if !types.Identical(def.typ, expressionType[0]) {
		errMsg := fmt.Errorf("%w, between var '%s' and expr '%s'", errTypeMismatch, def.Type(), expressionType[0])
		err := parser.NewParseError(node.VariableName, errMsg)
		errs = append(errs, err)

		return invalidTypes, errs
	}

	assignmentType[0] = def.typ
	assignmentType[1] = expressionType[1]

	return assignmentType, errs
}

func definitionAnalysisMultiExpression(node *parser.MultiExpressionNode, parent *parser.GroupStatementNode, file *FileDefinition, globalVariables, localVariables map[string]*VariableDefinition) ([2]types.Type, []lexer.Error) {
	if node.Kind() != parser.KIND_MULTI_EXPRESSION {
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

		tokenGroup := &lexer.Token{ID: lexer.GROUP, Value: []byte(groupName), Range: expression.Range()}
		expression.Symbols = append(expression.Symbols, tokenGroup)

		// TODO: Introduce the concept of reserved variable name
		// In my case, variable that start with "$__" are reserved for parser internal use

		// then insert that token as variable within the file
		def := NewVariableDefinition(groupName, nil, file.Name())
		def.rng = tokenGroup.Range
		def.IsValid = true

		def.typ = expressionType[0]
		// TODO: bring back enumeration to this file, other file is deprecated

		// file.ScopeToVariables[parent] = append(file.ScopeToVariables[parent], def)
		localVariables[def.Name()] = def

		expressionType, localErrs = definitionAnalysisExpression(expression, parent, file, globalVariables, localVariables)
		errs = append(errs, localErrs...)

		// once, processing is over, remove the group created from the expression
		size := len(expression.Symbols)
		expression.Symbols = expression.Symbols[:size-1]

		// remove temporary variable definition from file
		// TODO: do as stated above, and do a proper lookup
		// size = len(file.ScopeToVariables[parent])
		// defToRemove := file.ScopeToVariables[parent][size - 1]

		delete(localVariables, def.Name())
		// file.ScopeToVariables[parent] = file.ScopeToVariables[parent][:size - 1] // this is not good, nothing garanty that 'def' will this be at the end. look for it propertly

		// never forget about this one
		count++
	}

	return expressionType, errs
}

func definitionAnalysisExpression(node *parser.ExpressionNode, _ *parser.GroupStatementNode, file *FileDefinition, globalVariables, localVariables map[string]*VariableDefinition) ([2]types.Type, []lexer.Error) {
	if node.Kind() != parser.KIND_EXPRESSION {
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
		err.Range = node.Range()
		errs = append(errs, err)

		return expressionType, errs
	}

	// ------------
	// New area
	// ------------

	defChecker := NewDefinitionAnalyzer(node.Symbols, file, node.Range())
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
			def := p.file.functions[functionName]

			if def == nil {
				err = parser.NewParseError(symbol, errFunctionUndefined)
				errs = append(errs, err)
			} else {
				symbolType = def.typ
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

	parentType := varDef.typ
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
				varDef.typ = rootVariableType // new variable type (for the root only)

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
			errMsg := fmt.Errorf("%w, expected '%s' but got '%s'", errTypeMismatch, paramType, argumentType)
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

	if !types.Identical(varDef.typ, TYPE_ANY.Type()) {
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

// Will only guess if variable type is 'any', otherwise return the current type of the variable
func guessVariableTypeFromImplicitType(varDef *VariableDefinition) types.Type {
	if varDef == nil {
		return types.Typ[types.Invalid]
	}

	if !types.Identical(varDef.typ, TYPE_ANY.Type()) {
		return varDef.typ
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
		log.Printf("boundary.End.Line = %d ::: rangeRemaped.End.Line = %d\n", boundary.End.Line, rangeRemaped.End.Line)

		rangeRemaped.End.Line = boundary.End.Line
		// panic("remaped range cannot excede the comment GoCode boundary")
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

func NewParseErrorFromErrorType(err types.Error) *parser.ParseError {
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

	return &parseErr
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
			parseErr.Range = remapRangeFromCommentGoCodeToSource(virtualHeader, comment.GoCode.Range, parseErr.Range)

			log.Println("comment scanner error :: ", parseErr)

			errs = append(errs, parseErr)
		}
	}

	// 2. convert type error to lexer.Error

	for _, err := range errsType {
		parseErr := NewParseErrorFromErrorType(err)
		parseErr.Range = remapRangeFromCommentGoCodeToSource(virtualHeader, comment.GoCode.Range, parseErr.Range)

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
// TODO: to remove
func findLeadWithinAst(root parser.AstNode, position lexer.Position) (node parser.AstNode, name string, isTemplate bool) {
	if root == nil {
		return nil, "", false
	}

	if !root.Range().Contains(position) {
		return nil, "", false
	}

	switch r := root.(type) {
	case *parser.GroupStatementNode:
		if r.ControlFlow.Range().Contains(position) {

			node, name, isTemplate = findLeadWithinAst(r.ControlFlow, position)
			if _, ok := node.(*parser.GroupStatementNode); !ok {
				node = r
			}

			return node, name, isTemplate
		}

		for _, statement := range r.Statements {
			if !statement.Range().Contains(position) {
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
		if r.Kind() == parser.KIND_DEFINE_TEMPLATE || r.Kind() == parser.KIND_BLOCK_TEMPLATE {
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

		if r.Expression.Range().Contains(position) {
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

		if r.Value.Range().Contains(position) {
			return findLeadWithinAst(r.Value, position)
		}

		return nil, "", false
	case *parser.VariableAssignationNode:
		if r.VariableName.Range.Contains(position) {
			return r, string(r.VariableName.Value), false
		}

		if r.Value.Range().Contains(position) {
			return findLeadWithinAst(r.Value, position)
		}

		return nil, "", false
	case *parser.MultiExpressionNode:
		for _, expression := range r.Expressions {
			if !expression.Range().Contains(position) {
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

// NodeDefinition interface{} = FileDefinition(?) | VariableDefinition | TemplateDefinition | FunctionDefinition
func FindSourceDefinitionFromPosition(file *FileDefinition, position lexer.Position) []NodeDefinition {
	// 1. Find the node and token corresponding to the provided position
	seeker := &findAstNodeRelatedToPosition{Position: position}

	log.Println("position before walker: ", position)
	parser.Walk(seeker, file.root)
	log.Printf("seeker after walker : %#v\n", seeker)

	if seeker.TokenFound == nil {
		// No definition found
		return nil
	}

	// 2. From the node and token found, find the appropriate 'Source Definition'
	name := string(seeker.TokenFound.Value)

	if seeker.IsTemplate {
		log.Println("--> 4422 file.templates available:")
		for templateName, template := range file.templates {
			log.Printf("---> templateName = %s :::: template = %#v\n", templateName, template)
		}

		// TODO:
		// WIP
		// Find in all workspace, instead of just related to file
		// because template name with multiple definition throughout the workspace
		// cannot be computed with the all method eg. def.FileName == "" && def.Type == anyType

		// OLD VERSION
		/*
			templateDef := file.templates[name]
			if templateDef == nil {
				// return nil, seeker.TokenFound, seeker.NodeFound
				return nil
			}
		*/

		// NEW VERSION
		var allTemplateDefs []NodeDefinition = nil
		templateManager := TEMPLATE_MANAGER

		for templateScope, def := range templateManager.TemplateScopeToDefinition {
			if name == templateScope.TemplateName() {
				allTemplateDefs = append(allTemplateDefs, def)
			}
		}

		return allTemplateDefs

	} else if seeker.IsVariable {

		const MAX_LOOP_REPETITION int = 20
		var count int = 0

		parentScope := seeker.LastParent

		// Bubble up until you find the scope where the variable is defined
		for parentScope != nil {
			count++
			if count > MAX_LOOP_REPETITION {
				panic("possible infinite loop detected while processing 'goToDefinition()'")
			}

			scopedVariables := file.GetScopedVariables(parentScope)

			variableDef, ok := scopedVariables[name]
			if ok {
				singleDefinition := make([]NodeDefinition, 1)
				singleDefinition[0] = variableDef

				return singleDefinition
			}

			parentScope = parentScope.Parent()
		}

		return nil

	} else if seeker.IsExpression {
		// Either check the token is a 'function' or a 'variable'
		// There is no in-between

		// A. function check
		functionDef := file.functions[name]
		if functionDef != nil {
			singleDefinition := make([]NodeDefinition, 1)
			singleDefinition[0] = functionDef

			return singleDefinition
		}

		// B. otherwise variable check
		const MAX_LOOP_REPETITION int = 20
		var count int = 0

		parentScope := seeker.LastParent

		// Bubble up until you find the scope where the variable is defined
		for parentScope != nil {
			count++
			if count > MAX_LOOP_REPETITION {
				panic("possible infinite loop detected while processing 'goToDefinition()'")
			}

			scopedVariables := file.GetScopedVariables(parentScope)

			variableDef, ok := scopedVariables[name]
			if ok {
				singleDefinition := make([]NodeDefinition, 1)
				singleDefinition[0] = variableDef

				return singleDefinition
			}

			parentScope = parentScope.Parent()
		}

		return nil
	}

	return nil
}

func FindAstNodeRelatedToPosition(root *parser.GroupStatementNode, position lexer.Position) (*lexer.Token, parser.AstNode, *parser.GroupStatementNode, bool) {
	seeker := &findAstNodeRelatedToPosition{Position: position}

	log.Println("position before walker: ", position)
	parser.Walk(seeker, root)
	log.Printf("seeker after walker : %#v\n", seeker)

	return seeker.TokenFound, seeker.NodeFound, seeker.LastParent, seeker.IsTemplate
}

type findAstNodeRelatedToPosition struct {
	Position     lexer.Position
	TokenFound   *lexer.Token
	LastParent   *parser.GroupStatementNode
	NodeFound    parser.AstNode // nodeStatement
	IsTemplate   bool
	IsVariable   bool
	IsExpression bool
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

	if !node.Range().Contains(v.Position) {
		log.Println("NOPE, POSITION OUT OF RANGE, GO ELSEWHERE. nodeRange = ", node.Range())
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
			v.IsVariable = true

			return nil
		}

		v.NodeFound = n

		return v
	case *parser.VariableDeclarationNode:
		for _, variable := range n.VariableNames {
			if variable.Range.Contains(v.Position) {
				v.TokenFound = variable
				v.NodeFound = n
				v.IsVariable = true

				return nil
			}
		}

		v.NodeFound = n

		return v
	case *parser.MultiExpressionNode:
		for _, expression := range n.Expressions {
			if expression.Range().Contains(v.Position) {
				/*
					if v.NodeFound == nil {
						v.NodeFound = n
					}
				*/

				return v
			}
		}

		return nil
	case *parser.ExpressionNode:
		for _, symbol := range n.Symbols {
			if symbol.Range.Contains(v.Position) {
				v.TokenFound = symbol
				v.IsExpression = true

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
		templateFound := file.templates[name]

		if templateFound == nil {
			return file.Name(), nil, lexer.Range{}
		}

		if templateFound.fileName == "" {
			return file.Name(), nil, lexer.Range{}
		}

		return templateFound.fileName, templateFound.Node(), templateFound.Range()
	}

	name := string(from.Value)

	// 2. Try to find the function
	functionFound := file.functions[name]
	if functionFound != nil {
		return functionFound.fileName, functionFound.Node(), functionFound.Range()
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
			return variableFound.fileName, variableFound.Node(), variableFound.Range()
		}

		parentScope = parentScope.Parent()
	}

	return file.Name(), nil, lexer.Range{}
}

func Hover(definition NodeDefinition) (string, *lexer.Range) {
	if definition == nil {
		panic("Hover() do not accept <nil> definition")
	}

	reach := definition.Range()
	typeStringified := definition.TypeString()

	typeStringified = "```go\n" + typeStringified + "\n```"

	return typeStringified, &reach
}

/*
func Hover(from *lexer.Token, parentNodeStatement parser.AstNode, parentScope *parser.GroupStatementNode, file *FileDefinition, isTemplate bool) (fileName string, defFound parser.AstNode, reach lexer.Range) {
	// TODO: split those 3 check into function that return the corresponding definition : check for template, function and variable

	// Check if Template (return type for '$' or '.')
	if isTemplate {
		templateName := string(from.Value)

		templateDef := file.templates[templateName]
		if templateDef == nil {
			return "", nil
		}

		return templateDef.Name, templateDef.Type()
	}

	// Check if Function
	// Check if Method
	//
	// Check if Variable
	// Check if Property
	//

	// return tokenName, tokenType
	// return token, tokenType ???
	// return tokenName, tokenType, err ???
	// return tokenName, tokenType, varType(func, var, template) ???

	panic("not implemented yet")
}
*/

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
