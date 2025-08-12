package analyzer

import (
	// fmt"
	"errors"
	"github.com/yayolande/gota/lexer"
	"github.com/yayolande/gota/parser"
	"go/types"
	"log"
)

type ContainerTemplateDependencies struct {
	ListTemplateInCyclicDependencies  map[string]bool
	templateToFileNameDependencies    map[*parser.GroupStatementNode]map[string]bool
	fileNameWithCollidingTemplateName map[string]map[string]bool
	// templateDependencies type ???
}

func newContainerTemplateDependencies() *ContainerTemplateDependencies {
	return &ContainerTemplateDependencies{
		ListTemplateInCyclicDependencies:  make(map[string]bool),
		templateToFileNameDependencies:    make(map[*parser.GroupStatementNode]map[string]bool),
		fileNameWithCollidingTemplateName: make(map[string]map[string]bool),
	}
}

// Static VS Dynamics errors
// Static Errors: errors within a file that are not affected by other files change within the workspace
// Dynamics Errors: errors within a file that are affected by other files change within the workspace
// NOTE: Static & Dynamics (errors) distinction is important because it determine how deep a file must be parser
type ContainerFileAnalysisForDefinedTemplates struct {
	FileName    string
	PartialFile *FileDefinition

	TemplateErrs      map[*parser.GroupStatementNode][]lexer.Error // Static Errors
	CycleTemplateErrs []lexer.Error                                // Dynamics Errors
}

func (c ContainerFileAnalysisForDefinedTemplates) GetTemplateErrs() []lexer.Error {
	var errs []lexer.Error

	for _, localErrs := range c.TemplateErrs {
		errs = append(errs, localErrs...)
	}

	return errs
}

type ContainerFileAnalysisForRootTemplate struct {
	FileName     string
	CompleteFile *FileDefinition
	RootErrs     []lexer.Error
}

type FileAnalyzedStorage struct {
	FileName string
	File     *FileDefinition
	Errs     []lexer.Error
}

type WorkspaceTemplateManager struct {
	TemplateScopeToFileName   map[*parser.GroupStatementNode]string
	TemplateScopeToDefinition map[*parser.GroupStatementNode]*TemplateDefinition

	// affectedFiles                      map[string]bool
	AnalyzedDefinedTemplatesWithinFile map[string]ContainerFileAnalysisForDefinedTemplates // rename to 'analyzedTemplatesFromFile' or 'analyzedDefinedTemplatesWithinFile'
	AnalyzedMainTemplateInFile         map[string]ContainerFileAnalysisForRootTemplate
	// analyzedFileRootTemplate map[string]ContainerRootTemplateFileAnalysis // ContainerTopLevelFileAnalysis
	// analyzedMainTemplateInFile
	//
	// ContainerFileAnalysisForRootTemplate
	// ContainerFileAnalysisForDefinedTemplates
	//
	// ContainerTemplatesAnalysisWithinFile

	TemplateDependencies ContainerTemplateDependencies
}

func (t *WorkspaceTemplateManager) ResetTemplateDependencies() {
	// t.TemplateDependencies = ContainerTemplateDependencies{}
	t.TemplateDependencies = *newContainerTemplateDependencies()
}

func NewWorkspaceTemplateManager() *WorkspaceTemplateManager {
	manager := &WorkspaceTemplateManager{
		TemplateScopeToFileName:   make(map[*parser.GroupStatementNode]string),
		TemplateScopeToDefinition: make(map[*parser.GroupStatementNode]*TemplateDefinition),

		AnalyzedMainTemplateInFile:         make(map[string]ContainerFileAnalysisForRootTemplate),
		AnalyzedDefinedTemplatesWithinFile: make(map[string]ContainerFileAnalysisForDefinedTemplates),
		TemplateDependencies:               *newContainerTemplateDependencies(),
	}

	return manager
}

func (h *WorkspaceTemplateManager) RemoveTemplateScopeAssociatedToFileName(sourceFileName string) {
	for templateScopeToDelete, targetFileName := range h.TemplateScopeToFileName {

		if sourceFileName == targetFileName {
			delete(h.TemplateScopeToDefinition, templateScopeToDelete)
			delete(h.TemplateScopeToFileName, templateScopeToDelete)

			delete(h.AnalyzedDefinedTemplatesWithinFile, sourceFileName)
		}
	}
}

