package analyzer

import (
	// fmt"
	"errors"
	"github.com/yayolande/gota/lexer"
	"github.com/yayolande/gota/parser"
	"go/types"
	"log"
)

type templateHandler struct {
	fileNameToDefinition map[string]*FileDefinition // The file definition only contains accurate values for 'file.Functions' and 'file.TypeHints[root]'

	templatesDefinedInWorkspace map[string]*parser.GroupStatementNode
	multipleTemplateDefinition  map[string]bool
	templateInCyclicalCall      map[*parser.TemplateStatementNode]bool

	TemplateToDefinition map[*parser.GroupStatementNode]*TemplateDefinition
	TemplateToFileName   map[*parser.GroupStatementNode]string
	templateToOrigin     map[*parser.GroupStatementNode]*parser.GroupStatementNode

	callerStack []*parser.TemplateStatementNode
	callPath    map[string]bool

	maxDepth int
	Errs     []lexer.Error
}

func NewTemplateDefinitionHandler(parsedFilesInWorkspace map[string]*parser.GroupStatementNode) *templateHandler {
	handler := &templateHandler{
		fileNameToDefinition: make(map[string]*FileDefinition),

		templatesDefinedInWorkspace: make(map[string]*parser.GroupStatementNode),
		TemplateToFileName:          make(map[*parser.GroupStatementNode]string),
		TemplateToDefinition:        make(map[*parser.GroupStatementNode]*TemplateDefinition),
		templateToOrigin:            make(map[*parser.GroupStatementNode]*parser.GroupStatementNode),

		multipleTemplateDefinition: make(map[string]bool),
		templateInCyclicalCall:     make(map[*parser.TemplateStatementNode]bool),

		callPath:    make(map[string]bool),
		callerStack: make([]*parser.TemplateStatementNode, 0, 20),
		maxDepth:    20,

		Errs: make([]lexer.Error, 0),
	}

	// TODO: first sort 'parsedFilesInWorkspace' by fileName into an ordered array/slice
	// the goal is to have the same execution path at any re-execution

	for fileName, root := range parsedFilesInWorkspace {

		// 1. Create file definition
		file, globalVar, localVar := NewFileDefinition(fileName, root, nil)
		handler.fileNameToDefinition[fileName] = file

		goCode := root.ShortCut.CommentGoCode
		typs, _ := definitionAnalysisComment(goCode, root, file, globalVar, localVar) // this help to set 'file.Functions' and 'file.TypeHints' later only

		file.TypeHints[root] = typs[0]

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
				node, _ := templateFound.ControlFlow.(*parser.TemplateStatementNode)

				err := parser.NewParseError(node.TemplateName, errors.New("template already defined"))
				handler.Errs = append(handler.Errs, err)

				if templateFound != template {
					log.Printf("weirdly enough 'templateFound' is not similar to any 'template' available into the parent"+
						"\n templateFound = %#v \n template = %#v\n",
						templateFound, template)
					panic("weirdly enough 'templateFound' is not similar to any 'template' available into the parent")
				}
			}

			handler.TemplateToFileName[template] = fileName
			handler.templateToOrigin[template] = root

			_, found := handler.templatesDefinedInWorkspace[templateName]

			if found {
				handler.multipleTemplateDefinition[templateName] = true
				continue
			}

			handler.templatesDefinedInWorkspace[templateName] = template
		}
	}

	return handler
}

func (h templateHandler) FindTemplateWithinWorkspace(templateName string, rootOfTemplate *parser.GroupStatementNode) (*parser.GroupStatementNode, bool) {
	template, ok := h.templatesDefinedInWorkspace[templateName]
	if !ok {
		return nil, false
	}

	if h.multipleTemplateDefinition[templateName] {
		return template, true
	}

	return template, false
	// a. fetch the root node of this template (same file)
	// b. check if 'templateName' is defined in the same file
	// c. If not, return 'any template' and 'isUnique = false'
	// d. If template defined in same file, give it precedence

	// return template, isMultiple // return *parser.GroupStatementNode, bool
}

func (h templateHandler) MustGetTemplateStatement(template *parser.GroupStatementNode) *parser.TemplateStatementNode {
	if template.ControlFlow == nil {
		log.Printf("template cannot have 'ControlFlow' set to <nil>\n "+
			"template = %s\n templateHandler = %#v\n", template, h)
		panic("template cannot have 'ControlFlow' set to <nil>")
	}

	tmpl, ok := template.ControlFlow.(*parser.TemplateStatementNode)

	if !ok {
		log.Printf("expected a 'TemplateStatementNode' but found something else\n"+
			"template = %s\n templateHandler = %#v\n", template, h)
		panic("expected a 'TemplateStatementNode' but found something else")
	}

	return tmpl
}

