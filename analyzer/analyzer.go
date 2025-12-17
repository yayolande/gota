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
	"strconv"
	"strings"

	"github.com/yayolande/gota/lexer"
	"github.com/yayolande/gota/parser"
)

var (
	TYPE_ANY                                   = types.Universe.Lookup("any")
	TYPE_ERROR                                 = types.Universe.Lookup("error")
	TEMPLATE_MANAGER *WorkspaceTemplateManager = nil
)

const (
	OPERATOR_STRICT_TYPE OperatorType = iota
	OPERATOR_COMPATIBLE_TYPE
	// OPERATOR_KEY_VALUE_ITERABLE_TYPE
	OPERATOR_VALUE_ITERABLE_TYPE
	OPERATOR_KEY_ITERABLE_TYPE

	NAME_TEMPORARY_VAR string = "__TMP_VAR_TO_RECHECK_"
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

type InferenceFunc func(symbol *lexer.Token, symbolType, constraintType types.Type) (*collectionPostCheckImplicitTypeNode, *parser.ParseError)

type InferenceFoundReturn struct {
	uniqueVariableInExpression     *collectionPostCheckImplicitTypeNode
	variablesToRecheckAtEndOfScope []*collectionPostCheckImplicitTypeNode // used both to: type check & type resolution from 'def.TreeImplicitType' to 'def.typ'
}

type KeyValuePairDefinition struct {
	key, value *VariableDefinition
}

type OperatorType int

type collectionPostCheckImplicitTypeNode struct {
	candidate       *nodeImplicitType // Warning, this implicit node is not related to its variable definition
	candidateDef    *VariableDefinition
	candidateSymbol *lexer.Token

	constraint       *nodeImplicitType
	constraintDef    *VariableDefinition
	constraintSymbol *lexer.Token

	operation        OperatorType
	isAssignmentNode bool
}

func newCollectionPostCheckImplicitTypeNode(candidateNode, constraintNode *nodeImplicitType, candidateDef, constraintDef *VariableDefinition, candidateToken, constraintToken *lexer.Token) *collectionPostCheckImplicitTypeNode {
	if candidateToken == nil {
		log.Printf("candidate token to recheck later is <nil>\n"+" candidateImplicitType = %s\n candidateVarDef = %s\n", candidateNode, candidateDef)
		panic("candidate token to recheck later is <nil>")
	} else if candidateNode == nil {
		log.Printf("candidate implicit tree type to recheck later is <nil>\n"+" candidateVarDef = %s\n candidateToken = %s\n", candidateDef, candidateToken)
		panic("candidate implicit tree type to recheck later is <nil>")
	} else if candidateDef == nil {
		log.Printf("condidate variable definition to recheck later is <nil>\n"+" candidateTree = %s\n candidateToken = %s\n", candidateNode, candidateToken)
		panic("condidate variable definition to recheck later is <nil>")
	} else if constraintNode == nil {
		log.Printf("constraint implicit tree type to recheck later is <nil>\n"+" constraintVarDef = %s\n constraintToken = %s\n", constraintDef, constraintToken)
		panic("constraint implicit tree type to recheck later is <nil>")
	}

	ephemeral := extractOrInsertTemporaryImplicitTypeFromVariable(candidateDef, candidateToken)
	if candidateNode != ephemeral {
		panic("node tree extracted from 'candidateDef' do not correspond to the received one")
	}

	collection := &collectionPostCheckImplicitTypeNode{
		candidate:       candidateNode,
		candidateDef:    candidateDef,
		candidateSymbol: candidateToken,

		constraint:       constraintNode,
		constraintDef:    constraintDef,
		constraintSymbol: constraintToken,

		operation:        OPERATOR_STRICT_TYPE,
		isAssignmentNode: false,
	}

	return collection
}

type collectionPostCheckVariable struct {
	varDef                 *VariableDefinition
	symbol                 *lexer.Token
	constraintType         types.Type
	strictTypeCheckEnabled bool
}

func newCollectionPostCheckVariable(varDef *VariableDefinition, symbol *lexer.Token, constraintType types.Type) *collectionPostCheckVariable {
	// func newCollectionPostCheckVariable(varDef *VariableDefinition, symbol *lexer.Token, constraintType types.Type) *collectionPostCheckImplicitTypeNode {
	panic("function deprecated")

	/*
		return &collectionPostCheckVariable{
			varDef:                 varDef,
			symbol:                 symbol,
			constraintType:         constraintType,
			strictTypeCheckEnabled: true,
		}
	*/
}

type NodeDefinition interface {
	Name() string
	FileName() string
	Node() parser.AstNode
	Range() lexer.Range
	Type() types.Type
	TypeString() string
}

type BasicSymbolDefinition struct {
	node     parser.AstNode
	rng      lexer.Range
	fileName string
	name     string
	typ      *types.Basic
}

func (b BasicSymbolDefinition) Name() string {
	return b.name
}

func (b BasicSymbolDefinition) FileName() string {
	return b.fileName
}

func (b BasicSymbolDefinition) Type() types.Type {
	return b.typ
}

func (b BasicSymbolDefinition) Node() parser.AstNode {
	return b.node
}

func (b BasicSymbolDefinition) Range() lexer.Range {
	return b.rng
}

func (b BasicSymbolDefinition) TypeString() string {
	return fmt.Sprintf("var _ %s = %s", b.typ.String(), b.name)
}

//
// New DEFINITION
//

// WARNING: Name of this struct is not appropriate
type KeywordSymbolDefinition struct {
	node     parser.AstNode
	rng      lexer.Range
	fileName string
	name     string /// ???
	// typ      *types.Basic /// ???
}

func (k KeywordSymbolDefinition) Name() string {
	return k.name
}

func (k KeywordSymbolDefinition) FileName() string {
	return k.fileName
}

func (k KeywordSymbolDefinition) Type() types.Type {
	return types.Typ[types.Invalid]
}

func (k KeywordSymbolDefinition) Node() parser.AstNode {
	return k.node
}

func (k KeywordSymbolDefinition) Range() lexer.Range {
	return k.rng
}

func (k KeywordSymbolDefinition) TypeString() string {
	return k.name
}

func NewKeywordSymbolDefinition(name string, fileName string, node parser.AstNode) *KeywordSymbolDefinition {
	def := &KeywordSymbolDefinition{
		name:     name,
		node:     node,
		rng:      node.Range(),
		fileName: fileName,
	}

	return def
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

// Declared type (type known at declaration) VS inferred type (type deduced by compiler)
type VariableDefinition struct {
	node       parser.AstNode // direct node containing info about this variable
	parent     *parser.GroupStatementNode
	rng        lexer.Range // variable lifetime
	fileName   string
	name       string
	typ        types.Type // declared type
	shadowType types.Type

	TreeImplicitType *nodeImplicitType // inferred type
	IsUsedOnce       bool              // Only useful to detect whether or not a variable have never been used in the scope
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

func (v VariableDefinition) Parent() *parser.GroupStatementNode {
	return v.parent
}

func (v VariableDefinition) Range() lexer.Range {
	return v.rng
}

func (v VariableDefinition) TypeString() string {
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
	root                                       *parser.GroupStatementNode
	name                                       string
	typeHints                                  map[*parser.GroupStatementNode]types.Type // ???
	scopeToVariables                           map[*parser.GroupStatementNode](map[string]*VariableDefinition)
	functions                                  map[string]*FunctionDefinition
	templates                                  map[string]*TemplateDefinition
	isTemplateGroupAlreadyAnalyzed             bool
	extraVariableNameWithTypeInferenceBehavior map[string]*VariableDefinition // only useful to allow type inference on 'key' of the loop
	secondaryVariable                          *VariableDefinition            // only useful for passing around the 'key' of the loop
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
	file.isTemplateGroupAlreadyAnalyzed = false

	file.typeHints = make(map[*parser.GroupStatementNode]types.Type)
	file.templates = make(map[string]*TemplateDefinition)
	file.extraVariableNameWithTypeInferenceBehavior = make(map[string]*VariableDefinition)
	file.secondaryVariable = nil

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

	globalVariables, localVariables := NewGlobalAndLocalVariableDefinition(nil, root, fileName)

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
	file.root = partialFile.root
	file.functions = partialFile.functions
	file.scopeToVariables = partialFile.scopeToVariables

	return file, globalVariables, localVariables
}

func NewTemplateDefinition(name string, fileName string, node parser.AstNode, rng lexer.Range, typ types.Type) *TemplateDefinition {
	def := &TemplateDefinition{}
	def.name = name
	def.fileName = fileName
	def.node = node
	def.rng = rng
	def.inputType = typ

	return def
}

// TODO: remove this function and use *parser.GroupStatementNode.ShortCut.VariableDeclarations
// But seriously, I am sure sure about it since 'VariableDeclarationNode' is defined in 'parser' package
// but the 'VariableDefinition' is instead defined in 'analyzer'
// Mixing both DS will create a cyclical import issue
//
// Deprecated: is it really deprecated ?????????????
func (f FileDefinition) GetScopedVariables(scope *parser.GroupStatementNode) map[string]*VariableDefinition {
	scopedVariables := f.scopeToVariables[scope]
	if scopedVariables == nil {
		scopedVariables = make(map[string]*VariableDefinition)
	}

	return scopedVariables
}

func (f FileDefinition) GetVariableDefinitionWithinScope(variableName string, scope *parser.GroupStatementNode) *VariableDefinition {
	const MAX_LOOP_REPETITION int = 20
	var count int = 0

	for scope != nil {
		count++
		if count > MAX_LOOP_REPETITION {
			panic("possible infinite loop detected while processing 'GetVariableDefinitionWithinScope()'")
		}

		scopedVariables := f.GetScopedVariables(scope)

		varDef, ok := scopedVariables[variableName]
		if ok {
			return varDef
		}

		if scope.IsTemplate() {
			break
		}

		scope = scope.Parent()
	}

	return nil
}

// ------------
// Start Here -
// ------------

// TODO: this need some more work to be usable universally by all files within need to be recreated each time
// add types to every builtin functions
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

func NewGlobalAndLocalVariableDefinition(node parser.AstNode, parent *parser.GroupStatementNode, fileName string) (map[string]*VariableDefinition, map[string]*VariableDefinition) {
	globalVariables := make(map[string]*VariableDefinition)
	localVariables := make(map[string]*VariableDefinition)

	localVariables["."] = NewVariableDefinition(".", nil, parent, fileName)
	localVariables["$"] = NewVariableDefinition("$", nil, parent, fileName)

	return globalVariables, localVariables
}

func NewVariableDefinition(variableName string, node parser.AstNode, parent *parser.GroupStatementNode, fileName string) *VariableDefinition {
	def := &VariableDefinition{}

	def.name = variableName
	def.parent = parent
	def.fileName = fileName

	def.TreeImplicitType = nil
	def.typ = TYPE_ANY.Type()

	if node != nil {
		def.node = node
		def.rng = node.Range()
	}

	return def
}

func cloneVariableDefinition(old *VariableDefinition) *VariableDefinition {
	fresh := &VariableDefinition{}

	fresh.name = old.name
	fresh.fileName = old.fileName

	fresh.typ = old.typ
	fresh.node = old.node
	fresh.parent = old.parent
	fresh.rng = old.rng

	fresh.IsUsedOnce = old.IsUsedOnce
	fresh.TreeImplicitType = old.TreeImplicitType

	return fresh
}

func DefinitionAnalysisFromPartialFile(partialFile *FileDefinition, outterTemplate map[*parser.GroupStatementNode]*TemplateDefinition) (*FileDefinition, []lexer.Error) {
	if partialFile == nil {
		log.Printf("expected a partial file but got <nil> for 'DefinitionAnalysisFromPartialFile()'")
		panic("expected a partial file but got <nil> for 'DefinitionAnalysisFromPartialFile()'")
	}

	file, globalVariables, localVariables := NewFileDefinitionFromPartialFile(partialFile, outterTemplate)
	file.isTemplateGroupAlreadyAnalyzed = true

	typ, _, errs := definitionAnalysisRecursive(file.root, nil, file, globalVariables, localVariables)

	_ = typ
	// file.typeHints[file.root] = typ[0]

	return file, errs
}

func DefinitionAnalysis(fileName string, node *parser.GroupStatementNode, outterTemplate map[*parser.GroupStatementNode]*TemplateDefinition) (*FileDefinition, []lexer.Error) {
	if node == nil {
		return nil, nil
	}

	fileInfo, globalVariables, localVariables := NewFileDefinition(fileName, node, outterTemplate)

	_, _, errs := definitionAnalysisRecursive(node, nil, fileInfo, globalVariables, localVariables)

	return fileInfo, errs
}

func definitionAnalysisRecursive(node parser.AstNode, parent *parser.GroupStatementNode, file *FileDefinition, globalVariables, localVariables map[string]*VariableDefinition) ([2]types.Type, InferenceFoundReturn, []lexer.Error) {
	if globalVariables == nil || localVariables == nil {
		panic("arguments global/local variable defintion for 'definitionAnalysis()' shouldn't be 'nil'")
	}

	var errs []lexer.Error
	var localInferences InferenceFoundReturn
	var statementType [2]types.Type

	switch n := node.(type) {
	case *parser.GroupStatementNode:
		statementType, localInferences, errs = definitionAnalysisGroupStatement(n, parent, file, globalVariables, localVariables)

	case *parser.TemplateStatementNode:
		statementType, localInferences, errs = definitionAnalysisTemplatateStatement(n, parent, file, globalVariables, localVariables)

	case *parser.CommentNode:
		statementType, localInferences, errs = definitionAnalysisComment(n, parent, file, globalVariables, localVariables)

	case *parser.VariableDeclarationNode:
		statementType, localInferences, errs = definitionAnalysisVariableDeclaration(n, parent, file, globalVariables, localVariables)

	case *parser.VariableAssignationNode:
		statementType, localInferences, errs = definitionAnalysisVariableAssignment(n, parent, file, globalVariables, localVariables)

	case *parser.MultiExpressionNode:
		statementType, localInferences, errs = definitionAnalysisMultiExpression(n, parent, file, globalVariables, localVariables)

	case *parser.ExpressionNode:
		statementType, localInferences, errs = definitionAnalysisExpression(n, parent, file, globalVariables, localVariables)

	case *parser.SpecialCommandNode:
		// nothing to analyze here

	default:
		panic("unknown parseNode found. node type = " + node.Kind().String())
	}

	return statementType, localInferences, errs
}

func analyzeGroupStatementHeader(group *parser.GroupStatementNode, file *FileDefinition, scopedGlobalVariables, localVariables map[string]*VariableDefinition) ([2]types.Type, InferenceFoundReturn, []lexer.Error) {
	var controlFlowType [2]types.Type
	var inferences InferenceFoundReturn
	var errs []lexer.Error

	group.IsProcessingHeader = true
	defer func() { group.IsProcessingHeader = false }() // just for safety

	switch group.Kind() {
	case parser.KIND_IF, parser.KIND_ELSE_IF, parser.KIND_RANGE_LOOP, parser.KIND_WITH,
		parser.KIND_ELSE_WITH, parser.KIND_DEFINE_TEMPLATE, parser.KIND_BLOCK_TEMPLATE:

		if group.Err != nil { // do not analyze the header
			controlFlowType[0] = types.Typ[types.Invalid]
			break
		}

		if group.ControlFlow == nil {
			log.Printf("fatal, 'controlFlow' not found for 'GroupStatementNode'. \n %s \n", group)
			panic("this 'GroupStatementNode' expect a non-nil 'controlFlow' based on its type ('Kind') " + group.Kind().String())
		}

		// The only purpose of this is to ease the computation of '.' var type
		// by embedding 'MultiExpressionNode' within a 'VariableDeclarationNode'
		if group.Kind() == parser.KIND_RANGE_LOOP {
			if mExpression, ok := group.ControlFlow.(*parser.MultiExpressionNode); ok {
				varName := NAME_TEMPORARY_VAR + strconv.Itoa(parser.GetUniqueNumber())
				variable := lexer.NewToken(lexer.DOLLAR_VARIABLE, group.ControlFlow.Range(), []byte(varName))

				temporaryVarNode := parser.NewVariableDeclarationNode(parser.KIND_VARIABLE_DECLARATION, group.Range().Start, group.Range().End, nil)
				temporaryVarNode.Value = mExpression
				temporaryVarNode.VariableNames = append(temporaryVarNode.VariableNames, variable)

				controlFlowType, inferences, errs = definitionAnalysisRecursive(temporaryVarNode, group, file, scopedGlobalVariables, localVariables)
				break
			}
		}

		controlFlowType, inferences, errs = definitionAnalysisRecursive(group.ControlFlow, group, file, scopedGlobalVariables, localVariables)
	}

	group.IsProcessingHeader = false

	return controlFlowType, inferences, errs
}

func definitionAnalysisGroupStatement(node *parser.GroupStatementNode, _ *parser.GroupStatementNode, file *FileDefinition, globalVariables, localVariables map[string]*VariableDefinition) ([2]types.Type, InferenceFoundReturn, []lexer.Error) {
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
	file.secondaryVariable = nil

	var inferences InferenceFoundReturn
	var errs, localErrs []lexer.Error
	var controlFlowType, scopeType [2]types.Type

	scopeType[0] = TYPE_ANY.Type()
	scopeType[1] = TYPE_ERROR.Type()

	// 2. ControlFlow analysis
	controlFlowType, inferences, errs = analyzeGroupStatementHeader(node, file, scopedGlobalVariables, localVariables)

	// 3. Set Variables Scope
	// TODO: Use the new 'node.IsGroupWithNoVariableReset()' and the like
	switch node.Kind() {
	case parser.KIND_IF, parser.KIND_ELSE, parser.KIND_ELSE_IF, parser.KIND_END:

	case parser.KIND_RANGE_LOOP:
		var primaryVariable *VariableDefinition

		if inferences.uniqueVariableInExpression == nil {
			primaryVariable = NewVariableDefinition(".", node, node.Parent(), file.FileName())
			primaryVariable.typ = types.Typ[types.Invalid]
		} else {
			primaryVariable = inferences.uniqueVariableInExpression.candidateDef
		}

		localVariables["."] = primaryVariable
		markVariableAsUsed(localVariables["."])

		// if 'key' exists for the current loop, enable type inference for it
		// (this is an exceptional case only found in 'range' loop, since everywhere else '$' var dont have type inference capability)
		if file.secondaryVariable != nil {
			def := file.secondaryVariable
			file.secondaryVariable = nil

			if otherDef := file.extraVariableNameWithTypeInferenceBehavior[def.name]; otherDef != nil { // useful to save and later put back the original value, if it exists in the first place
				defer func() {
					file.extraVariableNameWithTypeInferenceBehavior[def.name] = otherDef
				}()
			}

			file.extraVariableNameWithTypeInferenceBehavior[def.name] = def
		}

	case parser.KIND_WITH, parser.KIND_ELSE_WITH:
		localVariables["."] = NewVariableDefinition(".", node.ControlFlow, node, file.Name())
		localVariables["."].typ = controlFlowType[0]

		// NOTE: This enable type resolution at end of scope for context variable '.'
		rhs := inferences.uniqueVariableInExpression
		if types.Identical(controlFlowType[0], TYPE_ANY.Type()) == true &&
			types.Identical(rhs.candidateDef.typ, TYPE_ANY.Type()) == true && rhs != nil {

			exprTree := extractOrInsertTemporaryImplicitTypeFromVariable(rhs.candidateDef, rhs.candidateSymbol)
			varDef := localVariables["."]
			varDef.TreeImplicitType = exprTree

			varName := NAME_TEMPORARY_VAR + strconv.Itoa(parser.GetUniqueNumber())
			fakeTree := newNodeImplicitType(varName, TYPE_ANY.Type(), rhs.candidate.rng)
			symbol := lexer.NewToken(lexer.DOT_VARIABLE, varDef.rng, []byte("."))

			recheck := newCollectionPostCheckImplicitTypeNode(varDef.TreeImplicitType, fakeTree, varDef, nil, symbol, nil)
			inferences.variablesToRecheckAtEndOfScope = append(inferences.variablesToRecheckAtEndOfScope, recheck) // enable late type resolution for type inference
			inferences.uniqueVariableInExpression = nil

			// NOTE: this is useful to type check the any 'header' type with the 'inner scope' type
			// the only goal is to manage the case rhs.candidate == TYPE_ANY
		} else if types.Identical(controlFlowType[0], TYPE_ANY.Type()) == true &&
			types.Identical(rhs.candidateDef.typ, TYPE_ANY.Type()) == false && rhs != nil {

			varDef := localVariables["."]
			varSymbol := lexer.NewToken(lexer.DOLLAR_VARIABLE, varDef.rng, []byte(varDef.name))

			exprDef := rhs.candidateDef
			exprSymbol := rhs.candidateSymbol
			exprTree := extractOrInsertTemporaryImplicitTypeFromVariable(rhs.candidateDef, rhs.candidateSymbol)
			exprTree.toDiscard = false

			if types.Identical(rhs.candidateDef.typ, TYPE_ANY.Type()) {
				varDef.TreeImplicitType = exprTree
			} else {
				varDef.TreeImplicitType = newNodeImplicitType(varDef.name, TYPE_ANY.Type(), varDef.rng)
			}

			if types.Identical(varDef.TreeImplicitType.fieldType, types.Typ[types.Invalid]) {
				varDef.typ = types.Typ[types.Invalid]
			}

			recheck := newCollectionPostCheckImplicitTypeNode(exprTree, varDef.TreeImplicitType, exprDef, varDef, exprSymbol, varSymbol)
			inferences.variablesToRecheckAtEndOfScope = append(inferences.variablesToRecheckAtEndOfScope, recheck) // enable late type resolution for type inference
			inferences.uniqueVariableInExpression = nil
		}

		markVariableAsUsed(localVariables["."])

	case parser.KIND_DEFINE_TEMPLATE, parser.KIND_BLOCK_TEMPLATE, parser.KIND_GROUP_STATEMENT:
		scopedGlobalVariables = make(map[string]*VariableDefinition)
		localVariables = make(map[string]*VariableDefinition)

		localVariables["."] = NewVariableDefinition(".", node, node.Parent(), file.name)
		localVariables["$"] = localVariables["."]

		markVariableAsUsed(localVariables["."])

		commentGoCode := node.ShortCut.CommentGoCode
		if commentGoCode != nil {
			_, _, localErrs := definitionAnalysisComment(commentGoCode, node, file, scopedGlobalVariables, localVariables)
			errs = append(errs, localErrs...)
		}

	default:
		panic("found unexpected 'Kind' for 'GroupStatementNode' during 'DefinitionAnalysis()'\n node = " + node.String())
	}

	// 4. Statements analysis
	var statementType [2]types.Type
	var localInferences InferenceFoundReturn

	for _, statement := range node.Statements {
		if statement == nil {
			panic("statement within 'GroupStatementNode' cannot be nil. make to find where this nil value has been introduced and rectify it")
		}

		// skip already analyzed 'goCode' (done above)
		if statement == node.ShortCut.CommentGoCode {
			continue
		}

		// skip template scope analysis when already done during template dependencies analysis
		scope, isScope := statement.(*parser.GroupStatementNode)
		if isScope && scope.IsTemplate() && file.isTemplateGroupAlreadyAnalyzed {
			if scope.Kind() == parser.KIND_BLOCK_TEMPLATE { // analyze the header 'expression' before skipping
				statementType, localInferences, localErrs = analyzeGroupStatementHeader(scope, file, scopedGlobalVariables, localVariables)
				errs = append(errs, localErrs...)
				inferences.variablesToRecheckAtEndOfScope = append(inferences.variablesToRecheckAtEndOfScope, localInferences.variablesToRecheckAtEndOfScope...)
			}

			continue
		}

		// Make DefinitionAnalysis for every children
		statementType, localInferences, localErrs = definitionAnalysisRecursive(statement, node, file, scopedGlobalVariables, localVariables)
		errs = append(errs, localErrs...)
		inferences.variablesToRecheckAtEndOfScope = append(inferences.variablesToRecheckAtEndOfScope, localInferences.variablesToRecheckAtEndOfScope...)

		if statementType[1] == nil || types.Identical(statementType[1], TYPE_ERROR.Type()) {
			continue
		}

		err := parser.NewParseError(&lexer.Token{}, fmt.Errorf("%w, second return type must be an 'error' type", errTypeMismatch))
		err.Range = statement.Range()
		errs = append(errs, err)
	}

	// Verify that all 'localVariables' have been used at least once
	for _, def := range localVariables {
		otherDef := file.extraVariableNameWithTypeInferenceBehavior[def.name]
		if otherDef == def {
			delete(file.extraVariableNameWithTypeInferenceBehavior, def.name)
		}

		if def.IsUsedOnce {
			continue
		}

		err := parser.NewParseError(&lexer.Token{}, errVariableNotUsed)
		err.Range = def.Node().Range()
		errs = append(errs, err)
	}

	// Implicitly guess the type of var '.' if no type is found (empty or 'any')
	if node.IsGroupWithDollarAndDotVariableReset() {
		typ := guessVariableTypeFromImplicitType(localVariables["."])
		localVariables["."].typ = typ

		file.typeHints[node] = localVariables["."].typ

		//
		// Check Implicit Node for the end of this scope (type inference resolution)
		//
		for _, recheck := range inferences.variablesToRecheckAtEndOfScope {
			if recheck == nil {
				panic("found unexpect <nil> amongs variable to recheck at end of scope")
			}

			// 1. Test that the whole token is valid, and build the root variable type
			constraintDef := recheck.constraintDef
			if constraintDef != nil {
				constraintDef.typ = guessVariableTypeFromImplicitType(constraintDef)

				tmpNode := extractOrInsertTemporaryImplicitTypeFromVariable(constraintDef, recheck.constraintSymbol)
				if recheck.constraint != tmpNode && recheck.constraint.toDiscard == false { // bc 'discardable' node can later be deleted (eg. inserting iterable when only discardable node are found)
					panic("constraint 'nodeImplicitType' do not match the one comming from its variable definition")
				}
			}

			candidateToken := recheck.candidateSymbol
			candidateDef := recheck.candidateDef

			tmpNode := extractOrInsertTemporaryImplicitTypeFromVariable(candidateDef, candidateToken)
			if recheck.candidate != tmpNode && recheck.candidate.toDiscard == false { // bc 'discardable' node can later be deleted (eg. inserting iterable when only discardable node are found)
				panic("'nodeImplicitType' do not match the one comming from its variable definition")
			}

			// this is to make sure that the variable path exist, as defined by the variable type
			candidateDef.typ = guessVariableTypeFromImplicitType(candidateDef)
			_, err := getRealTypeAssociatedToVariable(candidateToken, candidateDef)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			// 2. Test that the implicit type tree is valid
			candidateType := buildTypeFromTreeOfType(recheck.candidate)
			constraintType := buildTypeFromTreeOfType(recheck.constraint)

			switch recheck.operation {
			case OPERATOR_STRICT_TYPE:
				_, errMsg := TypeCheckAgainstConstraint(candidateType, constraintType)
				if errMsg != nil {
					err := parser.NewParseError(recheck.candidateSymbol, errMsg)
					errs = append(errs, err)
				}

			case OPERATOR_COMPATIBLE_TYPE:
				_, errMsg := TypeCheckCompatibilityWithConstraint(candidateType, constraintType)
				if errMsg != nil {
					err := parser.NewParseError(recheck.candidateSymbol, errMsg)
					errs = append(errs, err)
				}

			case OPERATOR_KEY_ITERABLE_TYPE:
				exprTree := recheck.candidate
				keyTree := recheck.constraint

				if keyTree == nil {
					panic("key implicit tree is <nil> while rechecking at end of scope (key of a loop)")
				} else if exprTree == nil {
					panic("expression inference tree is <nil> while rechecking at end of scope (expression of the loop)")
				}

				if !exprTree.isIterable { // no need to return an error while checking the key, since if the key is present, so is the value as well (we will check there)
					continue
				}

				keyType := buildTypeFromTreeOfType(keyTree)

				expectedKeyNode := exprTree.children["key"]
				if expectedKeyNode == nil {
					if types.Identical(keyType, types.Typ[types.Int]) { // sucess
						continue
					}

					errMsg := fmt.Errorf("%w, expect 'int' but found %s", errTypeMismatch, keyType.String())
					err := parser.NewParseError(recheck.constraintSymbol, errMsg)
					errs = append(errs, err)
					continue
				}

				expectedKeyType := buildTypeFromTreeOfType(expectedKeyNode)

				_, errMsg := TypeCheckAgainstConstraint(keyType, expectedKeyType)
				if errMsg != nil {
					err := parser.NewParseError(recheck.constraintSymbol, errMsg)
					errs = append(errs, err)
				}

			case OPERATOR_VALUE_ITERABLE_TYPE:

				exprTree := recheck.candidate
				valueTree := recheck.constraint

				if valueTree == nil {
					panic("value implicit tree is <nil> while rechecking at end of scope (key of a loop)")
				} else if exprTree == nil {
					panic("expression inference tree is <nil> while rechecking at end of scope (expression of the loop)")
				}

				if !exprTree.isIterable {
					errMsg := fmt.Errorf("%w, expected array, slice, map, int, chan, or iterator", errTypeMismatch)
					err := parser.NewParseError(recheck.candidateSymbol, errMsg)
					errs = append(errs, err)
					continue
				}

				valueType := buildTypeFromTreeOfType(valueTree)

				expectedValueNode := exprTree.children["value"]
				if expectedValueNode == nil {
					log.Printf("found <nil> value within iterable 'value'.\n exprTree = %#v\n recheck = %#v\n", exprTree, recheck)
					panic("found <nil> value within iterable 'value'")
				}

				expectedValueType := buildTypeFromTreeOfType(expectedValueNode)

				_, errMsg := TypeCheckAgainstConstraint(valueType, expectedValueType)
				if errMsg != nil {
					err := parser.NewParseError(recheck.constraintSymbol, errMsg)
					errs = append(errs, err)
				}

			default:
				panic("found unknown operator while rechecking variable type at end of scope")
			}

			// TODO: Not sure if this one is useful
			// WIP
			if recheck.isAssignmentNode && types.Identical(constraintDef.typ, TYPE_ANY.Type()) {
				varDef := constraintDef
				expressionType := candidateType

				if varDef.shadowType == nil || types.Identical(varDef.shadowType, TYPE_ANY.Type()) {
					varDef.shadowType = expressionType
				} else if !types.Identical(varDef.shadowType, TYPE_ANY.Type()) {
					_, errMsg := TypeCheckAgainstConstraint(expressionType, varDef.shadowType)
					if errMsg != nil {
						err := parser.NewParseError(recheck.candidateSymbol, fmt.Errorf("shadow %w", errMsg))
						errs = append(errs, err)
					}
				}
			}
		}

		// reset all inferences once resolved
		inferences.uniqueVariableInExpression = nil
		inferences.variablesToRecheckAtEndOfScope = nil
	}

	if localVariables["."] != nil {
		scopeType[0] = localVariables["."].typ
	}

	// save all local variables for the current scope; and set the type hint
	file.scopeToVariables[node] = localVariables

	return scopeType, inferences, errs
}

func definitionAnalysisTemplatateStatement(node *parser.TemplateStatementNode, parent *parser.GroupStatementNode, file *FileDefinition, globalVariables, localVariables map[string]*VariableDefinition) ([2]types.Type, InferenceFoundReturn, []lexer.Error) {
	if parent == nil {
		panic(fmt.Sprintf("template cannot be parentless, it shoud be contain in at least 1 one scope. file = %s :: node = %s", file.name, node.String()))
	}

	var errs, localErrs []lexer.Error
	var expressionType [2]types.Type
	var inferences InferenceFoundReturn

	invalidTypes := [2]types.Type{
		types.Typ[types.Invalid],
		TYPE_ERROR.Type(),
	}

	if node.Err != nil {
		return invalidTypes, inferences, nil
	}

	if node.TemplateName == nil {
		panic("the template name should never be empty for a template expression. make sure the template has been parsed correctly.\n" + node.String())
	}

	// 1. Expression analysis, if any
	if node.Expression != nil {
		expressionType, inferences, localErrs = definitionAnalysisRecursive(node.Expression, parent, file, globalVariables, localVariables)
		errs = append(errs, localErrs...)
	}

	// 2. template name analysis
	switch node.Kind() {
	case parser.KIND_DEFINE_TEMPLATE: // NOTE: the template definition has already been done in a previous phase (dependency analysis), no need to do again
		if parent.Kind() != node.Kind() {
			panic("value mismatch for 'define' kind. 'TemplateStatementNode.Kind' and 'TemplateStatementNode.parent.Kind' must be similar")
		}

		if parent.Parent().IsRoot() == false {
			err := parser.NewParseError(node.TemplateName, errors.New("template cannot be defined in local scope"))
			errs = append(errs, err)
		}

		if node.Expression != nil {
			err := parser.NewParseError(node.TemplateName, errors.New("'define' do not accept expression"))
			err.Range = node.Expression.Range()
			errs = append(errs, err)
		}

		inferences.uniqueVariableInExpression = nil

	case parser.KIND_BLOCK_TEMPLATE: // NOTE: the template definition has already been done in a previous phase (dependency analysis), fallthrough to next case
		if parent.Kind() != node.Kind() {
			panic("value mismatch for 'block' kind. 'TemplateStatementNode.Kind' and 'TemplateStatementNode.parent.Kind' must be similar")
		}

		if file.isTemplateGroupAlreadyAnalyzed == false {
			if parent.Parent().IsRoot() == false {
				err := parser.NewParseError(node.TemplateName, errors.New("template cannot be defined in local scope"))
				errs = append(errs, err)
			}

			if node.Expression == nil {
				err := parser.NewParseError(node.TemplateName, errors.New("missing expression"))
				errs = append(errs, err)
				return invalidTypes, inferences, errs
			}

			break
		}

		// file.isTemplateGroupAlreadyAnalyzed == true
		fallthrough

		// only type check expression after 'template dependency analysis'
		// the second phase is enabled though 'GroupStatementNode', while type checking every children

		/*
			rhs := inferences.uniqueVariableInExpression
			if rhs == nil {
				id := strconv.Itoa(parser.GetUniqueNumber())
				varName := NAME_TEMPORARY_VAR + id
				exprSymbol := lexer.NewToken(lexer.DOLLAR_VARIABLE, node.Expression.Range(), []byte(varName))
				exprDef := NewVariableDefinition("$_transit_expr_"+id, node.Expression, parent, file.name)
				exprDef.typ = expressionType[0]
				exprTree := extractOrInsertTemporaryImplicitTypeFromVariable(exprDef, exprSymbol)
				rhs = newCollectionPostCheckImplicitTypeNode(exprTree, exprTree, exprDef, nil, exprSymbol, nil)
			}

			fakeTree := newNodeImplicitType("$_fake_temporary_tree_template", types.Typ[types.Invalid], node.TemplateName.Range)
			recheck := newCollectionPostCheckImplicitTypeNode(rhs.candidate, fakeTree, rhs.candidateDef, nil, rhs.candidateSymbol, nil)
			recheck.operation = OPERATOR_COMPATIBLE_TYPE

			inferences.uniqueVariableInExpression = recheck
		*/

	case parser.KIND_USE_TEMPLATE:
		templateName := string(node.TemplateName.Value)

		templateDef, found := file.templates[templateName]
		if !found {
			err := parser.NewParseError(node.TemplateName, errTemplateUndefined)
			errs = append(errs, err)
			return invalidTypes, inferences, errs
		}

		if templateDef == nil {
			panic(fmt.Sprintf("'TemplateDefinition' cannnot be nil for an existing template. file = %#v", file))
		} else if templateDef.inputType == nil {
			panic(fmt.Sprintf("defined template cannot have 'nil' InputType. def = %#v", templateDef))
		}

		if expressionType[0] == nil {
			if types.Identical(templateDef.inputType, TYPE_ANY.Type()) {
				break
			}

			errMsg := fmt.Errorf("%w, template call expect expression", errTypeMismatch)
			err := parser.NewParseError(node.TemplateName, errMsg)
			errs = append(errs, err)
			return invalidTypes, inferences, errs
		}

		candidateType := expressionType[0]

		rhs := inferences.uniqueVariableInExpression
		if types.Identical(candidateType, TYPE_ANY.Type()) && rhs != nil {
			templateTree := newNodeImplicitType(templateName, templateDef.inputType, node.Range())
			recheck := newCollectionPostCheckImplicitTypeNode(rhs.candidate, templateTree, rhs.candidateDef, nil, rhs.candidateSymbol, nil)
			recheck.operation = OPERATOR_COMPATIBLE_TYPE

			inferences.variablesToRecheckAtEndOfScope = append(inferences.variablesToRecheckAtEndOfScope, recheck)
			break
		}

		_, errMsg := TypeCheckCompatibilityWithConstraint(candidateType, templateDef.inputType)
		if errMsg != nil {
			err := parser.NewParseError(node.TemplateName, errMsg)
			err.Range = node.Expression.Range()
			errs = append(errs, err)
			return invalidTypes, inferences, errs
		}

	default:
		panic("'TemplateStatementNode' do not accept any other type than 'KIND_DEFINE_TEMPLATE, KIND_BLOCK_TEMPLATE, KIND_USE_TEMPLATE'")
	}

	return expressionType, inferences, errs
}

// TODO: refactor this function, too many ugly code in here
// Only one 'go:code' allowed by files, the other one will be reported as error and discarded in the subsequent computation
// During this phase 'file' argument is modified (file.Functions)
func definitionAnalysisComment(comment *parser.CommentNode, parentScope *parser.GroupStatementNode, file *FileDefinition, _, localVariables map[string]*VariableDefinition) ([2]types.Type, InferenceFoundReturn, []lexer.Error) {

	var inferences InferenceFoundReturn

	commentType := [2]types.Type{
		TYPE_ANY.Type(),
		TYPE_ERROR.Type(),
	}

	if comment == nil || comment.Err != nil {
		return commentType, inferences, nil
	}

	if parentScope == nil {
		panic("'CommentNode' cannot be parentless, it shoud be contain in at least 1 one scope")
	}

	if comment.Kind() != parser.KIND_COMMENT {
		panic("found value mismatch for 'CommentNode.Kind' during DefinitionAnalysis().\n " + comment.String())
	}

	if comment.GoCode == nil {
		return commentType, inferences, nil
	}

	// Do not analyze orphaned 'GoCode'
	// Correct 'GoCode' is available in parent scope
	if comment != parentScope.ShortCut.CommentGoCode {
		return commentType, inferences, nil
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

		case *types.Named:

			if obj.Name() != "Input" {
				continue
			}

			if localVariables["."] == nil {
				continue
			}

			commentType[0] = typ

			convertGoAstPositionToProjectRange := func(goPosition token.Pos) lexer.Range {
				startPos := fileSet.Position(goPosition)
				endPos := fileSet.Position(goPosition)
				endPos.Column += endPos.Offset

				relativeRangeFunction := goAstPositionToRange(startPos, endPos)
				return remapRangeFromCommentGoCodeToSource(virtualHeader, comment.GoCode.Range, relativeRangeFunction)
			}

			// No need to handle 'localVariables["$"]' since it ultimately point to 'localVariables["."]' anyway
			localVariables["."].rng = convertGoAstPositionToProjectRange(obj.Pos())
			localVariables["."].typ = typ.Underlying()

			rootNode := newNodeImplicitType(obj.Name(), typ, localVariables["."].rng)
			localVariables["."].TreeImplicitType = createImplicitTypeFromRealType(rootNode, convertGoAstPositionToProjectRange)

		default:
			continue
		}
	}

	errs := convertThirdPartiesParseErrorToLocalError(err, errsType, file, comment, virtualHeader)
	errs = append(errs, errComments...)

	return commentType, inferences, errs
}

func definitionAnalysisVariableDeclaration(node *parser.VariableDeclarationNode, parentScope *parser.GroupStatementNode, file *FileDefinition, globalVariables, localVariables map[string]*VariableDefinition) ([2]types.Type, InferenceFoundReturn, []lexer.Error) {
	if parentScope == nil {
		panic("'variable declaration' cannot be parentless, it shoud be contain in at least 1 one scope")
	} else if node.Kind() != parser.KIND_VARIABLE_DECLARATION {
		log.Printf("found value mismatch for 'VariableDeclarationNode.Kind' during DefinitionAnalysis()"+"\n node = %#v\n parent = %#v\n", node, parentScope)
		panic("found value mismatch for 'VariableDeclarationNode.Kind' during DefinitionAnalysis()")
	} else if localVariables == nil || globalVariables == nil {
		log.Printf("either 'localVariables' or 'globalVariables' shouldn't be nil for 'VariableDeclarationNode.DefinitionAnalysis()'"+"\n localVariables = %#v\n globalVariables = %#v\n", localVariables, globalVariables)
		panic("either 'localVariables' or 'globalVariables' shouldn't be nil for 'VariableDeclarationNode.DefinitionAnalysis()'")
	}

	var errs []lexer.Error
	var expressionType [2]types.Type
	var localInferences InferenceFoundReturn
	invalidTypes := [2]types.Type{
		types.Typ[types.Invalid],
		TYPE_ERROR.Type(),
	}

	if node.Err != nil {
		return invalidTypes, localInferences, nil
	}

	if len(node.VariableNames) == 0 && len(node.VariableNames) > 2 {
		log.Printf("cannot analyze variable declaration with 0 or more than 2 variable. this error must be caught and discarded while parsing"+"\n node = %#v\n", node)
		panic("cannot analyze variable declaration with 0 or more than 2 variable. this error must be caught and discarded while parsing")
	}

	// 0. Check that variable names have proper syntax
	for _, variable := range node.VariableNames {
		if bytes.ContainsAny(variable.Value, ".") {
			err := parser.NewParseError(variable, errors.New("forbidden '.' in variable name while declaring"))
			errs = append(errs, err)

			return invalidTypes, localInferences, errs
		}
	}

	// 1. Check that 'expression' is valid
	if node.Value != nil {
		var localErrs []lexer.Error

		expressionType, localInferences, localErrs = definitionAnalysisMultiExpression(node.Value, parentScope, file, globalVariables, localVariables)
		errs = append(errs, localErrs...)

	} else {
		localErr := parser.NewParseError(&lexer.Token{}, errors.New("assignment expression cannot be empty"))
		localErr.Range = node.Range()
		errs = append(errs, localErr)

		return invalidTypes, localInferences, errs
	}

	// All the code blow suppose that:   len(node.VariableNames) > 0 && len(node.VariableNames) <= 2
	if parentScope.Kind() == parser.KIND_RANGE_LOOP && parentScope.IsProcessingHeader {

		// helper function
		createRecheckNode := func(def *VariableDefinition, variable *lexer.Token) *collectionPostCheckImplicitTypeNode {
			varName := NAME_TEMPORARY_VAR + strconv.Itoa(parser.GetUniqueNumber())
			fakeTree := newNodeImplicitType(varName, TYPE_ANY.Type(), variable.Range)
			return newCollectionPostCheckImplicitTypeNode(def.TreeImplicitType, fakeTree, def, nil, variable, nil)
		}

		computedExpressionType := expressionType[0]

		if types.Identical(computedExpressionType, TYPE_ANY.Type()) &&
			localInferences.uniqueVariableInExpression != nil &&
			types.Identical(localInferences.uniqueVariableInExpression.candidateDef.typ, TYPE_ANY.Type()) {

			rhs := localInferences.uniqueVariableInExpression
			exprTree := extractOrInsertTemporaryImplicitTypeFromVariable(rhs.candidateDef, rhs.candidateSymbol)

			// this code below is mandatory for 'insertIterableIntoImplicitTypeNode()' used later to work properly
			computedExpressionType = exprTree.fieldType // very important

			// NOTE:  in case 'exprTree.toDiscard == true', we do not to handle it here
			// this error is handled in 'makeSymboleDefinitionAnalysis()' for '$' variable
		}

		key, val, errMsg := getKeyAndValueTypeFromIterableType(computedExpressionType) // important
		if errMsg != nil {
			err := parser.NewParseError(&lexer.Token{}, errMsg)
			err.Range = node.Value.Range()
			errs = append(errs, err)
		}

		if types.Identical(key, types.Typ[types.Invalid]) && len(node.VariableNames) == 2 {
			err := parser.NewParseError(node.VariableNames[0], fmt.Errorf("'%s' type do not accept key", computedExpressionType))
			errs = append(errs, err)
		}

		var keyDefinition, valueDefinition *VariableDefinition
		var keyToken, valueToken *lexer.Token
		firstPass := true

		for index := len(node.VariableNames) - 1; index >= 0; index-- {
			variable := node.VariableNames[index]
			variableName := string(variable.Value)

			if _, found := localVariables[variableName]; found {
				err := parser.NewParseError(variable, errVariableRedeclaration)
				errs = append(errs, err)
				continue
			}

			def := NewVariableDefinition(variableName, node, parentScope, file.Name())
			def.rng.Start = variable.Range.Start

			localVariables[variableName] = def

			if firstPass {
				def.typ = val
				valueDefinition = def
				valueToken = variable
				firstPass = false
				continue
			}

			def.typ = key
			keyDefinition = def
			keyToken = variable
			file.secondaryVariable = def
		}

		// type inference of the loop expression variable to be completed latter (arr, slice, map, ...)
		rhs := localInferences.uniqueVariableInExpression
		if types.Identical(computedExpressionType, TYPE_ANY.Type()) && rhs != nil &&
			types.Identical(rhs.candidateDef.typ, TYPE_ANY.Type()) { // this condition is to make sure that the tree is still modifiable (type any)

			exprTree := extractOrInsertTemporaryImplicitTypeFromVariable(rhs.candidateDef, rhs.candidateSymbol)
			err := insertIterableIntoImplicitTypeNode(exprTree, keyDefinition, valueDefinition)
			if err != nil {
				err.Token = rhs.candidateSymbol
				err.Range = rhs.candidateSymbol.Range
				errs = append(errs, err)
			}
		} else if types.Identical(computedExpressionType, TYPE_ANY.Type()) { // cannot make type inference
			err := parser.NewParseError(&lexer.Token{}, errDefeatedTypeSystem)
			err.Range = node.Value.Range()
			errs = append(errs, err)
		}

		// solve issue when 'def.TreeImplicitType == nil'
		_ = extractOrInsertTemporaryImplicitTypeFromVariable(valueDefinition, valueToken)

		recheck := createRecheckNode(valueDefinition, valueToken)
		localInferences.uniqueVariableInExpression = recheck                                                             // used by parent scope to assign to '.' var
		localInferences.variablesToRecheckAtEndOfScope = append(localInferences.variablesToRecheckAtEndOfScope, recheck) // enable computation of real type at end of scope

		if keyDefinition != nil { // sizeCreatedVariable == 2
			_ = extractOrInsertTemporaryImplicitTypeFromVariable(keyDefinition, keyToken)
			recheck := createRecheckNode(keyDefinition, node.VariableNames[0])
			localInferences.variablesToRecheckAtEndOfScope = append(localInferences.variablesToRecheckAtEndOfScope, recheck) // enable computation of real type at end of scope
		}

		return expressionType, localInferences, errs
	}

	// else, simple variable declaration
	//
	if len(node.VariableNames) > 1 {
		localErr := parser.NewParseError(node.VariableNames[1], errors.New("only 'range' loop can declare 2 variables at once"))
		errs = append(errs, localErr)
		return invalidTypes, localInferences, errs
	}

	// simple var declaration check (only 1 variable at a time)
	variable := node.VariableNames[0]
	variableName := string(variable.Value)

	if _, found := localVariables[variableName]; found {
		err := parser.NewParseError(variable, errVariableRedeclaration)
		errs = append(errs, err)
		return invalidTypes, localInferences, errs
	}

	def := NewVariableDefinition(variableName, node, parentScope, file.Name())
	def.rng.Start = variable.Range.Start
	def.typ = expressionType[0]

	localVariables[variableName] = def

	// Handle the case when an expression come with an 'inferred type tree'
	// So that at inference type resolution, both variable share the same type
	var recheck *collectionPostCheckImplicitTypeNode = nil

	if types.Identical(def.typ, TYPE_ANY.Type()) {
		rhs := localInferences.uniqueVariableInExpression

		var candidate, constraint *nodeImplicitType
		var candidateSymbol *lexer.Token
		var candidateDef *VariableDefinition

		if rhs == nil ||
			(rhs != nil && types.Identical(rhs.candidateDef.typ, TYPE_ANY.Type()) == false) {
			// this help enforcing that the variable remain 'any_type' at end of scope, otherwise an error will show up

			varName := "$_fake_tree_enforce_any_type"
			exprSymbol := lexer.NewToken(lexer.DOLLAR_VARIABLE, node.Value.Range(), []byte(varName))

			fakeExprDef := NewVariableDefinition(varName, node.Value, parentScope, file.name)
			exprTree := extractOrInsertTemporaryImplicitTypeFromVariable(fakeExprDef, exprSymbol) // this is a candidate node
			varTree := extractOrInsertTemporaryImplicitTypeFromVariable(def, variable)            // this is the constraint, only for this special case

			candidate = exprTree
			constraint = varTree
			candidateSymbol = exprSymbol
			candidateDef = fakeExprDef

		} else if rhs != nil && types.Identical(rhs.candidateDef.typ, TYPE_ANY.Type()) == true {
			exprTree := extractOrInsertTemporaryImplicitTypeFromVariable(rhs.candidateDef, rhs.candidateSymbol) // this is the candidate node
			def.TreeImplicitType = exprTree

			candidate = exprTree
			constraint = exprTree
			candidateSymbol = variable
			candidateDef = def
		}

		recheck = newCollectionPostCheckImplicitTypeNode(candidate, constraint, candidateDef, nil, candidateSymbol, nil)
		localInferences.variablesToRecheckAtEndOfScope = append(localInferences.variablesToRecheckAtEndOfScope, recheck)
	}

	if recheck == nil {
		varName := NAME_TEMPORARY_VAR + strconv.Itoa(parser.GetUniqueNumber())
		fakeTree := newNodeImplicitType(varName, TYPE_ANY.Type(), variable.Range)
		varTree := extractOrInsertTemporaryImplicitTypeFromVariable(def, variable) // this only work bc of assignment rule on 'variable' token
		recheck = newCollectionPostCheckImplicitTypeNode(varTree, fakeTree, def, nil, variable, nil)
	}

	localInferences.uniqueVariableInExpression = recheck

	return expressionType, localInferences, errs
}

func definitionAnalysisVariableAssignment(node *parser.VariableAssignationNode, parent *parser.GroupStatementNode, file *FileDefinition, globalVariables, localVariables map[string]*VariableDefinition) ([2]types.Type, InferenceFoundReturn, []lexer.Error) {
	if parent == nil {
		panic("'variable declaration' cannot be parentless, it shoud be contain in at least 1 one scope")
	} else if node.Kind() != parser.KIND_VARIABLE_ASSIGNMENT {
		panic("found value mismatch for 'VariableAssignationNode.Kind' during DefinitionAnalysis()\n" + node.String())
	} else if globalVariables == nil || localVariables == nil {
		panic("'localVariables' or 'globalVariables' shouldn't be empty for 'VariableAssignationNode.DefinitionAnalysis()'")
	}

	var errs []lexer.Error
	var assignmentType, expressionType [2]types.Type
	var localInferences InferenceFoundReturn
	invalidTypes := [2]types.Type{
		types.Typ[types.Invalid],
		TYPE_ERROR.Type(),
	}

	if node.Err != nil {
		return invalidTypes, localInferences, nil
	}

	if len(node.VariableNames) == 0 && len(node.VariableNames) > 2 {
		panic("cannot analyze variable declaration with 0 or more than 2 variable. this error must be caught and discarded while parsing. node = " + node.String())
	}

	// 0. Check that variable names have proper syntax
	for _, variable := range node.VariableNames {
		if bytes.ContainsAny(variable.Value, ".") {
			err := parser.NewParseError(variable, errors.New("forbidden '.' in variable name while declaring"))
			errs = append(errs, err)
			return invalidTypes, localInferences, errs
		}
	}

	// 1. Check that 'expression' is valid
	if node.Value != nil {
		var localErrs []lexer.Error
		expressionType, localInferences, localErrs = definitionAnalysisMultiExpression(node.Value, parent, file, globalVariables, localVariables)
		errs = append(errs, localErrs...)

	} else {
		errLocal := parser.NewParseError(&lexer.Token{}, errors.New("assignment value cannot be empty"))
		errLocal.Range = node.Range()
		errs = append(errs, errLocal)
		return invalidTypes, localInferences, errs
	}

	// 2. variable within for loop
	if parent.Kind() == parser.KIND_RANGE_LOOP && parent.IsProcessingHeader {
		computedExpressionType := expressionType[0]

		rhs := localInferences.uniqueVariableInExpression
		if types.Identical(computedExpressionType, TYPE_ANY.Type()) && rhs != nil &&
			types.Identical(rhs.candidateDef.typ, TYPE_ANY.Type()) {

			exprTree := extractOrInsertTemporaryImplicitTypeFromVariable(rhs.candidateDef, rhs.candidateSymbol)
			computedExpressionType = exprTree.fieldType
		}

		key, _, errMsg := getKeyAndValueTypeFromIterableType(computedExpressionType) // important
		if errMsg != nil {
			err := parser.NewParseError(&lexer.Token{}, errMsg)
			err.Range = node.Value.Range()
			errs = append(errs, err)
		}

		if types.Identical(key, types.Typ[types.Invalid]) && len(node.VariableNames) == 2 {
			err := parser.NewParseError(node.VariableNames[0], fmt.Errorf("'%s' type do not accept key", computedExpressionType))
			errs = append(errs, err)
		}

		var keyDefinition, valueDefinition *VariableDefinition
		var keyToken, valueToken *lexer.Token
		firstPass := true

		for index := len(node.VariableNames) - 1; index >= 0; index-- {
			variable := node.VariableNames[index]

			def, err := getVariableDefinitionForRootField(variable, localVariables, globalVariables)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			if firstPass {
				firstPass = false
				valueToken = variable

				// the only reason why I cloned 'def' was to provide a better 'go-to-definition' within the 'range' loop only
				// a) share the same 'def.TreeImplicitType' pointer, so the update in one will affect the other
				// b) since 'def.typ' is known at declaration, it will not change till the end analysis. So assuming both definition have the same type is safe
				valueDefinition = cloneVariableDefinition(def)
				valueDefinition.rng = variable.Range
				continue
			}

			keyToken = variable
			keyDefinition = def
			file.secondaryVariable = def
		}

		// rhs := localInferences.uniqueVariableInExpression
		if rhs == nil {
			if types.Identical(computedExpressionType, TYPE_ANY.Type()) && rhs == nil {
				err := parser.NewParseError(&lexer.Token{}, errDefeatedTypeSystem)
				err.Range = node.Value.Range()
				errs = append(errs, err)
			}

			varFakeName := NAME_TEMPORARY_VAR + strconv.Itoa(parser.GetUniqueNumber())
			exprSymbol := lexer.NewToken(lexer.DOLLAR_VARIABLE, node.Range(), []byte("$_fake_expr_variable"))
			exprDef := NewVariableDefinition(varFakeName, node.Value, parent, file.FileName())
			exprDef.typ = computedExpressionType
			exprTree := extractOrInsertTemporaryImplicitTypeFromVariable(exprDef, exprSymbol)
			rhs = newCollectionPostCheckImplicitTypeNode(exprTree, exprTree, exprDef, nil, exprSymbol, nil)
		}

		// setup phase for 'keyDefinition' and 'valueDefinition' evaluation
		exprTree := rhs.candidate
		exprDef := rhs.candidateDef
		exprSymbol := rhs.candidateSymbol

		// Post recheck 'key' var and 'expr' iterable key type, when available
		if keyDefinition != nil {
			keyTree := extractOrInsertTemporaryImplicitTypeFromVariable(keyDefinition, keyToken)
			recheck := newCollectionPostCheckImplicitTypeNode(exprTree, keyTree, exprDef, keyDefinition, exprSymbol, keyToken)
			recheck.operation = OPERATOR_KEY_ITERABLE_TYPE
			localInferences.variablesToRecheckAtEndOfScope = append(localInferences.variablesToRecheckAtEndOfScope, recheck)
		}

		// Post recheck 'value' var and 'expr' iterable value type
		if valueDefinition != nil {
			valueTree := extractOrInsertTemporaryImplicitTypeFromVariable(valueDefinition, valueToken)
			recheck := newCollectionPostCheckImplicitTypeNode(exprTree, valueTree, exprDef, valueDefinition, exprSymbol, valueToken)
			recheck.operation = OPERATOR_VALUE_ITERABLE_TYPE
			localInferences.variablesToRecheckAtEndOfScope = append(localInferences.variablesToRecheckAtEndOfScope, recheck)
			localInferences.uniqueVariableInExpression = recheck // used by parent scope to assign to '.' var
		}

		return expressionType, localInferences, errs
	}

	// 3. only one variable, and not within for loop
	if len(node.VariableNames) > 1 {
		localErr := parser.NewParseError(node.VariableNames[1], errors.New("only 'range' loop can declare 2 variables at once"))
		errs = append(errs, localErr)
		return invalidTypes, localInferences, errs
	}

	variable := node.VariableNames[0]
	def, err := getVariableDefinitionForRootField(variable, localVariables, globalVariables)
	if err != nil {
		errs = append(errs, err)
		return invalidTypes, localInferences, errs
	}

	assignmentType[0] = def.typ
	assignmentType[1] = expressionType[1]

	// Whenever implicit type are found for either 'var' or 'expr'
	// simply, recheck the type later at end of root scope
	rhs := localInferences.uniqueVariableInExpression
	if types.Identical(expressionType[0], TYPE_ANY.Type()) && rhs != nil ||
		types.Identical(def.typ, TYPE_ANY.Type()) && def.TreeImplicitType != nil {

		varDef := def
		varTree := extractOrInsertTemporaryImplicitTypeFromVariable(varDef, variable)

		if rhs == nil {
			varName := NAME_TEMPORARY_VAR + strconv.Itoa(parser.GetUniqueNumber())
			exprSymbol := lexer.NewToken(lexer.DOLLAR_VARIABLE, node.Value.Range(), []byte(varName))
			exprDef := NewVariableDefinition(string(exprSymbol.Value), node, parent, file.FileName())
			exprDef.typ = expressionType[0]
			exprTree := extractOrInsertTemporaryImplicitTypeFromVariable(exprDef, exprSymbol)
			rhs = newCollectionPostCheckImplicitTypeNode(exprTree, exprTree, exprDef, nil, exprSymbol, nil)
		}

		exprTree := rhs.candidate
		exprSymbol := rhs.candidateSymbol
		exprDef := rhs.candidateDef

		recheck := newCollectionPostCheckImplicitTypeNode(exprTree, varTree, exprDef, varDef, exprSymbol, variable)
		recheck.isAssignmentNode = true
		localInferences.variablesToRecheckAtEndOfScope = append(localInferences.variablesToRecheckAtEndOfScope, recheck)
		localInferences.uniqueVariableInExpression = recheck

		return assignmentType, localInferences, errs
	}

	varTree := extractOrInsertTemporaryImplicitTypeFromVariable(def, variable) // this only work bc of assignment rule
	varName := NAME_TEMPORARY_VAR + strconv.Itoa(parser.GetUniqueNumber())
	fakeTree := newNodeImplicitType(varName, TYPE_ANY.Type(), variable.Range)

	recheck := newCollectionPostCheckImplicitTypeNode(varTree, fakeTree, def, nil, variable, nil)
	localInferences.uniqueVariableInExpression = recheck

	_, localErr := TypeCheckAgainstConstraint(expressionType[0], def.typ)
	if localErr != nil {
		errMsg := fmt.Errorf("%w, between var '%s' and expr '%s'", errTypeMismatch, def.Type(), expressionType[0])
		err := parser.NewParseError(variable, errMsg)
		errs = append(errs, err)
	}

	return assignmentType, localInferences, errs
}

func definitionAnalysisMultiExpression(node *parser.MultiExpressionNode, parent *parser.GroupStatementNode, file *FileDefinition, globalVariables, localVariables map[string]*VariableDefinition) ([2]types.Type, InferenceFoundReturn, []lexer.Error) {
	if node.Kind() != parser.KIND_MULTI_EXPRESSION {
		panic("found value mismatch for 'MultiExpressionNode.Kind' during DefinitionAnalysis()\n" + node.String())
	}

	var errs, localErrs []lexer.Error
	var inferences, localInferences InferenceFoundReturn
	expressionType := [2]types.Type{
		types.Typ[types.Invalid],
		TYPE_ERROR.Type(),
	}

	if node.Err != nil {
		return expressionType, localInferences, nil
	}

	for count, expression := range node.Expressions {
		if expression == nil {
			log.Printf("fatal, nil element within expression list for MultiExpressionNode. \n %s \n", node.String())
			panic("element within expression list cannot be 'nil' for MultiExpressionNode. instead of inserting the nil value, omit it")
		}

		// normal processing when this is the first expression
		if count == 0 {
			expressionType, localInferences, localErrs = definitionAnalysisExpression(expression, parent, file, globalVariables, localVariables)

			errs = append(errs, localErrs...)
			inferences = localInferences
			continue
		}

		// when piping, you pass the result of the previous expression to the end position of the current expression
		//
		// create a token group and insert it to the end of the expression
		groupName := NAME_TEMPORARY_VAR + "_GROUP_" + strconv.Itoa(parser.GetUniqueNumber())
		tokenGroup := &lexer.Token{ID: lexer.STATIC_GROUP, Value: []byte(groupName), Range: expression.Range()}
		expression.Symbols = append(expression.Symbols, tokenGroup)

		// then insert that token as variable within the file
		def := NewVariableDefinition(groupName, nil, parent, file.Name())
		def.rng = tokenGroup.Range

		def.typ = expressionType[0]
		if localInferences.uniqueVariableInExpression != nil {
			def.TreeImplicitType = localInferences.uniqueVariableInExpression.candidate
		}

		localVariables[def.Name()] = def
		expressionType, localInferences, localErrs = definitionAnalysisExpression(expression, parent, file, globalVariables, localVariables)

		errs = append(errs, localErrs...)
		inferences.uniqueVariableInExpression = localInferences.uniqueVariableInExpression
		inferences.variablesToRecheckAtEndOfScope = append(inferences.variablesToRecheckAtEndOfScope, localInferences.variablesToRecheckAtEndOfScope...)

		// once, processing is over, remove the group created from the expression
		size := len(expression.Symbols)
		expression.Symbols = expression.Symbols[:size-1]

		delete(localVariables, def.Name())
	}

	return expressionType, inferences, errs
}

func definitionAnalysisExpression(node *parser.ExpressionNode, parent *parser.GroupStatementNode, file *FileDefinition, globalVariables, localVariables map[string]*VariableDefinition) ([2]types.Type, InferenceFoundReturn, []lexer.Error) {
	if node.Kind() != parser.KIND_EXPRESSION {
		panic("found value mismatch for 'ExpressionNode.Kind'. expected 'KIND_EXPRESSION' instead. current node \n" + node.String())
	}

	if globalVariables == nil || localVariables == nil {
		panic("'globalVariables' or 'localVariables' or shouldn't be empty for 'ExpressionNode.DefinitionAnalysis()'")
	}

	var expressionType [2]types.Type = [2]types.Type{types.Typ[types.Invalid], TYPE_ERROR.Type()}
	var errs []lexer.Error
	inferences := InferenceFoundReturn{}

	if node.Err != nil {
		return expressionType, inferences, nil
	}

	if len(node.Symbols) == 0 {
		err := parser.NewParseError(&lexer.Token{}, errEmptyExpression)
		err.Range = node.Range()
		errs = append(errs, err)
		return expressionType, inferences, errs
	}

	defChecker := NewDefinitionAnalyzer(node.Symbols, node.ExpandedTokens, parent, file, node.Range())
	expressionType, inferences, errs = defChecker.makeSymboleDefinitionAnalysis(localVariables, globalVariables)

	if expressionType[0] == nil {
		log.Printf("found a <nil> return type for expression"+"\n file = %#v\n node = %#v\n inferences = %#v\n", file, node, inferences)
		panic("found a <nil> return type for expression")
	}

	return expressionType, inferences, errs
}

// first make definition analysis to find all existing reference
// then make the type analysis
type definitionAnalyzer struct {
	symbols         []*lexer.Token
	expandedTokens  []parser.AstNode
	index           int // current index of symbols within the expression
	isEOF           bool
	parent          *parser.GroupStatementNode
	file            *FileDefinition
	rangeExpression lexer.Range
}

func NewDefinitionAnalyzer(symbols []*lexer.Token, expandedGroups []parser.AstNode, parent *parser.GroupStatementNode, file *FileDefinition, rangeExpr lexer.Range) *definitionAnalyzer {
	ana := &definitionAnalyzer{
		symbols:         symbols,
		index:           0,
		parent:          parent,
		file:            file,
		rangeExpression: rangeExpr,
		expandedTokens:  expandedGroups,
	}

	return ana
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

// fetch all tokens and sort them
func (p *definitionAnalyzer) makeSymboleDefinitionAnalysis(localVariables, globalVariables map[string]*VariableDefinition) ([2]types.Type, InferenceFoundReturn, []lexer.Error) {
	var errs []lexer.Error
	var err *parser.ParseError
	lateVariableRecheck := InferenceFoundReturn{}

	processedToken := []*lexer.Token{}
	processedTypes := []types.Type{}

	makeTypeInference := func(symbol *lexer.Token, symbolType, constraintType types.Type) (*collectionPostCheckImplicitTypeNode, *parser.ParseError) {
		return makeTypeInferenceWhenPossible(symbol, symbolType, constraintType, localVariables, globalVariables)
	}

	var lastVarSymbol *lexer.Token
	var symbolType types.Type
	var varDef *VariableDefinition

	count := 0

	for p.isTokenAvailable() {
		if count > 100 {
			log.Printf("loop took too long to complete.\n analyzer = %s\n", p)
			panic("loop lasted more than expected on 'expression definition analysis'")
		}

		count++
		symbol := p.peek()

		{ // temporary scope to avoid namespace polution
			// the goal of this is to make 'key' value of the loop capable of type inference (for ease of use by the human user)
			symbolName := string(symbol.Value)
			def := p.file.extraVariableNameWithTypeInferenceBehavior[symbolName]

			if def != nil && symbol.ID == lexer.DOLLAR_VARIABLE {
				foundDef, _ := getVariableDefinitionForRootField(symbol, localVariables, globalVariables)

				if def == foundDef {
					symbol = lexer.CloneToken(symbol)
					symbol.ID = lexer.DOT_VARIABLE
				}
			}
		}

		switch symbol.ID {
		case lexer.STRING:
			processedToken = append(processedToken, symbol)
			processedTypes = append(processedTypes, mustGetBasicTypeFromTokenID(symbol.ID)) // String

			p.nextToken()
		case lexer.CHARACTER:
			processedToken = append(processedToken, symbol)
			processedTypes = append(processedTypes, mustGetBasicTypeFromTokenID(symbol.ID)) // Rune

			p.nextToken()
		case lexer.NUMBER:
			processedToken = append(processedToken, symbol)
			processedTypes = append(processedTypes, mustGetBasicTypeFromTokenID(symbol.ID)) // Int

			p.nextToken()
		case lexer.DECIMAL:
			processedToken = append(processedToken, symbol)
			processedTypes = append(processedTypes, mustGetBasicTypeFromTokenID(symbol.ID)) // Float64

			p.nextToken()
		case lexer.COMPLEX_NUMBER:
			processedToken = append(processedToken, symbol)
			processedTypes = append(processedTypes, mustGetBasicTypeFromTokenID(symbol.ID)) // Complex64

			p.nextToken()
		case lexer.BOOLEAN:
			processedToken = append(processedToken, symbol)
			processedTypes = append(processedTypes, mustGetBasicTypeFromTokenID(symbol.ID)) // Bool

			p.nextToken()
		case lexer.FUNCTION:

			var fakeVarDef *VariableDefinition = nil
			fields, _, _, err := splitVariableNameFields(symbol)

			functionName := fields[0]
			def := p.file.functions[functionName]

			if def == nil {
				symbolType = types.Typ[types.Invalid]
				err = parser.NewParseError(symbol, errFunctionUndefined)
				err.Range.End.Character = err.Range.Start.Character + len(functionName)
				errs = append(errs, err)
			} else {
				fakeVarDef = NewVariableDefinition(def.name, def.node, p.parent, def.fileName)
				fakeVarDef.typ = def.typ

				symbolType, err = getRealTypeAssociatedToVariable(symbol, fakeVarDef)
				if err != nil {
					errs = append(errs, err)
				}
			}

			processedToken = append(processedToken, symbol)
			processedTypes = append(processedTypes, symbolType)

			p.nextToken()

		case lexer.EXPANDABLE_GROUP:

			node := p.expandedTokens[p.index]
			if node == nil {
				panic("no associated AST found for 'expanded_token' " + symbol.String())
			}

			typs, inferences, localErrs := definitionAnalysisRecursive(node, p.parent, p.file, globalVariables, localVariables)
			errs = append(errs, localErrs...)

			// symbol = lexer.CloneToken(symbol)
			fields, _, _, err := splitVariableNameFields(symbol)
			if err != nil { // also an error when len(fields) == 0
				panic("malformated symbol name for 'expanded_token'. " + err.GetError())
			}

			varName := fields[0]
			varDef = NewVariableDefinition(varName, node, p.parent, p.file.name)
			varDef.typ = typs[0]

			localVariables[varName] = varDef

			if types.Identical(typs[0], TYPE_ANY.Type()) && inferences.uniqueVariableInExpression != nil {
				rhs := inferences.uniqueVariableInExpression
				varDef.TreeImplicitType = rhs.candidate
			}

			fallthrough

		case lexer.DOLLAR_VARIABLE, lexer.STATIC_GROUP:
			lastVarSymbol = symbol
			symbolType = TYPE_ANY.Type()

			varDef, err = getVariableDefinitionForRootField(symbol, localVariables, globalVariables)

			if err != nil {
				errs = append(errs, err)
				symbolType = types.Typ[types.Invalid]

			} else if types.Identical(varDef.typ, TYPE_ANY.Type()) == false {
				symbolType, err = getRealTypeAssociatedToVariable(symbol, varDef)
				if err != nil {
					errs = append(errs, err)
				}

				// NOTE: this is essential to test if a field **exist** when using '$' variable
				// type check will always pass, but this help in verifying that 'varTree.toDiscard != true' and type syst. defeated
			} else if types.Identical(varDef.typ, TYPE_ANY.Type()) == true {
				varTree := extractOrInsertTemporaryImplicitTypeFromVariable(varDef, symbol)
				varFakeName := NAME_TEMPORARY_VAR + strconv.Itoa(parser.GetUniqueNumber())
				fakeTree := newNodeImplicitType(varFakeName, TYPE_ANY.Type(), symbol.Range) // read note above

				recheck := newCollectionPostCheckImplicitTypeNode(varTree, fakeTree, varDef, nil, symbol, nil)
				lateVariableRecheck.variablesToRecheckAtEndOfScope = append(lateVariableRecheck.variablesToRecheckAtEndOfScope, recheck)
				symbolType = TYPE_ANY.Type() // this help pushback processing in the next step
			}

			/*
				symbolType, err = getRealTypeAssociatedToVariable(symbol, varDef)
				if err != nil && !errors.Is(err.Err, errDefeatedTypeSystem) {
					errs = append(errs, err)
				}
			*/

			markVariableAsUsed(varDef)

			processedToken = append(processedToken, symbol)
			processedTypes = append(processedTypes, symbolType)

			p.nextToken()

		case lexer.DOT_VARIABLE:

			lastVarSymbol = symbol
			symbolType = TYPE_ANY.Type()

			varDef, err = getVariableDefinitionForRootField(symbol, localVariables, globalVariables)

			if err != nil {
				errs = append(errs, err)
				symbolType = types.Typ[types.Invalid]

			} else if types.Identical(varDef.typ, TYPE_ANY.Type()) == false {
				symbolType, err = getRealTypeAssociatedToVariable(symbol, varDef)
				if err != nil {
					errs = append(errs, err)
				}

			} else if types.Identical(varDef.typ, TYPE_ANY.Type()) == true {
				symbolType, err = updateVariableImplicitType(varDef, symbol, symbolType)
				if err != nil {
					errs = append(errs, err)
				}
			}

			/*
				symbolType, err = getRealTypeAssociatedToVariable(symbol, varDef)
				if err != nil && !errors.Is(err.Err, errDefeatedTypeSystem) {
					errs = append(errs, err)
				}
			*/

			markVariableAsUsed(varDef)

			processedToken = append(processedToken, symbol)
			processedTypes = append(processedTypes, symbolType)

			p.nextToken()

		default: // LEFT_PAREN, RIGTH_PAREN, etc.
			panic("unexpected token type during 'symbol analysis' on Expression. " + symbol.String())
		}
	}

	// Only necessary to pass around single symbol expression to other node
	if count == 1 && varDef != nil && lastVarSymbol != nil {
		varFakeName := NAME_TEMPORARY_VAR + strconv.Itoa(parser.GetUniqueNumber())
		fakeTree := newNodeImplicitType(varFakeName, symbolType, lastVarSymbol.Range)
		varTree := extractOrInsertTemporaryImplicitTypeFromVariable(varDef, lastVarSymbol)

		recheck := newCollectionPostCheckImplicitTypeNode(varTree, fakeTree, varDef, nil, lastVarSymbol, nil)
		lateVariableRecheck.uniqueVariableInExpression = recheck
	}

	groupType, variablesToRecheck, localErrs := makeExpressionTypeCheck(processedToken, processedTypes, makeTypeInference, p.rangeExpression)
	for _, err := range localErrs {
		errs = append(errs, err)
	}

	lateVariableRecheck.variablesToRecheckAtEndOfScope = append(lateVariableRecheck.variablesToRecheckAtEndOfScope, variablesToRecheck...)

	return groupType, lateVariableRecheck, errs
}

func makeTypeInferenceWhenPossible(symbol *lexer.Token, symbolType, constraintType types.Type, localVariables, globalVariables map[string]*VariableDefinition) (recheck *collectionPostCheckImplicitTypeNode, err *parser.ParseError) {

	if types.Identical(constraintType, TYPE_ANY.Type()) { // no type inference to make here
		return nil, nil
	}

	// This shouldn't be executed, since 'makeTypeInference()' (this function) is only called
	// by 'makeTypeCheckOnSymbolForFunction()' whenever the argument type is == any
	// this 'symbolType' of this function == any
	if !types.Identical(symbolType, TYPE_ANY.Type()) { // if type != ANY_TYPE
		// TODO: remove the code below and panic instead ????
		_, errMsg := TypeCheckAgainstConstraint(symbolType, constraintType)
		if errMsg != nil {
			err := parser.NewParseError(symbol, errMsg)

			return nil, err
		}

		return nil, nil
	}

	switch symbol.ID {
	case lexer.DOLLAR_VARIABLE, lexer.DOT_VARIABLE:
		// do nothing, and postpone processing
	default:
		_, errMsg := TypeCheckAgainstConstraint(symbolType, constraintType)
		if errMsg != nil {
			err := parser.NewParseError(symbol, errMsg)

			return nil, err
		}

		return nil, nil
	}

	// If we reach here, that mean one thing
	// symbolType == ANY_TYPE && constraintType != ANY_TYPE
	//
	// In that case, do type inference implicitly, if '.' var but disallow '$' var

	varDef, err := getVariableDefinitionForRootField(symbol, localVariables, globalVariables)
	if err != nil {
		return nil, err
	}

	if symbol.ID == lexer.DOLLAR_VARIABLE { // ID = '.' var
		varFakeName := "$_fake_root"
		constraintTypeTree := newNodeImplicitType(varFakeName, constraintType, symbol.Range)
		candidateTypeTree := extractOrInsertTemporaryImplicitTypeFromVariable(varDef, symbol)

		recheck := newCollectionPostCheckImplicitTypeNode(candidateTypeTree, constraintTypeTree, varDef, nil, symbol, nil)

		return recheck, nil
	}

	// else 'lexer.DOT_VARIABLE'
	symbolType, err = updateVariableImplicitType(varDef, symbol, constraintType)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

var errEmptyVariableName error = errors.New("empty variable name")
var errVariableEndWithDot error = errors.New("variable cannot end with '.'")
var errVariableWithConsecutiveDot error = errors.New("consecutive '.' found in variable name")
var errVariableUndefined error = errors.New("variable undefined")
var errVariableRedeclaration error = errors.New("variable redeclaration")
var errFieldNotFound error = errors.New("field or method not found")
var errMethodNotInEndPosition error = errors.New("method call can only be at the end")
var errMapDontHaveChildren error = errors.New("map cannot have children")
var errSliceDontHaveChildren error = errors.New("slice cannot have children")
var errArrayDontHaveChildren error = errors.New("array cannot have children")
var errChannelDontHaveChildren error = errors.New("channel cannot have children")

// split the variable using '.' as a separator
// the returned locations are computed from the front (for the first one) and from the back (for the second)
// this is done since some variable path (namely expanded token) require computing from the back to remain accurate
func splitVariableNameFields(variable *lexer.Token) ([]string, []int, []int, *parser.ParseError) {
	if variable == nil {
		panic("cannot split variable fields from <nil> token")
	}

	var err *parser.ParseError
	var index, characterCount, counter int

	var fields []string
	var fieldsLocalPosition []int
	var fieldLocalPositionsComputedFromBack []int

	variableName := string(variable.Value)
	LengthVariableName := len(variableName)

	if variableName == "" {
		return nil, nil, nil, parser.NewParseError(variable, errEmptyVariableName)
	}

	if variableName[0] == '.' { // Handle the case of a 'DOT_VARIABLE', which start with '.'
		fields = append(fields, ".")
		fieldsLocalPosition = append(fieldsLocalPosition, characterCount)
		fieldLocalPositionsComputedFromBack = append(fieldLocalPositionsComputedFromBack, LengthVariableName-1-characterCount)

		characterCount++

		variableName = variableName[1:]

		if variableName == "" {
			return fields, fieldsLocalPosition, fieldLocalPositionsComputedFromBack, nil
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

				return fields, fieldsLocalPosition, fieldLocalPositionsComputedFromBack, err
			}

			index = len(variableName)
		} else if index == 0 {
			err = parser.NewParseError(variable, errors.New("consecutive '.' found in variable name"))

			err.Range.Start.Character += characterCount
			err.Range.End = err.Range.Start
			err.Range.End.Character += 1

			return fields, fieldsLocalPosition, fieldLocalPositionsComputedFromBack, err
		}

		fields = append(fields, variableName[:index])
		fieldsLocalPosition = append(fieldsLocalPosition, characterCount)
		fieldLocalPositionsComputedFromBack = append(fieldLocalPositionsComputedFromBack, LengthVariableName-1-characterCount)

		characterCount += index + 1

		if index >= len(variableName) {
			break
		}

		variableName = variableName[index+1:]
	}

	if len(fields) == 0 {
		return nil, nil, nil, parser.NewParseError(variable, errEmptyVariableName)
	}

	return fields, fieldsLocalPosition, fieldLocalPositionsComputedFromBack, nil
}

// join string together to obtain a valid variable name (dollar or dot)
// NOTE: if you need to use it to compute the Range,
// I recommend join from the middle to the end (rather than beginning to middle)
// This workaround was introduced because of 'EXPANDABLE_GROUP', the first field name don't always accurately
// represent the true Range of its associated sub-expression
func joinVariableNameFields(fields []string) (string, *parser.ParseError) {
	length := len(fields)
	if length == 0 {
		err := parser.NewParseError(&lexer.Token{}, errEmptyVariableName)
		return "", err
	}

	// Check for malformated 'field' that will compose the variable name
	for i := range length {
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

	if suffix == "" {
		return fields[0], nil
	}

	return fields[0] + "." + suffix, nil
}

// start checking from the back of the slice because starting from the front
// make finding position of element more complicated for 'lexer.EXPANDABLE_GROUP'
// func findFieldContainingRange(fields []string, pos lexer.Position) int {
func findFieldContainingRange(fieldPosCountedFromBack []int, pos lexer.Position) int {
	if pos.Line != 0 {
		return 0
	}

	if pos.Character < 0 {
		panic("to search field position within token, a positive relative position of the cursor is mandatory")
	}

	size := len(fieldPosCountedFromBack)

	for index := size - 1; index >= 0; index-- {
		fieldLocation := fieldPosCountedFromBack[index]

		if fieldLocation >= pos.Character {
			return index
		}
	}

	return 0
}

// compute the real type of variable, ignore the inference tree of type
// meaning, computation of 'varDef.typ' rather than 'varDef.TreeImplicitType'
// 'getDeclaredTypeAssociatedToVariable()', which is different from 'inferredType'
func getRealTypeAssociatedToVariable(variable *lexer.Token, varDef *VariableDefinition) (types.Type, *parser.ParseError) {
	invalidType := types.Typ[types.Invalid]

	// 1. Extract names/fields from variable (separated by '.')
	fields, fieldsLocalPosition, _, err := splitVariableNameFields(variable)
	if err != nil {
		return invalidType, err
	}

	if varDef == nil {
		err := parser.NewParseError(variable, errVariableUndefined)
		err.Range.End.Character = err.Range.Start.Character + len(fields[0])
		return invalidType, err
	}

	parentType := varDef.typ
	if parentType == nil {
		return TYPE_ANY.Type(), nil

	} else if types.Identical(parentType, TYPE_ANY.Type()) && len(fields) > 1 {
		err := parser.NewParseError(variable, errDefeatedTypeSystem)
		return TYPE_ANY.Type(), err

	} else if types.Identical(parentType, TYPE_ANY.Type()) {
		return TYPE_ANY.Type(), nil
	}

	// 2. Now go down the struct to find out the final variable or method
	if len(fields) == 0 {
		err := parser.NewParseError(variable, errEmptyVariableName)
		return invalidType, err
	} else if len(fields) == 1 {
		return parentType, nil
	}

	var count int
	var fieldName string
	var fieldPos int
	var sizevariable int = len(string(variable.Value))

	// parentType ==> type for i = 0 (at start)
	for i := 1; i < len(fields); i++ {
		count++
		if count > 100 {
			log.Printf("infinite loop detected while analyzing fields.\n fields = %q", fields)
			panic("infinite loop detected while analyzing fields")
		}

		fieldName = fields[i]
		fieldPos = fieldsLocalPosition[i]
		fieldPosCountedFromBack := (sizevariable) - fieldPos

		// Always check the parent type but always return the last field
		// of the variable without a check
		switch t := parentType.(type) {
		default:
			log.Printf("parentType = %#v \n reflec.TypeOf(parentType) = %s\n"+
				" fields.index = %d ::: fields = %q",
				parentType, reflect.TypeOf(parentType), i, fields,
			)
			panic("parentType not recognized")

		case *types.Basic:
			errBasic := fmt.Errorf("%w, '%s' cannot accept field", errTypeMismatch, t.String())
			err = parser.NewParseError(variable, errBasic)
			err.Range.Start.Character = variable.Range.End.Character - fieldPosCountedFromBack
			err.Range.End.Character = err.Range.Start.Character + len(fieldName)
			err.Range.Start.Line = err.Range.End.Line

			return invalidType, err

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
				err.Range.Start.Character = variable.Range.End.Character - fieldPosCountedFromBack
				err.Range.End.Character = err.Range.Start.Character + len(fieldName)
				err.Range.Start.Line = err.Range.End.Line

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

			parentType = nil

			for index := range t.NumMethods() {
				field := t.Method(index)

				if field.Name() != fieldName {
					continue
				}

				parentType = field.Type()

				break
			}

			// handle the case where the type is the empty interface 'interface{}'
			if types.Identical(t, TYPE_ANY.Type()) { // parentType == nil && t.NumMethods() == 0
				err = parser.NewParseError(variable, errDefeatedTypeSystem)
				err.Range.Start.Character = variable.Range.End.Character - fieldPosCountedFromBack
				err.Range.End.Character = err.Range.Start.Character + len(fieldName)
				err.Range.Start.Line = err.Range.End.Line

				return TYPE_ANY.Type(), err
			}

			if parentType == nil {
				err = parser.NewParseError(variable, errFieldNotFound)
				err.Range.Start.Character = variable.Range.End.Character - fieldPosCountedFromBack
				err.Range.End.Character = err.Range.Start.Character + len(fieldName)
				err.Range.Start.Line = err.Range.End.Line

				return invalidType, err
			}

			continue

		case *types.Signature:
			if t.Params().Len() != 0 {
				err = parser.NewParseError(variable, fmt.Errorf("%w, chained function must be parameterless", errFunctionParameterSizeMismatch))
				err.Range.Start.Character = variable.Range.End.Character - fieldPosCountedFromBack
				err.Range.End.Character = err.Range.Start.Character + len(fieldName)
				err.Range.Start.Line = err.Range.End.Line

				return invalidType, err
			}

			functionResult := t.Results()

			if functionResult == nil {
				err = parser.NewParseError(variable, errFunctionVoidReturn)
				err.Range.Start.Character = variable.Range.End.Character - fieldPosCountedFromBack
				err.Range.End.Character = err.Range.Start.Character + len(fieldName)
				err.Range.Start.Line = err.Range.End.Line

				return invalidType, err
			}

			if functionResult.Len() > 2 {
				err = parser.NewParseError(variable, errFunctionMaxReturn)
				err.Range.Start.Character = variable.Range.End.Character - fieldPosCountedFromBack
				err.Range.End.Character = err.Range.Start.Character + len(fieldName)
				err.Range.Start.Line = err.Range.End.Line

				return invalidType, err
			}

			if functionResult.Len() == 2 && !types.Identical(functionResult.At(1).Type(), TYPE_ERROR.Type()) {
				err = parser.NewParseError(variable, errFunctionSecondReturnNotError)
				err.Range.Start.Character = variable.Range.End.Character - fieldPosCountedFromBack
				err.Range.End.Character = err.Range.Start.Character + len(fieldName)
				err.Range.Start.Line = err.Range.End.Line

				return invalidType, err
			}

			i--
			parentType = functionResult.At(0).Type()

			continue

		case *types.Map:
			err = parser.NewParseError(variable, errMapDontHaveChildren)
			err.Range.Start.Character = variable.Range.End.Character - fieldPosCountedFromBack
			err.Range.End.Character = err.Range.Start.Character + len(fieldName)
			err.Range.Start.Line = err.Range.End.Line

			return invalidType, err

		case *types.Chan:
			err = parser.NewParseError(variable, errChannelDontHaveChildren)
			err.Range.Start.Character = variable.Range.End.Character - fieldPosCountedFromBack
			err.Range.End.Character = err.Range.Start.Character + len(fieldName)
			err.Range.Start.Line = err.Range.End.Line

			return invalidType, err

		case *types.Array:
			err = parser.NewParseError(variable, errArrayDontHaveChildren)
			err.Range.Start.Character = variable.Range.End.Character - fieldPosCountedFromBack
			err.Range.End.Character = err.Range.Start.Character + len(fieldName)
			err.Range.Start.Line = err.Range.End.Line

			return invalidType, err

		case *types.Slice:
			err = parser.NewParseError(variable, errSliceDontHaveChildren)
			err.Range.Start.Character = variable.Range.End.Character - fieldPosCountedFromBack
			err.Range.End.Character = err.Range.Start.Character + len(fieldName)
			err.Range.Start.Line = err.Range.End.Line

			return invalidType, err
			// array, slice, pointer, channel, map, nil, typeParams, Named, Union, tuple, signature, func
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
		return [2]types.Type{types.Typ[types.Invalid], TYPE_ERROR.Type()}
	}

	tuple, ok := typ.(*types.Tuple)
	if !ok {
		return [2]types.Type{typ, nil}
	}

	if tuple.Len() == 0 {
		return [2]types.Type{types.Typ[types.Invalid], TYPE_ERROR.Type()}
	} else if tuple.Len() == 1 {
		return [2]types.Type{tuple.At(0).Type(), nil}
	} else if tuple.Len() == 2 {
		return [2]types.Type{tuple.At(0).Type(), tuple.At(1).Type()}
	}

	return [2]types.Type{types.Typ[types.Invalid], TYPE_ERROR.Type()}
}

var errTemplateUndefined error = errors.New("template undefined")
var errEmptyExpression error = errors.New("empty expression")
var errArgumentsOnlyForFunction error = errors.New("only function and method accepts arguments")
var errFunctionUndefined error = errors.New("function undefined")
var errFunctionParameterSizeMismatch error = errors.New("function 'parameter' and 'argument' size mismatch")
var errFunctionNotEnoughArguments error = errors.New("not enough arguments for function")
var errFunctionMaxReturn error = errors.New("function cannot return more than 2 values")
var errFunctionVoidReturn error = errors.New("function cannot have 'void' return value")
var errFunctionSecondReturnNotError error = errors.New("function's second return value must be an 'error' only")
var errTypeMismatch error = errors.New("type mismatch")
var errVariableNotUsed error = errors.New("variable is never used")
var errDefeatedTypeSystem error = errors.New("type system defeated")

func makeExpressionTypeCheck(symbols []*lexer.Token, typs []types.Type, makeTypeInference InferenceFunc, nodeRange lexer.Range) (resultType [2]types.Type, variablesToRecheck []*collectionPostCheckImplicitTypeNode, errs []*parser.ParseError) {
	if len(symbols) != len(typs) {
		log.Printf("every symbol should must have a single type."+
			"\n symbols = %q\n typs = %q", symbols, typs)
		panic("every symbol should must have a single type")
	}

	// 1. len(symbols) == 0 && len == 1
	if len(symbols) == 0 {
		err := parser.NewParseError(&lexer.Token{}, errEmptyExpression)
		err.Range = nodeRange
		errs = append(errs, err)

		return unTuple(types.Typ[types.Invalid]), nil, errs
	} else if len(symbols) == 1 {
		symbol := symbols[0]
		typ := typs[0]

		funcType, ok := typ.(*types.Signature)
		if ok {
			returnType, _, localErrs := makeFunctionTypeCheck(funcType, symbol, typs[1:], symbols[1:], makeTypeInference)
			errs = append(errs, localErrs...)

			return unTuple(returnType), nil, errs
		}

		return unTuple(typ), nil, nil
	}

	// 2. len(symbols) >= 2 :: Always true if this section is reached
	funcType, ok := typs[0].(*types.Signature)
	if !ok {
		err := parser.NewParseError(symbols[0], errArgumentsOnlyForFunction)
		errs = append(errs, err)

		return unTuple(types.Typ[types.Invalid]), nil, errs
	}

	returnType, rechecks, localErrs := makeFunctionTypeCheck(funcType, symbols[0], typs[1:], symbols[1:], makeTypeInference)

	errs = append(errs, localErrs...)
	variablesToRecheck = append(variablesToRecheck, rechecks...)

	return unTuple(returnType), variablesToRecheck, errs
}

func makeFunctionTypeCheck(funcType *types.Signature, funcSymbol *lexer.Token, argTypes []types.Type, argSymbols []*lexer.Token, makeTypeInference InferenceFunc) (resultType types.Type, variablesToRecheck []*collectionPostCheckImplicitTypeNode, errs []*parser.ParseError) {
	if funcType == nil {
		err := parser.NewParseError(funcSymbol, errFunctionUndefined)
		errs = append(errs, err)
		return types.Typ[types.Invalid], nil, errs
	}

	// 1. Check Parameter VS Argument validity
	invalidReturnType := types.Typ[types.Invalid]

	paramSize := funcType.Params().Len()
	argumentSize := len(argSymbols)
	isVariadicFunction := funcType.Variadic()

	if isVariadicFunction == false && paramSize != argumentSize {
		err := parser.NewParseError(funcSymbol, errFunctionParameterSizeMismatch)
		errs = append(errs, err)
		return invalidReturnType, nil, errs

	} else if isVariadicFunction == true && argumentSize < paramSize {
		err := parser.NewParseError(funcSymbol, errFunctionNotEnoughArguments)
		errs = append(errs, err)
		return invalidReturnType, nil, errs
	}

	lastParamIndex := paramSize - 1
	var paramType, argumentType types.Type

	for i := range len(argSymbols) {

		if isVariadicFunction && i >= lastParamIndex {
			sliceParam, ok := funcType.Params().At(lastParamIndex).Type().(*types.Slice)
			if !ok {
				panic("expected variadic function with last param being a slice but didn't find the slice")
			}
			paramType = sliceParam.Elem()
		} else {
			paramType = funcType.Params().At(i).Type()
		}

		argumentType = argTypes[i]

		if argFuncType, ok := argumentType.(*types.Signature); ok {
			retVals, _, localErrs := makeFunctionTypeCheck(argFuncType, argSymbols[i], []types.Type{}, []*lexer.Token{}, makeTypeInference)

			if localErrs != nil {
				errs = append(errs, localErrs...)
				continue
			}
			argumentType = unTuple(retVals)[0]
		}

		// type inference processing for argument of type 'any'
		// BUG: WIP
		reconfigTriggeredByBuiltin := func(paramType types.Type) (types.Type, *parser.ParseError) {
			tParam, ok := paramType.(*types.TypeParam)
			_ = tParam
			if ok == false {
				return paramType, nil
			}
			return nil, nil
		}

		if types.Identical(argumentType, TYPE_ANY.Type()) {
			symbol := argSymbols[i]

			isBuiltinFunc := false
			if isBuiltinFunc {
				typ, err := reconfigTriggeredByBuiltin(paramType)
				if err != nil {
					return
				}

				fromArrayToSlice := func(typ types.Type) types.Type {
					return typ
				}

				paramType = typ
				argumentType = fromArrayToSlice(argumentType)
			}

			recheck, err := makeTypeInference(symbol, argumentType, paramType)
			if err != nil {
				errs = append(errs, err)
			}

			if recheck != nil {
				variablesToRecheck = append(variablesToRecheck, recheck)
			}

			continue
		}

		_, errMsg := TypeCheckAgainstConstraint(argumentType, paramType)
		if errMsg != nil {
			err := parser.NewParseError(argSymbols[i], errMsg)
			errs = append(errs, err)
		}
	}

	// 2. Check Validity for Return Type
	returnSize := funcType.Results().Len()
	if returnSize > 2 {
		err := parser.NewParseError(funcSymbol, errFunctionMaxReturn)
		errs = append(errs, err)

	} else if returnSize == 2 {
		secondReturnType := funcType.Results().At(1).Type()
		errorType := TYPE_ERROR.Type()

		if !types.Identical(secondReturnType, errorType) {
			err := parser.NewParseError(funcSymbol, errFunctionSecondReturnNotError)
			errs = append(errs, err)
		}
	} else if returnSize == 0 {
		err := parser.NewParseError(funcSymbol, errFunctionVoidReturn)
		errs = append(errs, err)
	}

	return funcType.Results(), variablesToRecheck, errs
}

func getVariableDefinitionForRootField(variable *lexer.Token, localVariables, globalVariables map[string]*VariableDefinition) (*VariableDefinition, *parser.ParseError) {

	fields, _, _, err := splitVariableNameFields(variable)
	if err != nil {
		return nil, err
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
		err := parser.NewParseError(variable, errVariableUndefined)
		err.Range.End.Character = err.Range.Start.Character + len(fields[0])
		return nil, err
	}

	return varDef, nil
}

func markVariableAsUsed(varDef *VariableDefinition) {
	if varDef == nil {
		return
	}

	varDef.IsUsedOnce = true
}

// TODO: raname to 'getVariableImplicitTypeWhenCurrentTypeIsAnyType()'
// 'getVariableImplicitTypeWhenAnyTypeIsTheCurrentType()'
// 'getVariableImplicitTypeOrReturnAnyTypeOnError()'
func getVariableImplicitType(varDef *VariableDefinition, symbol *lexer.Token, defaultType types.Type) (types.Type, *parser.ParseError) {
	if symbol == nil {
		log.Printf("cannot set implicit type of not existing symbol.\n varDef = %#v", varDef)
		panic("cannot set implicit type of not existing symbol")
	}

	if varDef == nil {
		return defaultType, nil
	}

	// Only check further down (pass this condition) if varDef == ANY
	if !types.Identical(varDef.typ, TYPE_ANY.Type()) {
		return defaultType, nil
	}

	if !types.Identical(defaultType, TYPE_ANY.Type()) {
		return defaultType, nil
	}

	// From here, were are certain of 2 things:
	// 1. defaultType == ANY_TYPE
	// 2. varDef.typ == ANY_TYPE

	fields, _, _, err := splitVariableNameFields(symbol)
	if err != nil {
		return TYPE_ANY.Type(), err
	}

	if varDef.TreeImplicitType == nil {
		return TYPE_ANY.Type(), nil
	}

	currentNode := varDef.TreeImplicitType

	// NOTE: I decide to not validate the fields[0] on purpose
	// All of this because of '.' and '$' variable
	// Specifically, '$' variable is an alias for '.'
	// Thus, there is a difference between the 'symbol' token and the 'VariableDefinition'
	// =======================
	// Within the token, '$' is refered as expected '$'
	// However, within the variable definition, '$' is refered as '.' instead
	// For this reason, I skipped the analysis of fields[0] to avoid doing workaround
	// But everything is working fine, as long as the associated function are used
	// =======================
	// eg. 'getRealTypeAssociatedToVariable()', 'getVariableDefinitionForRootField()'

	for index := 1; index < len(fields); index++ {
		fieldName := fields[index]

		childNode, ok := currentNode.children[fieldName]
		if !ok {
			erroneousFieldsJoined, _ := joinVariableNameFields(fields[index:])

			err = parser.NewParseError(symbol, errFieldNotFound)
			err.Range.Start.Character = err.Range.End.Character - len(erroneousFieldsJoined)

			return TYPE_ANY.Type(), err
		}

		currentNode = childNode
	}

	return currentNode.fieldType, nil
}

func getVariableImplicitRange(varDef *VariableDefinition, symbol *lexer.Token) *lexer.Range {
	if symbol == nil {
		log.Printf("cannot set implicit type of not existing symbol.\n varDef = %#v", varDef)
		panic("cannot set implicit type of not existing symbol")
	}

	if varDef == nil {
		return nil
	}

	if varDef.TreeImplicitType == nil {
		return nil
	}

	currentNode := varDef.TreeImplicitType

	fields, _, _, err := splitVariableNameFields(symbol)

	_ = err

	// NOTE: I decide to not validate the fields[0] on purpose
	// For the reason, look at note within 'getVariableImplicitType()' function

	for index := 1; index < len(fields); index++ {
		fieldName := fields[index]

		childNode, ok := currentNode.children[fieldName]
		if !ok {
			return &currentNode.rng
		}

		currentNode = childNode
	}

	return &currentNode.rng
}

// TODO: rename function 'updateVariableInferredType()', 'insertTypeIntoImplicitTypeNode()', ??????
// 'updateVariableFromPathToImplicitType'
func updateVariableImplicitType(varDef *VariableDefinition, symbol *lexer.Token, symbolType types.Type) (types.Type, *parser.ParseError) {
	if symbol == nil {
		log.Printf("cannot set implicit type of not existing symbol.\n varDef = %#v", varDef)
		panic("cannot set implicit type of not existing symbol")
	}

	if varDef == nil {
		return types.Typ[types.Invalid], nil
	}

	// Implicit type only work whenever 'varDef.typ == TYPE_ANY' only, otherwise the type is specific enough
	// in other word, if type is already known at declaration time, it is useful to try to infer it
	if !types.Identical(varDef.typ, TYPE_ANY.Type()) {
		return symbolType, nil
	}

	if symbolType == nil {
		symbolType = TYPE_ANY.Type()
	}

	// if 'varDef' type is 'any', then build the implicit type

	fields, _, fieldPosCountedFromBack, err := splitVariableNameFields(symbol)
	if err != nil {
		return types.Typ[types.Invalid], err
	}

	if len(fields) == 0 {
		err = parser.NewParseError(symbol, errEmptyVariableName)
		return types.Typ[types.Invalid], err
	}

	if varDef.TreeImplicitType == nil {
		rootName := fields[0]
		rootType := TYPE_ANY.Type()
		rootRange := symbol.Range
		rootRange.End.Character = rootRange.Start.Character + len(rootName)

		varDef.TreeImplicitType = newNodeImplicitType(rootName, rootType, rootRange)

	} else if varDef.TreeImplicitType.toDiscard {
		rootRange := symbol.Range
		rootRange.End.Character = rootRange.Start.Character + len(fields[0])

		varDef.TreeImplicitType.rng = rootRange
		// varDef.TreeImplicitType.fieldName = fields[0] // this one cause a bug for 'with' statement
		varDef.TreeImplicitType.toDiscard = false
	}

	var previousNode, currentNode *nodeImplicitType = nil, nil
	currentNode = varDef.TreeImplicitType

	// Tree traversal & creation with default value for node in the middle
	// only traverse to the last field in varName
	for index := 0; index < len(fields)-1; index++ {
		if currentNode == nil {
			log.Printf("an existing/created 'implicitTypeNode' cannot be also <nil>"+
				"\n fields = %q\n symbolType = %s\n", fields, symbolType)
			panic("an existing/created 'implicitTypeNode' cannot be also <nil>")
		}

		fieldRange := symbol.Range

		// partialVarName, _ := joinVariableNameFields(fields[:index+1])
		// fieldRange.End.Character = fieldRange.Start.Character + len(partialVarName)

		// NEW ERA
		fieldRange.Start.Character = symbol.Range.End.Character - fieldPosCountedFromBack[index] - 1
		fieldRange.End.Character = fieldRange.Start.Character + len(fields[index])
		// END NEW ERA

		// fieldRange.Start.Character = fieldRange.End.Character - len(fieldName)

		if currentNode.isIterable { // We cannot go deeper into the tree if found in middle of path
			errMsg := fmt.Errorf("%w, expected 'struct' or 'func' but found 'iterable'", errTypeMismatch)
			err := parser.NewParseError(symbol, errMsg)
			err.Range = fieldRange

			return currentNode.fieldType, err
		}

		fieldName := fields[index+1]
		childNode, ok := currentNode.children[fieldName]
		if ok {
			if childNode.toDiscard {
				childNode.toDiscard = false
				childNode.rng = fieldRange
			}

			if types.Identical(currentNode.fieldType, TYPE_ANY.Type()) {
				previousNode = currentNode
				currentNode = childNode

				continue
			}

			// Check that the remaining variable path is available within the the 'declared type'
			// ie, turn off 'type inference', and look at type directly
			//
			// currentNode.fieldType != ANY
			remainingVarName, _ := joinVariableNameFields(fields[index:])

			varTokenToCheck := lexer.NewToken(lexer.DOLLAR_VARIABLE, symbol.Range, []byte(remainingVarName))
			varTokenToCheck.Range.Start.Character = symbol.Range.End.Character - len(remainingVarName)

			fakeVarDef := NewVariableDefinition(remainingVarName, nil, nil, "fake_var_definition")
			fakeVarDef.typ = currentNode.fieldType

			fieldType, err := getRealTypeAssociatedToVariable(varTokenToCheck, fakeVarDef)
			if err != nil {
				return currentNode.fieldType, err
			}

			if !types.Identical(fieldType, symbolType) {
				err = parser.NewParseError(symbol, errTypeMismatch)
				err.Range = fieldRange

				return currentNode.fieldType, err
			}

			return currentNode.fieldType, nil
		}

		fieldType := TYPE_ANY.Type()

		childNode = newNodeImplicitType(fieldName, fieldType, fieldRange)
		currentNode.children[fieldName] = childNode

		previousNode = currentNode
		currentNode = childNode
	}

	// Last field in variable name
	lastFieldName := fields[len(fields)-1]
	constraintType := symbolType

	lastFieldRange := symbol.Range
	lastFieldRange.Start.Character = lastFieldRange.End.Character - len(lastFieldName)

	// NO NO NO NOOOOO, this is not the time to check a specific child
	// New Strategy:
	//
	// 0. When currentNode == nil, create a new node and exit
	// 1. Check that currentNode type != ANY ===> then compare 'symbolType' and 'currentNode.fieldType'
	// 2. Check that currentNode.fieldType == ANY ======> then 2 situations can occur:
	// 3. len(currentNode.children) == 0 ==========> then currentNode.fieldType = symbolType
	// 4. len(currentNode.children) > 0 ===========> then for each 'child' compute child_type
	//      and make sure it is part of 'symbolType'
	//

	if currentNode == nil {
		currentNode = newNodeImplicitType(lastFieldName, symbolType, lastFieldRange)
		previousNode.children[lastFieldName] = currentNode // safe bc rootNode != nil

		return currentNode.fieldType, nil
	}

	currentNode.toDiscard = false

	if types.Identical(constraintType, TYPE_ANY.Type()) { // do not update node type when received constraintType is ANY_TYPE
		return currentNode.fieldType, nil
	}

	if !types.Identical(currentNode.fieldType, TYPE_ANY.Type()) {
		if !types.Identical(currentNode.fieldType, constraintType) {
			errMsg := fmt.Errorf("%w, expected '%s' but got '%s'", errTypeMismatch, constraintType, currentNode.fieldType)

			err = parser.NewParseError(symbol, errMsg)
			err.Range = lastFieldRange

			return currentNode.fieldType, err
		}

		return currentNode.fieldType, nil
	}

	// currentNode.fieldType == ANY_TYPE && len(currentNode.children) == 0
	if len(currentNode.children) == 0 {
		currentNode.fieldType = constraintType

		return currentNode.fieldType, nil
	}

	// currentNode.fieldType == ANY_TYPE && len(currentNode.children) > 0
	typ := buildTypeFromTreeOfType(currentNode)

	if types.Identical(typ, TYPE_ANY.Type()) { // this is for case when childreen are node 'toDiscard'
		currentNode.fieldType = constraintType
		return currentNode.fieldType, nil

	} else if !types.Identical(typ, constraintType) {
		errMsg := fmt.Errorf("%w, expected '%s' but got '%s'", errTypeMismatch, constraintType, typ)
		err := parser.NewParseError(symbol, errMsg)

		return currentNode.fieldType, err
	}

	currentNode.fieldType = constraintType

	return currentNode.fieldType, nil
}

// Will only guess if variable type is 'any', otherwise return the current type of the variable
func guessVariableTypeFromImplicitType(varDef *VariableDefinition) types.Type {
	if varDef == nil {
		panic("cannot guess/infer the type of a <nil> variable")
	}

	if !types.Identical(varDef.typ, TYPE_ANY.Type()) {
		return varDef.typ
	}

	// reach here only if variable of type 'any'
	inferredType := buildTypeFromTreeOfType(varDef.TreeImplicitType)

	if types.Identical(inferredType, types.Typ[types.Invalid]) && varDef.name == "." {
		inferredType = varDef.typ
	}

	return inferredType
}

func createImplicitTypeFromRealType(parentNode *nodeImplicitType, convertGoAstPositionToProjectRange func(token.Pos) lexer.Range) *nodeImplicitType {
	if parentNode == nil {
		log.Printf("cannot build implicit type tree from <nil> parent node")
		panic("cannot build implicit type tree from <nil> parent node")
	}

	switch typ := parentNode.fieldType.(type) {
	case *types.Named:
		for index := range typ.NumMethods() {
			method := typ.Method(index)
			rng := convertGoAstPositionToProjectRange(method.Pos())

			childNode := newNodeImplicitType(method.Name(), method.Signature(), rng)
			parentNode.children[childNode.fieldName] = childNode
		}

		parentNode.fieldType = parentNode.fieldType.Underlying()

		_ = createImplicitTypeFromRealType(parentNode, convertGoAstPositionToProjectRange)

		parentNode.fieldType = typ
	case *types.Struct:
		for index := range typ.NumFields() {
			field := typ.Field(index)
			rng := convertGoAstPositionToProjectRange(field.Pos())

			childNode := newNodeImplicitType(field.Name(), field.Type(), rng)
			parentNode.children[childNode.fieldName] = childNode

			_ = createImplicitTypeFromRealType(childNode, convertGoAstPositionToProjectRange)
		}
	default:
		// do nothing
	}

	return parentNode
}

type nodeImplicitType struct {
	toDiscard   bool
	isIterable  bool
	fieldName   string
	fieldType   types.Type
	children    map[string]*nodeImplicitType
	garbageNode map[string]*nodeImplicitType // only useful that handle nasty case created when iterable are involved, do not use it for anything else
	rng         lexer.Range
}

func newNodeImplicitType(fieldName string, fieldType types.Type, reach lexer.Range) *nodeImplicitType {
	if fieldType == nil {
		fieldType = TYPE_ANY.Type()
	}

	node := &nodeImplicitType{
		isIterable:  false,
		toDiscard:   false,
		fieldName:   fieldName,
		fieldType:   fieldType,
		children:    make(map[string]*nodeImplicitType),
		garbageNode: make(map[string]*nodeImplicitType),
		rng:         reach,
	}

	return node
}

func buildTypeFromTreeOfType(tree *nodeImplicitType) types.Type {
	if tree == nil {
		return TYPE_ANY.Type()
	}

	if tree.fieldType == nil {
		tree.fieldType = TYPE_ANY.Type()
	}

	if tree.toDiscard {
		return types.Typ[types.Invalid]
	}

	if !types.Identical(tree.fieldType, TYPE_ANY.Type()) {
		return tree.fieldType
	}

	if tree.isIterable {
		if len(tree.children) > 2 {
			log.Printf("no iterable implicit node can have more than 2 children"+"\n tree = %#v\n", tree)
			panic("no iterable implicit node can have more than 2 children")
		}

		var keyType types.Type = types.Typ[types.Int]

		keyTree := tree.children["key"]
		if keyTree != nil {
			keyType = buildTypeFromTreeOfType(keyTree)
		}

		valueTree := tree.children["value"]
		if valueTree == nil {
			log.Printf("inferred iterable cannot exist without a 'value' node in its type definition"+"\n tree = %#v\n", tree)
			panic("inferred iterable cannot exist without a 'value' node in its type definition")
		}

		valueType := buildTypeFromTreeOfType(valueTree)

		var treeType types.Type
		if types.Identical(keyType, types.Typ[types.Int]) { // create slice
			treeType = types.NewSlice(valueType)
		} else { // create map
			treeType = types.NewMap(keyType, valueType)
		}

		return treeType
	}

	// tree.isNotIterable && tree.fieldType != ANY
	if len(tree.children) == 0 {
		return tree.fieldType
	}

	varFields := make([]*types.Var, 0, len(tree.children))

	for _, node := range tree.children {
		if node.toDiscard {
			continue
		}

		fieldType := buildTypeFromTreeOfType(node)
		field := types.NewVar(token.NoPos, nil, node.fieldName, fieldType)

		varFields = append(varFields, field)
	}

	finalType := tree.fieldType

	if len(varFields) > 0 {
		finalType = types.NewStruct(varFields, nil)
	}

	return finalType
}

// Do not affect the type of any node. It either create an new node or return an existing one
// if the node didn't previously exist, it is tagged as 'toDiscard'
func extractOrInsertTemporaryImplicitTypeFromVariable(varDef *VariableDefinition, symbol *lexer.Token) *nodeImplicitType {
	if varDef == nil {
		panic("found <nil> variable definition while trying to insert implicit node type")
	}

	if varDef.TreeImplicitType == nil {
		root := newNodeImplicitType(varDef.name, varDef.typ, varDef.rng)
		varDef.TreeImplicitType = root
	}

	tree := varDef.TreeImplicitType
	fields, _, _, _ := splitVariableNameFields(symbol)

	for index := 1; index < len(fields); index++ {
		if tree.isIterable { // impossible to go deeper, error
			symbolName := string(symbol.Value)
			root := varDef.TreeImplicitType

			node := root.garbageNode[symbolName]
			if node == nil {
				node = newNodeImplicitType(symbolName, types.Typ[types.Invalid], symbol.Range) // very important
				varDef.TreeImplicitType.garbageNode[node.fieldName] = node
			}

			return node
		}

		fieldName := fields[index]
		child, exists := tree.children[fieldName]
		if !exists { // if child not found create it, and then continue business as usual
			child = newNodeImplicitType(fieldName, TYPE_ANY.Type(), symbol.Range)
			child.toDiscard = true

			varName, _ := joinVariableNameFields(fields[:index+1])
			child.rng.End.Character = child.rng.Start.Character + len(varName)

			tree.children[fieldName] = child
		}

		tree = child
	}

	return tree
}

func insertIterableIntoImplicitTypeNode(tree *nodeImplicitType, keyDefinition, valueDefinition *VariableDefinition) *parser.ParseError {
	if tree == nil {
		log.Printf("expected an implicit node to insert interable type but found <nil>"+"\n keyDef = %#v \n valDef = %#v\n", keyDefinition, valueDefinition)
		panic("expected an implicit node to insert interable type but found <nil>")
	} else if valueDefinition == nil {
		log.Printf("found <nil> as iterable value type\n treeNode = %#v\n keyDef = %#v\n", tree, keyDefinition)
		panic("found <nil> as iterable value type")
	} else if !types.Identical(tree.fieldType, TYPE_ANY.Type()) {
		log.Printf("loop 'key' and 'value' have been created expecting <any-type> for the expression\n tree = %s\n keyDef = %s\n valDef = %s\n", tree, keyDefinition, valueDefinition)
		panic("loop 'key' and 'value' have been created expecting <any-type> for the expression, but its type is " + tree.fieldType.String())
	} else if !types.Identical(valueDefinition.typ, TYPE_ANY.Type()) {
		panic("expected <any-type> for loop value in order to trigger type inference, but got " + valueDefinition.typ.String())
	}

	if tree.isIterable { // if 'iterable' already inferred earlier, use it rather than creating a new one
		if len(tree.children) == 0 {
			log.Printf("no iterable implicit node can have 0 childreen"+"\n tree = %#v\n key = %#v\n value = %#v\n", tree, keyDefinition, valueDefinition)
			panic("no iterable implicit node can have 0 childreen")
		} else if len(tree.children) > 2 {
			log.Printf("no iterable implicit node can have more than 2 children"+"\n tree = %#v\n key = %#v\n value = %#v\n", tree, keyDefinition, valueDefinition)
			panic("no iterable implicit node can have more than 2 children")
		}

		key := tree.children["key"]
		if key != nil && keyDefinition != nil {
			keyDefinition.TreeImplicitType = key
		}

		value := tree.children["value"]
		if value == nil {
			log.Printf("inferred iterable cannot exist without a 'value' node in its type definition"+"\n tree = %#v\n key = %#v\n value = %#v\n", tree, keyDefinition, valueDefinition)
			panic("inferred iterable cannot exist without a 'value' node in its type definition")
		}

		valueDefinition.TreeImplicitType = value

	} else { // tree.isNotIterable, then create a new inferred iterable type
		if hasOnlyDiscardableChildren(tree) {
			clear(tree.children) // very important
		}

		if len(tree.children) > 0 {
			err := parser.NewParseError(&lexer.Token{}, fmt.Errorf("%w, expected array, slice, map, int, chan, or iterator", errTypeMismatch))
			err.Range = tree.rng
			return err
		}

		// make sure there is at least a root node for inference to kick in
		tok := lexer.NewToken(lexer.DOLLAR_VARIABLE, lexer.Range{}, []byte("$fake_root"))
		if valueDefinition.TreeImplicitType == nil {
			valueTree := extractOrInsertTemporaryImplicitTypeFromVariable(valueDefinition, tok)
			valueTree.toDiscard = false
			valueDefinition.TreeImplicitType = valueTree
		}

		if keyDefinition != nil && keyDefinition.TreeImplicitType == nil {
			keyTree := extractOrInsertTemporaryImplicitTypeFromVariable(keyDefinition, tok)
			keyTree.toDiscard = false
			keyDefinition.TreeImplicitType = keyTree
		}

		// if ok (empty children), convert node to iterable type
		tree.isIterable = true

		if keyDefinition != nil {
			tree.children["key"] = keyDefinition.TreeImplicitType
		}

		tree.children["value"] = valueDefinition.TreeImplicitType
	}

	return nil
}

func hasOnlyDiscardableChildren(tree *nodeImplicitType) bool {
	if tree == nil {
		panic("<nil> nodeImplicitType found while looking for its children discard status")
	}

	for _, child := range tree.children {
		if child.toDiscard == false {
			return false
		}
	}

	return true
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
// 3. Send over the file data or at least definition for all function that will be used
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
		// msg := "boundary.End.Line = %d ::: rangeRemaped.End.Line = %d\n"
		// log.Printf(msg, boundary.End.Line, rangeRemaped.End.Line)
		// log.Printf("boundary.End.Line = %d ::: rangeRemaped.End.Line = %d\n", boundary.End.Line, rangeRemaped.End.Line)

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
		// log.Println("comment scanner error found, ", parseError)

		errorList, ok := parseError.(scanner.ErrorList)
		if !ok {
			panic("unexpected error, error obtained by go code parsing did not return expected type ('scanner.ErrorList')")
		}

		const randomColumnOffset int = 7

		for _, errScanner := range errorList {
			// A. Build diagnostic errors
			parseErr := NewParseErrorFromErrorList(errScanner, randomColumnOffset)
			parseErr.Range = remapRangeFromCommentGoCodeToSource(virtualHeader, comment.GoCode.Range, parseErr.Range)

			// log.Println("comment scanner error :: ", parseErr)

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

// ------------
// ------------
// LSP-like helper functions
// ------------
// ------------

// NodeDefinition interface{} = FileDefinition(?) | VariableDefinition | TemplateDefinition | FunctionDefinition
func FindSourceDefinitionFromPosition(file *FileDefinition, position lexer.Position) []NodeDefinition {
	// 1. Find the node and token corresponding to the provided position
	seeker := &findAstNodeRelatedToPosition{Position: position, fileName: file.name}

	log.Println("position before walker: ", position)
	parser.Walk(seeker, file.root)
	log.Printf("seeker after walker : %#v\n", seeker)

	if seeker.TokenFound == nil { // No definition found
		return nil
	}

	//
	// 2. From the node and token found, find the appropriate 'Source Definition'
	//
	invalidVariableDefinition := NewVariableDefinition(string(seeker.TokenFound.Value), seeker.NodeFound, seeker.LastParent, file.FileName())
	invalidVariableDefinition.typ = types.Typ[types.Invalid]
	invalidVariableDefinition.rng.Start = position

	if seeker.IsTemplate {
		var allTemplateDefs []NodeDefinition = nil
		templateManager := TEMPLATE_MANAGER

		templateName := string(seeker.TokenFound.Value)

		for templateScope, def := range templateManager.TemplateScopeToDefinition {
			if templateName == templateScope.TemplateName() {
				allTemplateDefs = append(allTemplateDefs, def)
			}
		}

		return allTemplateDefs

	} else if seeker.IsExpression || seeker.IsVariable {

		// handle case where 'seeker.tokenFound' is either 'string', 'number', 'bool'
		basicDefinition := createNodeDefinitionForBasicType(seeker.TokenFound, seeker.NodeFound, file.name)
		if basicDefinition != nil {
			return []NodeDefinition{basicDefinition}
		}

		fields, _, fieldPosCountedFromBack, errSplit := splitVariableNameFields(seeker.TokenFound)
		if errSplit != nil {
			log.Printf("warning, spliting variable name was unsuccessful :: varName = %s\n", string(seeker.TokenFound.Value))
			singleDefinition := []NodeDefinition{invalidVariableDefinition}
			return singleDefinition
		}

		if len(fields) == 0 {
			return nil
		}

		var symbolDefinition NodeDefinition = nil
		rootVarName := fields[0]

		// Check whether it is a function or variable
		functionDef := file.functions[rootVarName]
		variableDef := file.GetVariableDefinitionWithinScope(rootVarName, seeker.LastParent)

		if functionDef != nil {
			symbolDefinition = functionDef

		} else if variableDef != nil {
			symbolDefinition = variableDef

		} else {
			singleDefinition := []NodeDefinition{invalidVariableDefinition}
			return singleDefinition
		}

		if len(fields) == 1 {
			singleDefinition := []NodeDefinition{symbolDefinition}
			return singleDefinition
		}

		relativeCursorPosition := position
		relativeCursorPosition.Line = position.Line - seeker.TokenFound.Range.End.Line
		relativeCursorPosition.Character = (seeker.TokenFound.Range.End.Character - 1) - position.Character // char > 0

		fieldIndex := findFieldContainingRange(fieldPosCountedFromBack, relativeCursorPosition)

		newVarName, err := joinVariableNameFields(fields[:fieldIndex+1])
		if err != nil {
			log.Printf("variable name was split successfully, but now cannot be joined for some reason"+
				"\n fields = %q\n", fields)
			panic("variable name was split successfully, but now cannot be joined for some reason")
		}

		newToken := lexer.CloneToken(seeker.TokenFound)
		newToken.Value = []byte(newVarName)
		newToken.Range.End.Character = newToken.Range.End.Character - fieldPosCountedFromBack[fieldIndex] + len(fields[fieldIndex])

		temporaryVariableDef := NewVariableDefinition(symbolDefinition.Name(), symbolDefinition.Node(), nil, symbolDefinition.FileName())
		temporaryVariableDef.typ = symbolDefinition.Type()

		typ, err := getRealTypeAssociatedToVariable(newToken, temporaryVariableDef)
		if err != nil {
			log.Printf("error while analysis variable chain :: "+err.String()+
				"\n\n associated type = %s\n", typ)
		}

		variableDef = NewVariableDefinition(newVarName, seeker.NodeFound, seeker.LastParent, file.FileName())
		variableDef.typ = typ

		// TODO: use 'variableDef.implicitType' to find location of the source type instead
		variableDef.rng = newToken.Range

		varDef, ok := symbolDefinition.(*VariableDefinition)
		if ok && varDef.TreeImplicitType != nil {
			reach := getVariableImplicitRange(varDef, newToken)
			if reach != nil {
				variableDef.rng = *reach
			}
		}

		if fieldIndex == 0 {
			variableDef.rng = symbolDefinition.Range()
		}

		singleDefinition := []NodeDefinition{variableDef}

		return singleDefinition
	} else if seeker.IsKeyword {

		switch node := seeker.NodeFound.(type) {
		case *parser.SpecialCommandNode:
			target := node.Target
			if target == nil {
				return nil
			}

			def := NewKeywordSymbolDefinition(target.StreamToken.String(), file.name, target)
			singleDefinition := []NodeDefinition{def}

			return singleDefinition

		case *parser.GroupStatementNode:
			target := node.NextLinkedSibling
			if target == nil {
				return nil
			}

			def := NewKeywordSymbolDefinition(target.StreamToken.String(), file.name, target)
			singleDefinition := []NodeDefinition{def}

			return singleDefinition

		default:
		}

		panic("keyword symbol definition finder not implemented yet!")
	}

	return nil
}

func createNodeDefinitionForBasicType(token *lexer.Token, node parser.AstNode, fileName string) NodeDefinition {
	def := &BasicSymbolDefinition{
		node:     node,
		rng:      token.Range,
		fileName: fileName,
		name:     fmt.Sprintf("%s", string(token.Value)),
		typ:      getBasicTypeFromTokenID(token.ID),
	}

	if def.typ == nil {
		return nil
	}

	switch token.ID {
	case lexer.STRING:
		def.name = fmt.Sprintf("`%s`", string(token.Value))
	}

	return def
}

func mustGetBasicTypeFromTokenID(tokenId lexer.Kind) *types.Basic {
	typ := getBasicTypeFromTokenID(tokenId)
	if typ == nil {
		panic("no basic type found for this token kind: " + tokenId.String())
	}

	return typ
}

func getBasicTypeFromTokenID(tokenId lexer.Kind) *types.Basic {
	switch tokenId {
	case lexer.NUMBER:
		return types.Typ[types.Int]
	case lexer.DECIMAL:
		return types.Typ[types.Float64]
	case lexer.COMPLEX_NUMBER:
		return types.Typ[types.Complex128]
	case lexer.BOOLEAN:
		return types.Typ[types.Bool]
	case lexer.STRING:
		return types.Typ[types.String]
	case lexer.CHARACTER:
		return types.Typ[types.Int]
	}

	return nil
}

type findAstNodeRelatedToPosition struct {
	Position     lexer.Position
	TokenFound   *lexer.Token
	LastParent   *parser.GroupStatementNode
	NodeFound    parser.AstNode // nodeStatement
	fileName     string
	IsTemplate   bool
	IsVariable   bool
	IsExpression bool
	IsKeyword    bool
	IsHeader     bool
}

func (v *findAstNodeRelatedToPosition) SetHeaderFlag(ok bool) {
	v.IsHeader = ok
}

// the search for the appropriate node is highly dependent on AstNode 'Range'
// If the parent node do not contain the range of its children, then they will never be discover/found
func (v *findAstNodeRelatedToPosition) Visit(node parser.AstNode) parser.Visitor {
	if node == nil {
		return nil
	}

	// 1. Going down the node tree
	if v.TokenFound != nil {
		return nil
	}

	if !node.Range().Contains(v.Position) {
		return nil
	}

	switch n := node.(type) {
	case *parser.GroupStatementNode:
		if n.KeywordRange.Contains(v.Position) {
			v.TokenFound = n.KeywordToken
			v.NodeFound = n
			v.IsKeyword = true

			return nil
		}

		v.LastParent = n
		return v
	case *parser.TemplateStatementNode:
		if n.TemplateName == nil {
			return nil
		} else if n.TemplateName.Range.Contains(v.Position) {
			v.TokenFound = n.TemplateName
			v.NodeFound = n
			v.IsTemplate = true
		}

		v.NodeFound = n

		return v
	case *parser.VariableAssignationNode:
		for _, variable := range n.VariableNames {
			if variable.Range.Contains(v.Position) {
				v.TokenFound = variable
				v.NodeFound = n
				v.IsVariable = true

				if v.IsHeader {
					v.LastParent = v.LastParent.Parent()
				}

				return nil
			}
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
				return v
			}
		}

		return nil
	case *parser.ExpressionNode:
		for index, symbol := range n.Symbols {
			if symbol.Range.Contains(v.Position) {

				// if expendable token, trigger analysis of all 'ExpandedTokens' element
				if symbol.ID == lexer.EXPANDABLE_GROUP {
					childExpression := n.ExpandedTokens[index]
					if childExpression == nil {
						panic("no AST Node found for expandable token at " + symbol.Range.String() + " in " + v.fileName)
					}

					if childExpression.Range().Contains(v.Position) {
						return v
					}

					// if we reach here, this mean that the token is expandable
					// but we are either targeting the paren '(' or ')'
					// or even the 'fields' connected to the root var name
				}

				// otherwise
				v.TokenFound = symbol
				v.IsExpression = true

				if v.NodeFound == nil {
					v.NodeFound = n
				}

				if v.IsHeader {
					v.LastParent = v.LastParent.Parent()
				}

				return nil
			}
		}

	case *parser.SpecialCommandNode:
		if n.Range().Contains(v.Position) {
			v.TokenFound = n.Value
			v.NodeFound = n
			v.IsKeyword = true

			return nil
		}
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

func Hover(definition NodeDefinition) (string, lexer.Range) {
	if definition == nil {
		panic("Hover() do not accept <nil> definition")
	}

	reach := definition.Range()
	typeStringified := definition.TypeString()
	typeStringified = "```go\n" + typeStringified + "\n```"

	return typeStringified, reach
}

func goToDefinitionForFileNodeOnly(position lexer.Range) (node parser.AstNode, reach lexer.Range) {
	panic("not implemented yet")
}

// constraintType is the 'receiver' type
// old function name: 'TypeCheckClassic()'
func TypeCheckAgainstConstraint(candidateType, constraintType types.Type) (types.Type, error) {
	if types.Identical(constraintType, TYPE_ANY.Type()) {
		return candidateType, nil
	}

	switch receiver := constraintType.(type) {
	case *types.TypeParam:
		it, ok := receiver.Constraint().Underlying().(*types.Interface)
		if ok == false {
			panic("type checker expected an 'interface' within 'type parameter'. type = " + constraintType.String())
		}

		if types.Satisfies(candidateType, it) {
			return candidateType, nil
		}

	default:
		if types.AssignableTo(candidateType, constraintType) {
			return candidateType, nil
		}
	}

	err := fmt.Errorf("%w, expected '%s' but got '%s'", errTypeMismatch, constraintType, candidateType)
	return types.Typ[types.Invalid], err
}

// WARNING: the returned 'typ' is rubish, do not use it in any circumstance
// otherwise, candidateType must have at least all fields and method contained within templateType to pass
func TypeCheckCompatibilityWithConstraint(expressionType, templateType types.Type) (types.Type, error) {
	if types.Identical(expressionType, TYPE_ANY.Type()) {
		return TYPE_ANY.Type(), errDefeatedTypeSystem
	} else if types.Identical(templateType, TYPE_ANY.Type()) {
		return TYPE_ANY.Type(), nil
	}

	if types.Identical(expressionType, templateType) {
		return expressionType, nil
	}

	// way to go
	// 1. transform expressionType & templateType to implicitType tree
	// 2. compare the path of those two tree only whenever children are found
	// 3. If we have reached the leave of the tree, compare both the path and the type

	constraintTree := convertTypeToImplicitType(templateType)
	exprTree := convertTypeToImplicitType(expressionType)

	typ, err := checkImplicitTypeCompatibility(exprTree, constraintTree, "$")
	return typ, err
}

func convertTypeToImplicitType(sourceType types.Type) *nodeImplicitType {
	if sourceType == nil {
		log.Printf("unable to convert <nil> type to an implicit type tree")
		panic("unable to convert <nil> type to an implicit type tree")
	}

	parentNode := newNodeImplicitType("<PARENT_NODE>", sourceType, lexer.Range{})

	switch typ := sourceType.(type) {
	case *types.Named: // tree's leave
		for index := range typ.NumMethods() {
			fieldName := typ.Method(index).Name()
			fieldType := typ.Method(index).Signature()

			node := newNodeImplicitType(fieldName, fieldType, lexer.Range{})
			parentNode.children[node.fieldName] = node
		}
	case *types.Struct:
		for index := range typ.NumFields() {
			currentField := typ.Field(index)

			node := convertTypeToImplicitType(currentField.Type())
			node.fieldName = currentField.Name()

			parentNode.children[node.fieldName] = node
		}

	case *types.Alias:
		parentNode = convertTypeToImplicitType(types.Unalias(typ))
	default: // tree's leave
		parentNode.fieldType = typ
		parentNode.fieldName = "<LEAVE_NODE>"
	}

	return parentNode
}

func checkImplicitTypeCompatibility(candidateTree, constraintTree *nodeImplicitType, rootPath string) (types.Type, error) {
	if candidateTree == nil {
		panic("<nil> value for 'candidateTree' within 'checkImplicitTypeCompatibility()'")
	} else if constraintTree == nil {
		panic("<nil> value for 'constraintTree' within 'checkImplicitTypeCompatibility()'")
	}

	// 1. Whenever reaching tree's leave, check that the type match
	if len(constraintTree.children) == 0 {
		typ, err := TypeCheckAgainstConstraint(candidateTree.fieldType, constraintTree.fieldType)
		if err != nil {
			return typ, fmt.Errorf("%w for field '%s'", err, rootPath)
		}

		return typ, nil
	}

	// 2. Check that every field in constraintTree are also present into candidateTree
	for childName, childNode := range constraintTree.children {
		_, ok := candidateTree.children[childName]

		if !ok {
			return types.Typ[types.Invalid], fmt.Errorf("%w, field not found: '%s' of type '%s'", errTypeMismatch, rootPath+"."+childName, childNode.fieldType)
		}
	}

	// 3. Now go check one level deeper
	for childName, childNode := range constraintTree.children {
		newRootPath := rootPath + "." + childName
		candidateChildNode := candidateTree.children[childName]

		typ, err := checkImplicitTypeCompatibility(candidateChildNode, childNode, newRootPath)
		if err != nil {
			return typ, err
		}
	}

	return constraintTree.fieldType, nil
}

// resolve/obtain, resolveKeyTypeFromInterableType()
// Also, 'iterator' type is still not working properly ! to improve in the future
func getKeyAndValueTypeFromIterableType(source types.Type) (key types.Type, value types.Type, err error) {
	source = source.Underlying()

	if types.Identical(source, TYPE_ANY.Type()) {
		return TYPE_ANY.Type(), TYPE_ANY.Type(), nil
	}

	switch typ := source.(type) {
	case *types.Slice:
		return types.Typ[types.Int], typ.Elem(), nil

	case *types.Map:
		return typ.Key(), typ.Elem(), nil

	case *types.Array:
		return types.Typ[types.Int], typ.Elem(), nil

	case *types.Basic:
		if !types.Identical(typ, types.Typ[types.Int]) {
			break
		}

		return types.Typ[types.Invalid], types.Typ[types.Int], nil

	case *types.Chan:
		return types.Typ[types.Int], typ.Elem(), nil

	// case *types.Named:
	//
	// look for iter.seq & iter.seq2 method
	case *types.Signature: // handle iter.seq & iter.seq2 here
		err = fmt.Errorf("%w, iterator type poorly definied", errTypeMismatch)

		// 1. verify that return type match iter.seq definition
		if typ.Results().Len() != 1 {
			break // will return an error
		}

		returnType := typ.Results().At(0).Type().Underlying()
		if !types.Identical(returnType, types.Typ[types.Bool]) {
			break // will return an error
		}

		// 2. verify that parameter size match iter.seq definition
		// TODO: continue handling iterator
		// however, I should fetch the definiton of the iterator within the std
		// then compare the std version against the user version to see if there is match
		// eg. types.Identical(stdIteratorSignature, typ)
		// For now return any_type

		return TYPE_ANY.Type(), TYPE_ANY.Type(), nil

	default:
		break // will return an error
	}

	err = fmt.Errorf("%w, expected array, slice, map, int, chan, or iterator", errTypeMismatch)

	return types.Typ[types.Invalid], types.Typ[types.Invalid], err
}

// End