// TODO:
// func name: rebuildWorkspaceTemplateDefinition() ?
func (h *WorkspaceTemplateManager) BuildWorkspaceTemplateDefinition(parsedFilesInWorkspace map[string]*parser.GroupStatementNode) map[string]bool {
	handler := NewTemplateBuilder(parsedFilesInWorkspace, h)

	h.ResetTemplateDependencies()

	// 1. Main process
	//
	// Get previous template definition for unchanged files
	nameOfTemplateModified := make(map[string]bool)

	for template := range handler.TemplateToFileName {
		def := handler.templateManager.TemplateScopeToDefinition[template]
		if def == nil {
			nameOfTemplateModified[template.TemplateName()] = true
			continue
		}

		handler.TemplateToDefinition[template] = def
	}

	// compute the template definition of file that have change or are affected by the change

	for template, visited := range handler.TemplateVisited {
		if visited {
			continue
		}

		templateName := template.TemplateName()
		nameOfTemplateModified[templateName] = true

		handler.BuildTemplateDefinition(template, templateName)
	}

	// error checking
	if len(handler.TemplateToFileName) != len(handler.TemplateToDefinition) {
		log.Printf("length mismatch between template existing and template analyzed\n"+
			" handler.TemplateToFileName = %#v\n handler.TemplateToDefinition = %#v\n", handler.TemplateToFileName, handler.TemplateToDefinition)
		panic("length mismatch between template existing and template analyzed")
	}

	if len(handler.TemplateVisited) != len(handler.TemplateToDefinition) {
		log.Printf("all template visited in the workspace should have a 'TemplateDefinition'\n"+
			" len(templateVisited) = %d\n len(TemplateToDefinition) = %d\n templateVisited = %#v\n TemplateToDefinition = %#v\n",
			len(handler.TemplateVisited), len(handler.TemplateToDefinition), handler.TemplateVisited, handler.TemplateToDefinition)
		panic("all template visited in the workspace should have a 'TemplateDefinition'")
	}

	// delete old data from template manager and only keep the frest one
	handler.templateManager.TemplateScopeToDefinition = handler.TemplateToDefinition
	handler.templateManager.TemplateScopeToFileName = handler.TemplateToFileName

	// 2. Find files affected by the change
	//
	// Find all files affected by the template change (file that call the said template)
	// TODO: WIP

	// TODO: At a top level, change of template call must trigger a rebuild
	// However, in local template definition, rebuilding the root template (file) is overkill
	// We need a strategy that keep data concerning errors of 'root template' and 'childreen templates' associated to the same file
	for fileName, root := range parsedFilesInWorkspace {

		// Find all template call within the root file
		for _, templateCall := range root.ShortCut.TemplateCallUsed {
			templateNameCalled := string(templateCall.TemplateName.Value)

			if nameOfTemplateModified[templateNameCalled] {
				handler.addFileAffected(fileName)
			}
		}

		// Find all template call within the templates defined in root file
		for _, templateScope := range root.ShortCut.TemplateDefined {
			for _, templateCall := range templateScope.ShortCut.TemplateCallUsed {
				templateNameCalled := string(templateCall.TemplateName.Value)

				if nameOfTemplateModified[templateNameCalled] {
					handler.addFileAffected(fileName)
				}
			}
		}
	}

	return handler.affectedFiles
}

type templateStatementCallStack struct {
	templateCall        *parser.TemplateStatementNode
	parentTemplateCall  *parser.GroupStatementNode
	templateNameToVisit string
	isDependencyError   bool // rename to 'isDependencyError'
}

