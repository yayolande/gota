package gota

// TODO: Rename package to 'gosh' ???? I am on the fence about it !

import (
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

type FileAnalysisAndError struct {
	FileName string
	File     *checker.FileDefinition
	Errs     []lexer.Error
}

type Error = lexer.Error

// Recursively open files from 'rootDir'.
// However there is a depth limit for the recursion (current MAX_DEPTH = 5)
// TODO: expand authorized file extension to '.gohtml, .gohtmpl, .tmpl, .tpl, etc.'
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

	return fileNamesToContent
}

// Parse a file content (buffer). The output is an AST node, and an error list containing parsing error and suggestions
func ParseSingleFile(source []byte) (*parser.GroupStatementNode, []Error) {
	streamsOfToken, tokenErrs := lexer.Tokenize(source)
	parseTree, parseErrs := parser.Parse(streamsOfToken)

	parseErrs = append(parseErrs, tokenErrs...)

	return parseTree, parseErrs
}

// Parse all files within a workspace.
// The output is an AST node, and an error list containing parsing error and suggestions
// Never return nil, always an empty 'map' if nothing found
func ParseFilesInWorkspace(workspaceFiles map[string][]byte) (map[string]*parser.GroupStatementNode, []Error) {
	parsedFilesInWorkspace := make(map[string]*parser.GroupStatementNode)

	var errs []Error

	for longFileName, content := range workspaceFiles {
		streamsOfToken, tokenErrs := lexer.Tokenize(content)
		parseTree, parseError := parser.Parse(streamsOfToken)

		parsedFilesInWorkspace[longFileName] = parseTree

		errs = append(errs, tokenErrs...)
		errs = append(errs, parseError...)
	}

	if len(workspaceFiles) != len(parsedFilesInWorkspace) {
		panic("number of parsed files do not match the amount present in the workspace")
	}

	return parsedFilesInWorkspace, errs
}

// This version is inefficient but simplier to use
// Use 'DefinitionAnalysisChainTrigerredBysingleFileChange()' instead since it more performant and more accurate as well
func DefinitionAnalysisSingleFile(fileName string, parsedFilesInWorkspace map[string]*parser.GroupStatementNode) (*checker.FileDefinition, []Error) {
	if len(parsedFilesInWorkspace) == 0 {
		return nil, nil
	}

	if parsedFilesInWorkspace[fileName] == nil {
		log.Printf("file '%s' is unavailable in the current workspace\n parsedFilesInWorkspace = %#v\n", fileName, parsedFilesInWorkspace)
		panic("file '" + fileName + "' is unavailable in the current workspace")
	}

	templateManager := checker.TEMPLATE_MANAGER
	templateManager.RemoveTemplateScopeAssociatedToFileName(fileName)

	_ = templateManager.BuildWorkspaceTemplateDefinition(parsedFilesInWorkspace)

	workspaceTemplateDefinition := templateManager.TemplateScopeToDefinition
	templateErrs := templateManager.AnalyzedDefinedTemplatesWithinFile[fileName].GetTemplateErrs()
	cycleErrs := templateManager.AnalyzedDefinedTemplatesWithinFile[fileName].CycleTemplateErrs
	partialFile := templateManager.AnalyzedDefinedTemplatesWithinFile[fileName].PartialFile

	file, errs := checker.DefinitionAnalysisFromPartialFile(partialFile, workspaceTemplateDefinition)

	errs = append(errs, templateErrs...)
	errs = append(errs, cycleErrs...)

	return file, errs
}

