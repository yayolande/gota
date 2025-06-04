package gota

// TODO: Rename package to 'gosh' ???? I am on the fence about it !

import (
	"bytes"
	"errors"
	// errors"
	"fmt"
	// go/types"
	"io"
	"log"
	"maps"
	"os"
	"path/filepath"
	"strings"

	checker "github.com/yayolande/gota/analyzer"
	"github.com/yayolande/gota/lexer"
	"github.com/yayolande/gota/parser"
)

// TODO: I have an architecture/design mistake concerning the handling of error while parsing
// First, the problem.
// As it stands, the parsing pipeline is as follows:
//
// text -> extract template -> lexer -> parsing -> analysis
//
// While lexing, if an error occur on a template line, that line is dropped altogether. The same goes for the parsing
// This mean that the output of the lexer and parser drop template line
// The issue with this design decision start with group-like template. For instance
// `{{ if -- }} hello {{ end }}` has an error on the first template line, and because of this only "{{ end }}" token
// will be output while lexing. Then you may ask, what may go wrong with this output ?
// Well you see, every 'if' statement should be close by an 'end' statement. At least that's how the parser
// expect thing to be. However, since the 'if' statement have been dropped, the analyzer will only see the "{{ end }}" AST
// then it will swiftly report an Error
// thus an error on "{{ if ... }}" during lexing trigger an automatic error on "{{ end }}" while parsing. That's just wrong
// {{ end }} do not have any issue, only "{{ if ... }}" does. this could mislead the user to think that he made a syntax error
// or that there is too many {{ end }} statement
//
// I there recommend, on a future version, to overhaul the lexer and parser architecture.
// 2 things need to change
// first, is the tokens returned by the lexer. second, the lexer and parser should return failed tokens and ast all along
//
// For the first, `[]Token` should become `TokenFile struct { listToken [][]Token; listTokenSucessStatus []bool }` for the return type
// Or maybe `TokenStatements { statement []Token; status bool }` and `TokenFile { line []TokenStatements }`.
// But I still wonder, is 'TokenFile' mandatory ?
//
// token < token line (statement == "{{ ... }}") < token file
// token line (statement) = list of tokens with the last token being 'EOL'
// token file = list of token line
//
// For the second, the lexer and parser should do as much as possible to return the closest valid token/ast, so that
// those tokens and their states (boolean status) are seen by the parser. The parser then could make adjustment onto which statement
// can return error while parsing. obviously, a line of tokens that have failed should in any case return a parse error, since the
// whole goal of this process is to make sure the 'analysis' phase do not output indesirable error to the user
// On the same vain, a field should be added in the AST, allowing to identify whether the ast is failed parsing or not, since now all
// failing and successful ast are returned. `ast { isParseError bool }`

type Error = lexer.Error

// Recursively open files from 'rootDir'.
// However there is a depth limit for the recursion (current MAX_DEPTH = 5)
func OpenProjectFiles(rootDir, withFileExtension string) map[string][]byte {
	const maxDepth int = 5
	var currentDepth int = 0

	return openProjectFilesSafely(rootDir, withFileExtension, currentDepth, maxDepth)
}

func openProjectFilesSafely(rootDir, withFileExtension string, currentDepth, maxDepth int) map[string][]byte {
	if currentDepth > maxDepth {
		return nil
	}

	list, err := os.ReadDir(rootDir)
	if err != nil {
		panic("error while reading directory content: " + err.Error())
	}

	fileNamesToContent := make(map[string][]byte)

	for _, entry := range list {
		fileName := filepath.Join(rootDir, entry.Name())

		if entry.IsDir() {
			subDir := fileName
			subFiles := openProjectFilesSafely(subDir, withFileExtension, currentDepth+1, maxDepth)

			maps.Copy(fileNamesToContent, subFiles)
			continue
		}

		if !strings.HasSuffix(fileName, withFileExtension) {
			continue
		}

		file, err := os.Open(fileName)
		if err != nil {
			log.Println("unable to open file, ", err.Error())
			continue
		}

		fileContent, err := io.ReadAll(file)
		fileNamesToContent[fileName] = fileContent
	}

	if fileNamesToContent == nil {
		panic("'openProjectFilesSafely()' should never return a 'nil' file hierarchy. return an empty map instead")
	}

	return fileNamesToContent
}