type workspaceTemplateBuilder struct {
	// TODO: REMOVE THIS FIELD when appropriate
	fileNameToDefinition map[string]*FileDefinition // The file definition only contains accurate values for 'file.Functions' and 'file.TypeHints[root]'

	multipleTemplateFromTemplateName map[string][]*parser.GroupStatementNode
	// templateInCyclicalCall     map[*parser.TemplateStatementNode]bool

	// New data
	TemplateToDefinition map[*parser.GroupStatementNode]*TemplateDefinition
	TemplateToFileName   map[*parser.GroupStatementNode]string // All Defined template in workspace
	TemplateVisited      map[*parser.GroupStatementNode]bool   // Template traversal is based on this

	templateManager *WorkspaceTemplateManager

	// old data
	// callerStack []*parser.GroupStatementNode
	callerStack []templateStatementCallStack
	callPath    map[string]bool

	maxDepth int
	// Errs     []lexer.Error

	affectedFiles map[string]bool
}

func NewTemplateBuilder(parsedFilesInWorkspace map[string]*parser.GroupStatementNode, templateHandler *WorkspaceTemplateManager) *workspaceTemplateBuilder {
	if templateHandler == nil {
		panic("global template Handler/Manager must be defined at all time, but it wasn't the case")
	}

	handler := &workspaceTemplateBuilder{
		fileNameToDefinition: make(map[string]*FileDefinition),

		TemplateToFileName:   make(map[*parser.GroupStatementNode]string),
		TemplateToDefinition: make(map[*parser.GroupStatementNode]*TemplateDefinition),
		TemplateVisited:      make(map[*parser.GroupStatementNode]bool),

		multipleTemplateFromTemplateName: make(map[string][]*parser.GroupStatementNode),
		// templateInCyclicalCall:     make(map[*parser.TemplateStatementNode]bool),
		templateManager: templateHandler,

		// callerStack: make([]*parser.GroupStatementNode, 0, 20),
		callPath:    make(map[string]bool),
		callerStack: make([]templateStatementCallStack, 0, 20),
		maxDepth:    20,

		// Errs:          make([]lexer.Error, 0),
		affectedFiles: make(map[string]bool),
	}

	// Flush previous computation to rebuild from fresh
	handler.templateManager.AnalyzedDefinedTemplatesWithinFile = make(map[string]ContainerFileAnalysisForDefinedTemplates)
	handler.templateManager.TemplateDependencies = *newContainerTemplateDependencies()
	//handler.templateManager.TemplateDependencies = ContainerTemplateDependencies{}

	// extract all template scope node from the root file
	for fileName, root := range parsedFilesInWorkspace {
		if root == nil {
			continue
		}

		// 1. Create file definition if it doesn't already exists
		_, ok := handler.templateManager.AnalyzedDefinedTemplatesWithinFile[fileName]
		if !ok {
			file, globalVar, localVar := NewFileDefinition(fileName, root, nil)

			goCode := root.ShortCut.CommentGoCode
			definitionAnalysisComment(goCode, root, file, globalVar, localVar) // this help to set 'file.Functions' and 'file.TypeHints' for later use

			handler.templateManager.AnalyzedDefinedTemplatesWithinFile[fileName] = ContainerFileAnalysisForDefinedTemplates{
				PartialFile:       file,
				FileName:          fileName,
				TemplateErrs:      make(map[*parser.GroupStatementNode][]lexer.Error),
				CycleTemplateErrs: make([]lexer.Error, 0),
			}
		}

		handler.fileNameToDefinition[fileName] = handler.templateManager.AnalyzedDefinedTemplatesWithinFile[fileName].PartialFile

		handler.templateManager.TemplateDependencies.fileNameWithCollidingTemplateName[fileName] = make(map[string]bool)

		// 2. Register all defined templates within a file
		localTemplateDefinition := make(map[string]*parser.GroupStatementNode)

		for templateName, template := range root.ShortCut.TemplateDefined {

			if !template.IsTemplate() {
				log.Printf("expected a template but found something else\n"+
					"GroupStatementNode = %s\n", template)
				panic("expected a template but found something else")
			}

			templateFound := localTemplateDefinition[templateName]
			if templateFound == nil {
				localTemplateDefinition[templateName] = template
			} else {
				err := parser.NewParseError(template.TemplateNameToken(), errors.New("template already defined"))
				// handler.Errs = append(handler.Errs, err)

				handler.templateManager.AnalyzedDefinedTemplatesWithinFile[fileName].TemplateErrs[template] =
					append(handler.templateManager.AnalyzedDefinedTemplatesWithinFile[fileName].TemplateErrs[template], err)

				if templateFound != template {
					log.Printf("weirdly enough 'templateFound' is not similar to any 'template' available into the parent"+
						"\n templateFound = %#v \n template = %#v\n",
						templateFound, template)
					panic("weirdly enough 'templateFound' is not similar to any 'template' available into the parent")
				}
			}

			handler.TemplateToFileName[template] = fileName
			handler.TemplateVisited[template] = false

			handler.multipleTemplateFromTemplateName[templateName] =
				append(handler.multipleTemplateFromTemplateName[templateName], template)

			handler.templateManager.TemplateDependencies.templateToFileNameDependencies[template] = make(map[string]bool)
		}
	}

	// Build fileName collision dependencies with template name
	for _, templatesWithSameName := range handler.multipleTemplateFromTemplateName {

		for _, templateScope := range templatesWithSameName {
			fileName := handler.TemplateToFileName[templateScope]

			for _, collidingTemplate := range templatesWithSameName {
				if collidingTemplate == templateScope {
					continue
				}

				collidingFileName := handler.TemplateToFileName[collidingTemplate]

				handler.templateManager.TemplateDependencies.
					fileNameWithCollidingTemplateName[fileName][collidingFileName] = true

			}
		}

	}

	// Delete analyzed file that are not in the workspace anymore
	for fileName := range handler.templateManager.AnalyzedDefinedTemplatesWithinFile {
		_, ok := parsedFilesInWorkspace[fileName]

		if ok {
			continue
		}

		delete(handler.templateManager.AnalyzedDefinedTemplatesWithinFile, fileName)
	}

	if len(handler.templateManager.AnalyzedDefinedTemplatesWithinFile) != len(parsedFilesInWorkspace) {
		log.Printf("size mismatch between 'AnalyzedDefinedTemplatesWithinFile' and the available file in the workspace ! All of them must be partially analyzed at the start\n"+
			" len(AnalyzedDefinedTemplatesWithinFile) = %d\n len(parsedFilesInWorkspace) = %d\n", len(handler.templateManager.AnalyzedDefinedTemplatesWithinFile), len(parsedFilesInWorkspace))
		panic("size mismatch between 'AnalyzedDefinedTemplatesWithinFile' and the available file in the workspace ! All of them must be partially analyzed at the start")
	}

	if len(handler.TemplateVisited) != len(handler.TemplateToFileName) {
		log.Printf("size of template to visit must be equal to all the available template in the workspace\n"+
			" len(TemplateVisited) = %d\n len(handler.TemplateToFileName) = %d\n", len(handler.TemplateVisited), len(handler.TemplateToFileName))
		panic("size of template to visit must be equal to all the available template in the workspace")
	}

	return handler
}