// Prefered function for computing the semantic analysis of a file in a workspace
// This also compute the semantic analysis of other files affected by the change initiated by 'fileName'
// So this process the semantic analysis of 'fileName' and other file affected by the change
func DefinitionAnalysisChainTrigerredBysingleFileChange(parsedFilesInWorkspace map[string]*parser.GroupStatementNode, fileName string) []FileAnalysisAndError {
	if len(parsedFilesInWorkspace) == 0 {
		return nil
	}

	if parsedFilesInWorkspace[fileName] == nil {
		log.Printf("file '%s' is unavailable in the current workspace\n parsedFilesInWorkspace = %#v\n", fileName, parsedFilesInWorkspace)
		panic("file '" + fileName + "' is unavailable in the current workspace")
	}

	templateManager := checker.TEMPLATE_MANAGER
	templateManager.RemoveTemplateScopeAssociatedToFileName(fileName)

	affectedFiles := templateManager.BuildWorkspaceTemplateDefinition(parsedFilesInWorkspace)
	affectedFiles[fileName] = true

	workspaceTemplateDefinition := templateManager.TemplateScopeToDefinition
	chainAnalysis := make([]FileAnalysisAndError, 0, len(affectedFiles))

	for fileNameAffected := range affectedFiles {
		partialFile := templateManager.AnalyzedDefinedTemplatesWithinFile[fileNameAffected].PartialFile

		file, errs := checker.DefinitionAnalysisFromPartialFile(partialFile, workspaceTemplateDefinition)

		templateErrs := templateManager.AnalyzedDefinedTemplatesWithinFile[fileNameAffected].GetTemplateErrs()
		cycleErrs := templateManager.AnalyzedDefinedTemplatesWithinFile[fileNameAffected].CycleTemplateErrs

		errs = append(errs, templateErrs...)
		errs = append(errs, cycleErrs...)

		group := FileAnalysisAndError{
			FileName: fileNameAffected,
			File:     file,
			Errs:     errs,
		}

		chainAnalysis = append(chainAnalysis, group)
	}

	return chainAnalysis
}

// Prefered function for computing the semantic analysis of many files change in a workspace
func DefinitionAnalysisChainTrigerredByBatchFileChange(parsedFilesInWorkspace map[string]*parser.GroupStatementNode, fileNames ...string) []FileAnalysisAndError {
	if len(parsedFilesInWorkspace) == 0 {
		return nil
	}

	templateManager := checker.TEMPLATE_MANAGER
	nameOfFileChanged := make(map[string]bool)

	for _, fileName := range fileNames {
		if parsedFilesInWorkspace[fileName] == nil {
			log.Printf("file '%s' is unavailable in the current workspace\n parsedFilesInWorkspace = %#v\n", fileName, parsedFilesInWorkspace)
			panic("file '" + fileName + "' is unavailable in the current workspace")
		}

		templateManager.RemoveTemplateScopeAssociatedToFileName(fileName)
		nameOfFileChanged[fileName] = true
	}

	affectedFiles := templateManager.BuildWorkspaceTemplateDefinition(parsedFilesInWorkspace)
	maps.Copy(affectedFiles, nameOfFileChanged)

	workspaceTemplateDefinition := templateManager.TemplateScopeToDefinition
	chainAnalysis := make([]FileAnalysisAndError, 0, len(affectedFiles))

	for fileNameAffected := range affectedFiles {
		partialFile := templateManager.AnalyzedDefinedTemplatesWithinFile[fileNameAffected].PartialFile

		file, errs := checker.DefinitionAnalysisFromPartialFile(partialFile, workspaceTemplateDefinition)

		templateErrs := templateManager.AnalyzedDefinedTemplatesWithinFile[fileNameAffected].GetTemplateErrs()
		cycleErrs := templateManager.AnalyzedDefinedTemplatesWithinFile[fileNameAffected].CycleTemplateErrs

		errs = append(errs, templateErrs...)
		errs = append(errs, cycleErrs...)

		group := FileAnalysisAndError{
			FileName: fileNameAffected,
			File:     file,
			Errs:     errs,
		}

		chainAnalysis = append(chainAnalysis, group)
	}

	return chainAnalysis
}