// Parse a file content (buffer). The output is an AST node, and an error list containing parsing error and suggestions
func ParseSingleFile(source []byte) (*parser.GroupStatementNode, []Error) {
	tokens, _, tokenErrs := lexer.Tokenize(source)
	parseTree, parseErrs := parser.Parse(tokens)

	parseErrs = append(parseErrs, tokenErrs...)

	return parseTree, parseErrs
}

// Parse all files within a workspace.
// The output is an AST node, and an error list containing parsing error and suggestions
func ParseFilesInWorkspace(workspaceFiles map[string][]byte) (map[string]*parser.GroupStatementNode, []Error) {
	parsedFilesInWorkspace := make(map[string]*parser.GroupStatementNode)

	var errs []Error
	for longFileName, content := range workspaceFiles {
		tokens, _, tokenErr := lexer.Tokenize(content)
		parseTree, parseError := parser.Parse(tokens)

		parsedFilesInWorkspace[longFileName] = parseTree

		errs = append(errs, tokenErr...)
		errs = append(errs, parseError...)
	}

	if len(workspaceFiles) != len(parsedFilesInWorkspace) {
		panic("number of parsed files do not match the amount present in the workspace")
	}

	if parsedFilesInWorkspace == nil {
		panic("'ParseFilesInWorkspace()' should never return a 'nil' workspace. return an empty map instead")
	}

	return parsedFilesInWorkspace, errs
}

// TODO: disallow circular dependencies for 'template definition'
func DefinitionAnalysisSingleFile(fileName string, parsedFilesInWorkspace map[string]*parser.GroupStatementNode) (*checker.FileDefinition, []Error) {
	if len(parsedFilesInWorkspace) == 0 {
		return nil, nil
	}

	parseTreeActiveFile := parsedFilesInWorkspace[fileName]
	if parseTreeActiveFile == nil {
		return nil, nil
	}

	workspaceTemplateDefinition, templateErrs := buildWorkspaceTemplateDefinition(parsedFilesInWorkspace)
	file, errs := checker.DefinitionAnalysis(fileName, parseTreeActiveFile, workspaceTemplateDefinition)

	// TODO: I am not sure about this one
	errs = append(errs, templateErrs...)

	return file, errs
}

// Definition analysis for all files within a workspace.
// It should only be done after 'ParseFilesInWorkspace()' or similar
// TODO: REMAKE THIS FUNCTION
func DefinitionAnalisisWithinWorkspace(parsedFilesInWorkspace map[string]*parser.GroupStatementNode) (map[string]*checker.FileDefinition, []Error) {
	if len(parsedFilesInWorkspace) == 0 {
		return nil, nil
	}

	var errs []lexer.Error
	analyzedFilesInWorkspace := make(map[string]*checker.FileDefinition)

	workspaceTemplateDefinition, templateErrs := buildWorkspaceTemplateDefinition(parsedFilesInWorkspace)

	for longFileName, fileParseTree := range parsedFilesInWorkspace {
		if fileParseTree == nil {
			continue
		}

		file, localErrs := checker.DefinitionAnalysis(longFileName, fileParseTree, workspaceTemplateDefinition)

		analyzedFilesInWorkspace[longFileName] = file
		errs = append(errs, localErrs...)
	}

	// TODO: not sure about this one
	errs = append(errs, templateErrs...)

	return analyzedFilesInWorkspace, errs
}

// TODO: not completed, need to receive the workspaceFiles (parsed and/or analyzed)
func GoToDefinition(file *checker.FileDefinition, position lexer.Position) (fileName string, reach lexer.Range, err error) {
	tok, nodecontainer, parentScope, isTemplate := checker.FindAstNodeRelatedToPosition(file.Root, position)
	if tok == nil {
		log.Println("token not found for definition")
		return "", lexer.Range{}, errors.New("meaningful token not found")
	}

	fileName, nodeDef, reach := checker.GoToDefinition(tok, nodecontainer, parentScope, file, isTemplate)
	if nodeDef == nil {
		return "", reach, errors.New("token definition not found anywhere")
	}

	return fileName, reach, nil
}