func (h workspaceTemplateBuilder) FindTemplateWithinWorkspace(templateName string, rootOfTemplate *parser.GroupStatementNode) (*parser.GroupStatementNode, bool) {
	var templateFound *parser.GroupStatementNode = nil
	var isMultiple = false

	for template := range h.TemplateToFileName {
		if templateName != template.TemplateName() {
			continue
		}

		templateFound = template
	}

	if templateFound == nil {
		isMultiple = false

		return nil, isMultiple
	}

	if len(h.multipleTemplateFromTemplateName[templateName]) > 1 {
		isMultiple = true

		return templateFound, isMultiple
	}

	isMultiple = false
	return templateFound, isMultiple
	// a. fetch the root node of this template (same file)
	// b. check if 'templateName' is defined in the same file
	// c. If not, return 'any template' and 'isUnique = false'
	// d. If template defined in same file, give it precedence

	// return template, isMultiple // return *parser.GroupStatementNode, bool
}

func (h *workspaceTemplateBuilder) markCallPathAsCyclicalError(templateCollision *parser.TemplateStatementNode) {
	sizeCallStack := len(h.callerStack)

	if len(h.callerStack) != len(h.callPath) {
		log.Printf("length mismatch between 'callerStack' and 'callPath'\n callPath = %#v\n callerStack = %#v", h.callPath, h.callerStack)
		panic("length mismatch between 'callerStack' and 'callPath'")
	}

	if sizeCallStack < 1 {
		log.Println("cyclical call with less element than expect. callPath = ", h.callPath)
		panic("cyclical call cannot be detected with only 0 element. At least 1 is needed")
	}

	foundCollidingTemplate := false
	indexStartCollision := 0

	templateCollisionName := string(templateCollision.TemplateName.Value)

	for index := range sizeCallStack {
		templateName := h.callerStack[index].parentTemplateCall.TemplateName() // location of the template call

		if templateName == templateCollisionName {
			foundCollidingTemplate = true
			indexStartCollision = index

			break
		}
	}

	if !foundCollidingTemplate {
		log.Printf("caller said there is template name collision, "+
			"but 'cyclical' detector was unable to find it\n"+
			"handler = %#v\n", h)
		panic("caller said there is template name collision, but 'cyclical' detector was unable to find it")
	}

	// Place a warning before the entering the collision site ???
	// c. beget an error for the entry call to cyclical dependency if possible
	//
	if indexStartCollision > 0 && !h.callerStack[indexStartCollision-1].isDependencyError {
		frontierStackToCyclicalCall := h.callerStack[indexStartCollision-1]
		frontierToCyclicalCall := h.callerStack[indexStartCollision-1].templateCall

		// errMsg := errors.New("this template call will enter an infinite loop call")
		errMsg := errors.New("entering an infinite call loop")
		err := parser.NewParseError(frontierToCyclicalCall.TemplateName, errMsg)

		fileName := h.TemplateToFileName[frontierStackToCyclicalCall.parentTemplateCall]
		partialFile := h.templateManager.AnalyzedDefinedTemplatesWithinFile[fileName]

		partialFile.CycleTemplateErrs = append(partialFile.CycleTemplateErrs, err)
		h.templateManager.AnalyzedDefinedTemplatesWithinFile[fileName] = partialFile

		h.callerStack[indexStartCollision-1].isDependencyError = true
	}

	for index := indexStartCollision; index < sizeCallStack-1; index++ {
		// a. Add parentTemplateCall to cyclical error
		//
		currentTemplateHolderName := h.callerStack[index].parentTemplateCall.TemplateName()
		h.templateManager.TemplateDependencies.ListTemplateInCyclicDependencies[currentTemplateHolderName] = true

		// b. mark all templateCall between 'indexStartCollision' to 'last element' within callerStack as errorneous
		// b. then produce the file error related to cyclic templateCall if and only if the element of the stack were not previously marked before
		//
		if !h.callerStack[index].isDependencyError {
			// errMsg := errors.New("template call into infinite loop")
			// errMsg := errors.New("template will enventually call itself")
			errMsg := errors.New("continuing cyclical template call")
			err := parser.NewParseError(h.callerStack[index].templateCall.TemplateName, errMsg)

			fileName := h.TemplateToFileName[h.callerStack[index].parentTemplateCall]
			partialFile := h.templateManager.AnalyzedDefinedTemplatesWithinFile[fileName]

			partialFile.CycleTemplateErrs = append(partialFile.CycleTemplateErrs, err)
			h.templateManager.AnalyzedDefinedTemplatesWithinFile[fileName] = partialFile
		}

		h.callerStack[index].isDependencyError = true
	}

	// d. verify special error when a 'templateCall' call himself (its parentTemplate)
	// e.g. {{ define "tmpl_name" }} {{ template "tmpl_name" . }} {{ end }}
	// handle last element on the callerStack personally
	// in order to know if the last element call itself or not
	// ie. Does the template call himself

	lastInStack := h.callerStack[sizeCallStack-1]
	h.templateManager.TemplateDependencies.ListTemplateInCyclicDependencies[lastInStack.parentTemplateCall.TemplateName()] = true

	if lastInStack.parentTemplateCall.TemplateName() == templateCollisionName {
		errMsg := errors.New("template calling itself")
		err := parser.NewParseError(lastInStack.templateCall.TemplateName, errMsg)

		fileName := h.TemplateToFileName[lastInStack.parentTemplateCall]
		partialFile := h.templateManager.AnalyzedDefinedTemplatesWithinFile[fileName]

		partialFile.CycleTemplateErrs = append(partialFile.CycleTemplateErrs, err)
		h.templateManager.AnalyzedDefinedTemplatesWithinFile[fileName] = partialFile

	} else {
		// errMsg := errors.New("template call into infinite loop")
		// errMsg := errors.New("template will enventually call itself")
		errMsg := errors.New("continuing cyclical template call")
		err := parser.NewParseError(lastInStack.templateCall.TemplateName, errMsg)

		fileName := h.TemplateToFileName[lastInStack.parentTemplateCall]
		partialFile := h.templateManager.AnalyzedDefinedTemplatesWithinFile[fileName]

		partialFile.CycleTemplateErrs = append(partialFile.CycleTemplateErrs, err)
		h.templateManager.AnalyzedDefinedTemplatesWithinFile[fileName] = partialFile
	}
}