func (h *templateHandler) markCallPathAsCyclicalError(templateCollision *parser.TemplateStatementNode) {
	if len(h.callerStack) != len(h.callPath)-1 {
		panic("length mismatch between 'callerStack' and 'callPath'")
	}

	if len(h.callerStack) < 2 {
		panic("cyclical call cannot be detected with only 0 or 1 element. At least 2 are needed")
	}

	foundCollidingTemplate := false
	indexStart := 0

	for ; indexStart < len(h.callerStack); indexStart++ {
		templateName := string(h.callerStack[indexStart].TemplateName.Value)
		templateCollisionName := string(templateCollision.TemplateName.Value)

		if templateName == templateCollisionName {
			foundCollidingTemplate = true
			break
		}
	}

	if !foundCollidingTemplate {
		log.Printf("caller said there is template name collision, "+
			"but 'cyclical' detector was unable to find it\n"+
			"handler = %#v\n", h)
		panic("caller said there is template name collision, but 'cyclical' detector was unable to find it")
	}

	if indexStart > 0 {
		originalCaller := h.callerStack[indexStart-1]

		if !h.templateInCyclicalCall[originalCaller] {
			errMsg := errors.New("this template call will enter an infinite depency call")
			err := parser.NewParseError(originalCaller.TemplateName, errMsg)
			h.Errs = append(h.Errs, err)

			h.templateInCyclicalCall[originalCaller] = true
		}
	}

	// TODO: Is this code below really necessary ? I want to remove it if not useful
	// Leaving it for now because I am still not sure 100%
	for index := indexStart; index < len(h.callerStack); index++ {
		originalCaller := h.callerStack[indexStart-1]

		if h.templateInCyclicalCall[originalCaller] {
			continue
		}

		// errMsg := errors.New("template call into infinite loop")
		errMsg := errors.New("template will enventually call itself")
		err := parser.NewParseError(originalCaller.TemplateName, errMsg)
		h.Errs = append(h.Errs, err)

		h.templateInCyclicalCall[originalCaller] = true
	}
}

func (h templateHandler) CreateTemplateDefinition(template *parser.GroupStatementNode, templateName string, typ types.Type) *TemplateDefinition {
	fileName := h.TemplateToFileName[template]

	if typ == nil {
		typ = TYPE_ANY.Type()
	}

	def := &TemplateDefinition{
		Node:      template,
		Name:      templateName,
		FileName:  fileName,
		Range:     template.Range,
		IsValid:   true,
		InputType: typ,
	}

	return def
}

func (h *templateHandler) BuildTemplateDefinition(startTemplate *parser.GroupStatementNode, templateName string) *TemplateDefinition {
	if startTemplate == nil {
		log.Printf("template to investigate is <nil>\n handler = %#v\n", h)
		panic("template to investigate is <nil>")
	}

	h.callPath[templateName] = true

	// TODO: what to do if 2 or more template have the same name ?

	// outterTemplates := make([]*TemplateDefinition, 0)
	outterTemplates := make(map[*parser.GroupStatementNode]*TemplateDefinition)

	listTemplatescall := startTemplate.ShortCut.TemplateCallUsed
	rootOfTemplate := h.templateToOrigin[startTemplate]

	for templateNameToVisit, templateCaller := range listTemplatescall {
		templateFound, isMultiple := h.FindTemplateWithinWorkspace(templateNameToVisit, rootOfTemplate)
		if templateFound == nil { // nothing found
			continue
		}

		if isMultiple { // many template outside current file only found
			typ := TYPE_ANY.Type()
			def := h.CreateTemplateDefinition(templateFound, templateNameToVisit, typ)

			outterTemplates[templateFound] = def
			// outterTemplates = append(outterTemplates, def)
			continue
		}

		oldDef := h.TemplateToDefinition[templateFound]
		if oldDef != nil {

			outterTemplates[templateFound] = oldDef
			// outterTemplates = append(outterTemplates, oldDef)
			continue
		}

		if h.callPath[templateNameToVisit] { // error, cycle found
			h.markCallPathAsCyclicalError(templateCaller)

			typ := TYPE_ANY.Type()
			def := h.CreateTemplateDefinition(templateFound, templateName, typ)

			h.TemplateToDefinition[startTemplate] = def

			outterTemplates[startTemplate] = def
			// outterTemplates = append(outterTemplates, def)
			continue
		}

		h.increaseDepth(templateCaller)
		def := h.BuildTemplateDefinition(templateFound, templateNameToVisit)
		h.decreaseDepth()

		h.TemplateToDefinition[startTemplate] = def
		outterTemplates[startTemplate] = def
		// outterTemplates = append(outterTemplates, def)
	}

	delete(h.callPath, templateName)

	// 3. from all template caller found, build the current template definition

	fileName := h.TemplateToFileName[startTemplate]
	file, globalVariables, localVariables := NewFileDefinition(fileName, startTemplate, outterTemplates)

	rootDefinition := h.fileNameToDefinition[fileName]
	if rootDefinition == nil {
		log.Printf("for some reason, root 'FileDefinition' for cannot be found for, "+fileName+
			"\n h.fileNameToDefinition = %#v \n file_tmp = %#v \n", h.fileNameToDefinition, file)
		panic("for some reason, root 'FileDefinition' for cannot be found for, " + fileName)
	}

	file.Functions = rootDefinition.Functions

	typs, _ := definitionAnalysisGroupStatement(startTemplate, startTemplate.Parent(), file, globalVariables, localVariables)
	// h.Errs = append(h.Errs, errs...)

	def := h.CreateTemplateDefinition(startTemplate, templateName, typs[0])
	h.TemplateToDefinition[startTemplate] = def

	return def
}

func (h *templateHandler) increaseDepth(templateNode *parser.TemplateStatementNode) {
	if len(h.callerStack) > h.maxDepth {
		log.Printf("max depth reached during depencies analysis of 'TemplateStatementNode'\n "+
			"h.callerStack = %q\n handler = %#v\n", h.callerStack, h)
		panic("max depth reached during depencies analysis of 'TemplateStatementNode'")
	}

	h.callerStack = append(h.callerStack, templateNode)
}

func (h *templateHandler) decreaseDepth() {
	size := len(h.callerStack)

	if size == 0 {
		log.Printf("no more element to unwind from the caller stack during depencies analysis for 'TemplateStatementNode'\n"+
			"size = %d\n handler = %#v\n", size, h)
		panic("no more element to unwind from the caller stack during depencies analysis for 'TemplateStatementNode'")
	}

	h.callerStack = h.callerStack[:size-1]
}