// TODO: This one is unused, should I remove it ?
func GetDependenciesFilesForTemplateCallWithinWorkspace(workspace map[string]*parser.GroupStatementNode) (dependencies [][]string, errs []Error) {
	templatesWithinWorkspace := make(map[string][]*checker.TemplateDefinition)

	for fileName, scope := range workspace {
		// get templates for each files
		templatesDefinition := getRootTemplateDefinition(scope, fileName)

		// convert it into map[string][]*TemplateDefinition, so that string = templateName; []*TemplateDefinition = related definition
		for _, templateDef := range templatesDefinition {
			templatesWithinWorkspace[templateDef.Name] = append(templatesWithinWorkspace[templateDef.Name], templateDef)
		}
	}

	for fileName, scope := range workspace {
		visitor := &extractTemplateUse{}
		visitor.templatesWithinWorkspace = templatesWithinWorkspace
		visitor.dependencyFileNames = append(visitor.dependencyFileNames, []string{fileName})

		parser.Walk(visitor, scope)
	}

	// TODO: implementation incomplete
	panic("not yet implemented")
}

// TODO: This one is unused, should I remove it ?
type extractTemplateUse struct {
	isRootVisited            bool
	templatesWithinWorkspace map[string][]*checker.TemplateDefinition
	dependencyFileNames      [][]string

	singleFileDepencies []string
}

func (v *extractTemplateUse) Visit(node parser.AstNode) parser.Visitor {

	switch n := node.(type) {
	case *parser.GroupStatementNode:
		if !v.isRootVisited {
			v.isRootVisited = true
			return v
		}

		// Do not analyze within template node when they are not the root of the tree
		if n.Kind == parser.KIND_DEFINE_TEMPLATE || n.Kind == parser.KIND_BLOCK_TEMPLATE {
			return nil
		}

		return v

	case *parser.TemplateStatementNode:
		// Do not count other type of 'TemplateStatementNode' for the dependencies analysis
		if n.Kind != parser.KIND_USE_TEMPLATE {
			return nil
		}

		templateName := string(n.TemplateName.Value)
		templatesFound, ok := v.templatesWithinWorkspace[templateName]
		if !ok {
			// when there is error, it is not the role of depency graph to report it
			// the only goal is report the depency graph and whether or not there is a cyclical depency
			// the rest of the error (beside cyclical deps) are to be ignored and be handled
			// by other form of analysis
			return nil
		}

		v.dependencyFileNames = nil

		for _, templateDef := range templatesFound {
			// add template name to depency
			fileName := templateDef.FileName

			// explore parse tree of the 'template' in question to find its own depencies as well
			visitor := &extractTemplateUse{}
			visitor.templatesWithinWorkspace = v.templatesWithinWorkspace
			visitor.dependencyFileNames = append(visitor.dependencyFileNames, []string{fileName})

			parser.Walk(visitor, templateDef.Node) // Not sure about 'v', perhaps I should create another one

			// append the result to global dependency graph
		}

		return v
	}

	return nil
}

// Print in JSON format the AST node to the screen. Use a program like 'jq' for pretty formatting
func Print(node ...parser.AstNode) {
	str := parser.PrettyFormater(node)
	fmt.Println(str)
}