// TODO: shouldn't this function be located near 'TemplateDefinition' type ????
func (h workspaceTemplateBuilder) CreateTemplateDefinition(template *parser.GroupStatementNode, templateName string, typ types.Type) *TemplateDefinition {
	fileName := h.TemplateToFileName[template]

	if typ == nil {
		typ = TYPE_ANY.Type()
	}

	def := NewTemplateDefinition(templateName, fileName, template, template.Range(), typ, true)

	return def
}

func (h *workspaceTemplateBuilder) BuildTemplateDefinition(templateScope *parser.GroupStatementNode, templateName string) *TemplateDefinition {
	if templateScope == nil {
		log.Printf("template to investigate is <nil>\n handler = %#v\n", h)
		panic("template to investigate is <nil>")
	}

	// DEBUG
	//
	log.Printf("=> special template name = %s\n\n", templateScope.TemplateName())
	for _, templateName := range templateScope.ShortCut.TemplateCallUsed {
		log.Printf("--> call to :: %s\n", templateName)
	}

	log.Println()
	log.Println()
	//
	// END DEBUG

	currentScopeFilenameDependency := make(map[string]bool)

	// START experimental
	h.TemplateVisited[templateScope] = true
	// END experimental

	h.callPath[templateName] = true
	h.increaseDepth(nil, nil, "") // nasty trick since 'defer' do not work withing 'for loop'

	outterTemplates := make(map[*parser.GroupStatementNode]*TemplateDefinition)

	for _, templateCall := range templateScope.ShortCut.TemplateCallUsed {
		templateNameToVisit := string(templateCall.TemplateName.Value)

		h.decreaseDepth() // continuation of the stack trick since 'defer' do not work within loop
		h.increaseDepth(templateScope, templateCall, templateNameToVisit)

		templateFound, isMultiple := h.FindTemplateWithinWorkspace(templateNameToVisit, nil)

		if templateFound == nil { // nothing found
			// TODO: handler error template not found here
			// For better locality of behaviour ???
			continue
		}

		fileName := h.TemplateToFileName[templateScope]
		currentScopeFilenameDependency[fileName] = true

		if h.callPath[templateNameToVisit] { // error, cycle found
			h.markCallPathAsCyclicalError(templateCall)

			anyTyp := TYPE_ANY.Type()
			anyDef := h.CreateTemplateDefinition(templateFound, templateName, anyTyp)

			outterTemplates[templateFound] = anyDef
			continue
		}

		if h.templateManager.TemplateDependencies.ListTemplateInCyclicDependencies[templateNameToVisit] {
			errMsg := errors.New("entering an infinite call loop [to review]")
			err := parser.NewParseError(templateCall.TemplateName, errMsg)

			fileName := h.TemplateToFileName[templateScope]
			partialFile := h.templateManager.AnalyzedDefinedTemplatesWithinFile[fileName]

			partialFile.CycleTemplateErrs = append(partialFile.CycleTemplateErrs, err)
			h.templateManager.AnalyzedDefinedTemplatesWithinFile[fileName] = partialFile
		}

		if isMultiple { // many template outside current file only found
			anyTyp := TYPE_ANY.Type()
			anyDef := h.CreateTemplateDefinition(templateFound, templateNameToVisit, anyTyp)

			outterTemplates[templateFound] = anyDef
			continue
		}

		if h.TemplateVisited[templateFound] {
			def := h.TemplateToDefinition[templateFound]
			if def == nil {
				log.Printf("during template dependency analysis, a visited template cannot have a <nil> 'TemplateDefinition'\n"+
					"template in question = %#v\n", templateFound)
				panic("during template dependency analysis, a visited template cannot have a <nil> 'TemplateDefinition'")
			}

			outterTemplates[templateFound] = def
			continue
		}

		/*
			oldDef := h.TemplateToDefinition[templateFound]
			if oldDef != nil {

				outterTemplates[templateFound] = oldDef
				continue
			}
		*/

		def := h.BuildTemplateDefinition(templateFound, templateNameToVisit)

		outterTemplates[templateFound] = def
	}

	delete(h.callPath, templateName)
	h.decreaseDepth()

	// resolve dependencies found
	h.templateManager.TemplateDependencies.templateToFileNameDependencies[templateScope] = currentScopeFilenameDependency

	//
	// 3. from all template caller found, build the current template definition
	//

	// a. Do not compute unchanged files, this was only needed to build template call dependency but not the definition
	if def := h.TemplateToDefinition[templateScope]; def != nil {
		return def
	}

	// b. Only compute file that have changed
	fileName := h.TemplateToFileName[templateScope]

	file := h.fileNameToDefinition[fileName]
	if file == nil {
		log.Printf("template dependency analyzer was unable to find the partial file definition for %s\n"+
			" templateName = %s\n templateScope = %#v\n", fileName, templateName, templateScope)
		panic("template dependency analyzer was unable to find the partial file definition for " + fileName)
	}

	for template, templateDef := range outterTemplates {
		file.templates[template.TemplateName()] = templateDef
	}

	// swap and keep reference to the true root of the file
	// this is required because late inference of variable type use the root value
	// and since this analysis will not go further up than 'templateScope'
	// we temporarily assign it as root, to force the variable type analysis at that moment
	originalRoot := file.root
	file.root = templateScope

	globalVariables, localVariables := NewGlobalAndLocalVariableDefinition(templateScope, templateScope.Parent(), fileName)
	typ, errs := definitionAnalysisGroupStatement(templateScope, templateScope.Parent(), file, globalVariables, localVariables)

	file.root = originalRoot

	h.templateManager.AnalyzedDefinedTemplatesWithinFile[fileName].TemplateErrs[templateScope] = errs

	def := h.CreateTemplateDefinition(templateScope, templateName, typ[0])

	h.TemplateToDefinition[templateScope] = def
	h.TemplateVisited[templateScope] = true

	return def
}