// Definition analysis for all files within a workspace.
// It should only be done after 'ParseFilesInWorkspace()' or similar
func DefinitionAnalisisWithinWorkspace(parsedFilesInWorkspace map[string]*parser.GroupStatementNode) []FileAnalysisAndError {
	if len(parsedFilesInWorkspace) == 0 {
		return nil
	}

	checker.TEMPLATE_MANAGER = checker.NewWorkspaceTemplateManager()
	templateManager := checker.TEMPLATE_MANAGER

	affectedFiles := templateManager.BuildWorkspaceTemplateDefinition(parsedFilesInWorkspace)
	for fileName := range parsedFilesInWorkspace {
		affectedFiles[fileName] = true
	}

	if len(affectedFiles) != len(parsedFilesInWorkspace) {
		log.Printf("count of files in workspace do not match number of files found during template analysis"+
			"\n len(affectedFiles) = %d ::: len(parsedFilesInWorkspace) = %d\n", len(affectedFiles), len(parsedFilesInWorkspace))
		panic("count of files in workspace do not match number of files found during template analysis")
	}

	workspaceTemplateDefinition := templateManager.TemplateScopeToDefinition
	chainAnalysis := make([]FileAnalysisAndError, 0, len(affectedFiles))

	for fileNameAffected := range affectedFiles {
		partialFile := templateManager.AnalyzedDefinedTemplatesWithinFile[fileNameAffected].PartialFile

		file, errs := checker.DefinitionAnalysisFromPartialFile(partialFile, workspaceTemplateDefinition)

		templateErrs := templateManager.AnalyzedDefinedTemplatesWithinFile[fileNameAffected].GetTemplateErrs()
		cycleErrs := templateManager.AnalyzedDefinedTemplatesWithinFile[fileNameAffected].CycleTemplateErrs

		errs = append(errs, templateErrs...)
		errs = append(errs, cycleErrs...)

		group := FileAnalysisAndError{
			FileName: fileNameAffected,
			File:     file,
			Errs:     errs,
		}

		chainAnalysis = append(chainAnalysis, group)
	}

	return chainAnalysis
}

func GoToDefinition(file *checker.FileDefinition, position lexer.Position) (fileNames []string, reachs []lexer.Range, err error) {
	definitions := checker.FindSourceDefinitionFromPosition(file, position)

	if len(definitions) == 0 {
		log.Println("token not found for definition")
		return nil, nil, errors.New("meaningful token not found for go-to definition")
	}

	fileNames = make([]string, 0, len(definitions))
	reachs = make([]lexer.Range, 0, len(definitions))

	for _, definition := range definitions {
		fileNames = append(fileNames, definition.FileName())
		reachs = append(reachs, definition.Range())
	}

	return fileNames, reachs, nil
}

func Hover(file *checker.FileDefinition, position lexer.Position) (string, lexer.Range) {
	definitions := checker.FindSourceDefinitionFromPosition(file, position)
	if len(definitions) == 0 {
		log.Println("definition not found for token at position, ", position)
		return "", lexer.EmptyRange()
	}

	if len(definitions) > 1 {
		typeStringified := "Multiple Source Found [lsp]"

		return typeStringified, lexer.EmptyRange()
	}

	definition := definitions[0]
	typeStringified, reach := checker.Hover(definition)

	if typeStringified == "" {
		log.Printf("definition exist, but type was not found\n definition = %#v\n", definition)
		panic("definition exist, but type was not found")
	}

	if file.FileName() != definition.FileName() {
		return typeStringified, lexer.EmptyRange()
	}

	return typeStringified, reach
}

func FoldingRange(rootNode *parser.GroupStatementNode) ([]*parser.GroupStatementNode, []*parser.CommentNode) {
	foldingGroups := make([]*parser.GroupStatementNode, 0, 10)
	foldingComments := make([]*parser.CommentNode, 0, 10)
	queue := make([]*parser.GroupStatementNode, 0, 10)

	queue = append(queue, rootNode)
	index := 0
	counter := 0

	for {
		if counter++; counter > 10_000 {
			panic("infinite loop while computing 'FoldingRange()'")
		}

		if index >= len(queue) { // index out of bound
			break
		}

		scope := queue[index]
		index++

		for _, statement := range scope.Statements {
			switch node := statement.(type) {
			case *parser.CommentNode:
				foldingComments = append(foldingComments, node)

			case *parser.GroupStatementNode:
				foldingGroups = append(foldingGroups, node)
				queue = append(queue, node)

			default: // do nothing
			}
		}
	}

	return foldingGroups, foldingComments
}

// Print in JSON format the AST node to the screen. Use a program like 'jq' for pretty formatting
func Print(node ...parser.AstNode) {
	str := parser.PrettyFormater(node)
	fmt.Println(str)
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