// Obtains all template definition available at the root of the group nodes only (no node traversal)
func getRootTemplateDefinition(root *parser.GroupStatementNode, fileName string) []*checker.TemplateDefinition {
	if root == nil {
		return nil
	}

	var listTemplateDefinition []*checker.TemplateDefinition

	for _, statement := range root.Statements {
		if statement == nil {
			panic("unexpected 'nil' statement found in scope holder (group) while listing template definition available in parent scope")
		}

		if !(statement.GetKind() == parser.KIND_DEFINE_TEMPLATE || statement.GetKind() == parser.KIND_BLOCK_TEMPLATE) {
			continue
		}

		templateScope, ok := statement.(*parser.GroupStatementNode)
		if !ok {
			panic("unexpected type found. As per standard, 'template' parent should be 'GroupStatementNode' type only")
		}

		templateHeader, ok := templateScope.ControlFlow.(*parser.TemplateStatementNode)
		if !ok {
			panic("unexpected type found. As per standard, 'template' header should be wrapped by 'TemplateStatementNode' type only")
		}

		templateName := string(bytes.Clone(templateHeader.TemplateName.Value))

		// TODO: this is no good reguarding the typing system (to improve when type-system is ready)
		def := &checker.TemplateDefinition{}
		def.Name = templateName
		def.Node = templateScope
		def.Range = templateScope.Range
		def.FileName = fileName
		def.IsValid = true

		// TODO: REFACTOR THIS CODE COMMENTED BELOW

		// def.InputType = &checker.DataStructureDefinition{}
		// def.InputType.Name = "any"
		// def.InputType.IsValid = true

		listTemplateDefinition = append(listTemplateDefinition, def)
		// listTemplateDefinition[templateName] = statement
	}

	return listTemplateDefinition
}

// TODO: WIP
func buildWorkspaceTemplateDefinition(parsedFilesInWorkspace map[string]*parser.GroupStatementNode) (map[*parser.GroupStatementNode]*checker.TemplateDefinition, []lexer.Error) {
	handler := checker.NewTemplateDefinitionHandler(parsedFilesInWorkspace)

	// main processing
	for template := range handler.TemplateToFileName {
		if handler.TemplateToDefinition[template] != nil {
			continue
		}

		templateName := string(template.ControlFlow.(*parser.TemplateStatementNode).TemplateName.Value) // safe bc of 'NewTemplateDefinitionHandler()'
		handler.BuildTemplateDefinition(template, templateName)
	}

	// error checking
	if len(handler.TemplateToFileName) != len(handler.TemplateToDefinition) {
		log.Printf("length mismatch between template existing and template analyzed\n"+
			" handler.TemplateToFileName = %#v\n handler.TemplateToDefinition = %#v\n",
			handler.TemplateToFileName, handler.TemplateToDefinition)
		panic("length mismatch between template existing and template analyzed")
	} else if handler.TemplateToDefinition == nil {
		msg := "'TemplateToDefinition' cannot be 'nil' while at the end of 'buildWorkspaceTemplateDefinition' process"

		log.Printf(msg+"\n handler = %#v\n", handler)
		panic(msg)
	}

	return handler.TemplateToDefinition, handler.Errs
}

// TODO: This one is unused, should I remove it ?
//
// Get a list of all template definition (identified with "define" keyword) within the workspace
func getWorkspaceTemplateDefinition(parsedFilesInWorkspace map[string]*parser.GroupStatementNode) []*checker.TemplateDefinition {
	var workspaceTemplateDefinition []*checker.TemplateDefinition
	var fileTemplateDefinition []*checker.TemplateDefinition

	for fileName, parseTree := range parsedFilesInWorkspace {
		fileTemplateDefinition = getRootTemplateDefinition(parseTree, fileName)
		workspaceTemplateDefinition = append(workspaceTemplateDefinition, fileTemplateDefinition...)
	}

	if workspaceTemplateDefinition == nil {
		workspaceTemplateDefinition = []*checker.TemplateDefinition{}
	}

	return workspaceTemplateDefinition
}

// TODO: This one is unused, should I remove it ?
func getBuiltinVariableDefinition() parser.SymbolDefinition {
	globalVariables := parser.SymbolDefinition{
		".": nil,
		"$": nil,
	}

	return globalVariables
}

// TODO: This one is unused, should I remove it ?
func getBuiltinFunctionDefinition() parser.SymbolDefinition {
	builtinFunctionDefinition := parser.SymbolDefinition{
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

	return builtinFunctionDefinition
}