func (h *workspaceTemplateBuilder) increaseDepth(parentTemplateCall *parser.GroupStatementNode, templateCall *parser.TemplateStatementNode, templateNameToVisit string) {
	if len(h.callerStack) > h.maxDepth {
		log.Printf("max depth reached during depencies analysis of 'TemplateStatementNode'\n "+
			"h.callerStack = %#v\n handler = %#v\n", h.callerStack, h)
		panic("max depth reached during depencies analysis of 'TemplateStatementNode'")
	}

	newStack := templateStatementCallStack{
		parentTemplateCall:  parentTemplateCall,
		templateCall:        templateCall,
		templateNameToVisit: templateNameToVisit,
		isDependencyError:   false,
	}

	h.callerStack = append(h.callerStack, newStack)
}

func (h *workspaceTemplateBuilder) decreaseDepth() {
	size := len(h.callerStack)

	if size == 0 {
		log.Printf("no more element to unwind from the caller stack during depencies analysis for 'TemplateStatementNode'\n"+
			"size = %d\n handler = %#v\n", size, h)
		panic("no more element to unwind from the caller stack during depencies analysis for 'TemplateStatementNode'")
	}

	h.callerStack = h.callerStack[:size-1]
}

func (h *workspaceTemplateBuilder) addFileAffected(fileName string) {
	if h.affectedFiles[fileName] {
		return
	}

	h.affectedFiles[fileName] = true
}
