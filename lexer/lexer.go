package lexer

import (
	"bytes"
	"errors"
	"log"
	"regexp"
)

// ----------------------
// Lexer Types definition
// ----------------------

type Position struct {
	Line      int
	Character int
}

type Range struct {
	Start Position
	End   Position
}

func (r Range) Contains(pos Position) bool {
	if r.Start.Line > pos.Line {
		return false
	}

	if r.End.Line < pos.Line {
		return false
	}

	if r.Start.Line == pos.Line && pos.Character < r.Start.Character {
		return false
	}

	if r.End.Line == pos.Line && pos.Character >= r.End.Character {
		return false
	}

	return true
}

func (r Range) IsEmpty() bool {
	if r.Start.Line > 0 {
		return false
	} else if r.Start.Character > 0 {
		return false
	} else if r.End.Line > 0 {
		return false
	} else if r.End.Character > 0 {
		return false
	}

	return true
}

func EmptyRange() Range {
	return Range{}
}

type Kind int

type StreamToken struct {
	Tokens                  []Token
	Err                     *LexerError
	rng                     Range
	IndexFirstEqualOperator int
}

func (s StreamToken) IsEmpty() bool {
	if len(s.Tokens) == 0 {
		panic("token stream must at least have an 'EOL' token")
	}

	if len(s.Tokens) == 1 && s.Tokens[0].ID == EOL {
		return true
	}

	return false
}

func (s StreamToken) String() string {
	size := len(s.Tokens)

	if size == 0 {
		panic("token stream must at least have an 'EOL' token")
	} else if token := s.Tokens[size-1]; token.ID != EOL {
		panic("token stream must be terminated by an 'EOL' token")
	}

	str := ""
	for index := range size - 1 { // ignore last #EOL token
		tok := s.Tokens[index]

		piece := string(tok.Value)
		if tok.ID == STRING {
			// piece = `"` + piece + `"`
			// piece = "\"" + piece + "\""
			// piece = "\\\"" + piece + "\\\""
			piece = "`" + piece + "`"
		}

		str = str + " " + piece
	}

	return str[1:]
}

type Token struct {
	ID    Kind
	Range Range
	Value []byte
}

func NewToken(id Kind, reach Range, val []byte) *Token {
	fresh := &Token{
		ID:    id,
		Range: reach,
		Value: val,
	}

	return fresh
}

func CloneToken(old *Token) *Token {
	if old == nil {
		return nil
	}

	fresh := &Token{
		ID:    old.ID,
		Range: old.Range,
		Value: bytes.Clone(old.Value),
	}

	return fresh
}

func NewStreamToken(tokens []Token, err *LexerError, reach Range, loc int) *StreamToken {
	stream := &StreamToken{
		Tokens:                  tokens,
		Err:                     err,
		rng:                     reach,
		IndexFirstEqualOperator: loc,
	}

	return stream
}

type LexerError struct {
	Err   error
	Range Range
	Token *Token
}

func (l LexerError) GetError() string {
	return l.Err.Error()
}

func (l LexerError) GetRange() Range {
	return l.Range
}

type Error interface {
	GetError() string
	GetRange() Range
	String() string
}

// Tokenize the source code provided by 'content'.
// Each template pair delimitator ('{{' and '}}') represent an instruction of statement.
// Each source code instruction is tokenized separately, and the output are tokens representing the instruction.
// Every tokens representing an instruction always end by a 'EOL' tokens
// To sum up, the lexer/tokenizer return an array of token stream representing all instruction inside a file
func Tokenize(content []byte) (file []*StreamToken, errs []Error) {
	if len(content) == 0 {
		return nil, nil
	}

	templateCodes, templatePositions := extractTemplateCode(content)

	if templateCodes == nil {
		return nil, nil
	}

	var lineEndToken Token

	for i := range templateCodes {
		code := templateCodes[i]
		position := templatePositions[i]

		stream, tokenErrs := tokenizeLine(code, position)

		if stream == nil {
			log.Printf("unexpected <nil> token stream found at end of tokenizer process\n line = %q\n fileContent = %q\n", code, content)
			panic("unexpected <nil> token stream found at end of tokenizer process")
		}

		lineEndToken = Token{ID: EOL, Value: []byte("#EOL"), Range: position}
		stream.Tokens = append(stream.Tokens, lineEndToken)

		errs = append(errs, tokenErrs...)
		file = append(file, stream)
	}

	return file, errs
}

func extractTemplateCode(content []byte) ([][]byte, []Range) {
	if len(content) == 0 {
		return nil, nil
	}

	var ORIGINAL_CONTENT = content
	var CLONED_CONTENT = bytes.Clone(content)
	content = CLONED_CONTENT

	var templateCode [][]byte
	var templatePositions []Range
	var insideTemplate []byte

	captureLonelyTemplateDelimitator := regexp.MustCompile("{{|}}")
	// captureTemplateStatementOnly := regexp.MustCompile("(?:{{(?:[^{}]|[\n\r\t])*?}})")
	captureTemplateStatementOnly := regexp.MustCompile("(?:{{(?:[^{}]|[^{}]{|[^{}]}|[^{}]{}|[^{}]}{|[\n\r\t])*?}})")

	// TODO: line shouldn't start at '0' but '1' instead
	currentLine := 0
	currentColumn := 0

	var loc, loneLoc []int
	var templatePosition Range

	for {
		loneLoc = captureLonelyTemplateDelimitator.FindIndex(content)
		loc = captureTemplateStatementOnly.FindIndex(content)

		if loc == nil {
			// Do some checking before break out of the loop
			for {
				if loneLoc != nil {
					templatePosition = convertRangeIndexToTextEditorPosition(content, loneLoc, currentLine, currentColumn)

					templatePositions = append(templatePositions, templatePosition)
					templateCode = append(templateCode, content[loneLoc[0]:loneLoc[1]])

					currentLine = templatePosition.End.Line
					currentColumn = templatePosition.End.Character + 1
					content = content[loneLoc[1]:]

					loneLoc = captureLonelyTemplateDelimitator.FindIndex(content)
					continue
				}

				break
			}

			break
		}

		// A lone delimitator has been found in the wild, do something
		for {
			if loneLoc[0] < loc[0] {
				templatePosition = convertRangeIndexToTextEditorPosition(content, loneLoc, currentLine, currentColumn)

				templatePositions = append(templatePositions, templatePosition)
				templateCode = append(templateCode, content[loneLoc[0]:loneLoc[1]])

				currentLine = templatePosition.End.Line
				currentColumn = templatePosition.End.Character + 1
				content = content[loneLoc[1]:]

				loneLoc = captureLonelyTemplateDelimitator.FindIndex(content)
				loc = captureTemplateStatementOnly.FindIndex(content)
				continue
			}

			break
		}

		templatePosition = convertRangeIndexToTextEditorPosition(content, loc, currentLine, currentColumn)

		currentLine = templatePosition.End.Line
		currentColumn = templatePosition.End.Character + 1

		// Trim '{{' and '}}'
		insideTemplate = content[loc[0]+2 : loc[1]-2]

		templatePosition.Start.Character += 2
		templatePosition.End.Character -= 1

		templatePositions = append(templatePositions, templatePosition)
		templateCode = append(templateCode, insideTemplate)

		content = content[loc[1]:]
	}

	if bytes.Equal(ORIGINAL_CONTENT, CLONED_CONTENT) == false {
		log.Printf("ORIGINAL_CONTENT = \n%q\n===================\ncontent = \n%q\n=============", ORIGINAL_CONTENT, CLONED_CONTENT)
		panic("content of the file has changed during lexical analysis (extracting template)." +
			"In a perfect world, it shouldn't change")
	}

	return templateCode, templatePositions
}

func ConvertSingleIndexToTextEditorPosition(buffer []byte, charIndex int) Position {
	var line, col int

	for i := range buffer {
		if i == charIndex {
			break
		}

		if buffer[i] == byte('\n') {
			line++
			col = 0

		} else {
			col++
		}
	}

	pos := Position{Line: line, Character: col}

	return pos
}

// 'rangeIndex' is incluse at rangeIndex[0] but exclusive at rangeIndex[1] (eg. [a, b[)
func convertRangeIndexToTextEditorPosition(editorContent []byte, rangeIndex []int, initialLine, initialColumn int) Range {
	if rangeIndex[0] > rangeIndex[1] {
		log.Printf("bad range formating.\n start = '%d' :: end = '%d'\n", rangeIndex[0], rangeIndex[1])
		panic("bad range formating, 'end position' cannot be before 'start position'")
	}

	if rangeIndex[0] == rangeIndex[1] {
		return Range{}
	}

	position := Range{}
	position.Start = ConvertSingleIndexToTextEditorPosition(editorContent, rangeIndex[0])
	position.End = ConvertSingleIndexToTextEditorPosition(editorContent, rangeIndex[1]-1)

	if position.Start.Line == 0 {
		position.Start.Character += initialColumn
	}

	if position.End.Line == 0 {
		position.End.Character += initialColumn
	}

	position.Start.Line += initialLine
	position.End.Line += initialLine

	return position
}

func tokenizeLine(data []byte, initialPosition Range) (*StreamToken, []Error) {
	tokenHandler := createTokenizer()
	data, isCommentAllowed, isTrimmed, err := handleExternalWhiteSpaceTrimmer(data, initialPosition)

	if err != nil {
		tokenHandler.Errs = append(tokenHandler.Errs, err)
	}

	if isTrimmed[0] {
		initialPosition.Start.Character++
	}

	patternTokens := tokenHandler.PatternToRecognize
	ignorePattern := tokenHandler.PatternToIgnore

	regIgnore := regexp.MustCompile(ignorePattern)

	var reg *regexp.Regexp
	var loc []int

	var isCurrentTokenSeparatedFromPrevious bool = true
	var isPreviousTokenAcceptBindingToken bool = true
	var found bool

	var lengthDataStart int = -1
	var currentLocalLineNumber, currentLocalColumnNumber int
	var indexFirstEqualOperator int = -1

	var parenthesisUnclosed []*Token = make([]*Token, 0, 3)

	currentLocalLineNumber = initialPosition.Start.Line
	currentLocalColumnNumber = initialPosition.Start.Character

	for len(data) > 0 && lengthDataStart != len(data) {
		lengthDataStart = len(data)
		found = false

		// Ignore White Space
		loc = regIgnore.FindIndex(data)
		if loc != nil && loc[0] == 0 {
			content := data[loc[0]:loc[1]]

			position := ConvertSingleIndexToTextEditorPosition(content, loc[1])
			if position.Line != 0 {
				currentLocalColumnNumber = 0
			}

			currentLocalLineNumber += position.Line
			currentLocalColumnNumber += position.Character

			isCurrentTokenSeparatedFromPrevious = true
			isPreviousTokenAcceptBindingToken = true
			data = data[loc[1]:]
		}

		// Match a pattern to a token
		for _, pattern := range patternTokens {
			reg = regexp.MustCompile(pattern.Value)
			loc = reg.FindIndex(data)

			isCurrentTokenSeparatedFromPrevious = isCurrentTokenSeparatedFromPrevious ||
				pattern.CanBeRightAfterToken || isPreviousTokenAcceptBindingToken

			if loc != nil && loc[0] == 0 {
				if !isCurrentTokenSeparatedFromPrevious {
					break
				}

				pos := convertRangeIndexToTextEditorPosition(data, loc, currentLocalLineNumber, currentLocalColumnNumber)
				pos.End.Character++
				currentLocalColumnNumber += loc[1]

				text := trimSuperflousCharacter(data[0:loc[1]], pattern.ID)
				tokenHandler.appendToken(pattern.ID, pos, text)
				token := tokenHandler.LastToken

				isPreviousTokenAcceptBindingToken = pattern.CanBeRightAfterToken
				isCurrentTokenSeparatedFromPrevious = false
				found = true
				data = data[loc[1]:]

				switch pattern.ID {
				case COMMENT:
					if isCommentAllowed == false {
						err := errors.New("no white space or characters between 'comment' and '{{' or '}}'")
						tokenHandler.appendError(err, token)
					}

				case LEFT_PAREN:
					parenthesisUnclosed = append(parenthesisUnclosed, token)

				case RIGTH_PAREN:
					size := len(parenthesisUnclosed)
					if len(parenthesisUnclosed) == 0 {
						// tokenHandler.appendError(errors.New("extra closing parenthesis ')'"), token)
						tokenHandler.appendError(errors.New("missing opening parenthesis '('"), token)
						break
					}

					parenthesisUnclosed = parenthesisUnclosed[:size-1]

				case ASSIGNEMENT, DECLARATION_ASSIGNEMENT:
					if indexFirstEqualOperator < 0 {
						indexFirstEqualOperator = len(tokenHandler.Tokens) - 1
					}
				}

				break
			}
		}

		// If no matching token found, add to error list
		if !found && len(data) > 0 {
			loc = regIgnore.FindIndex(data)

			if loc == nil {
				loc = []int{0, len(data)}
			} else {
				loc = []int{0, loc[0]}
			}

			pos := convertRangeIndexToTextEditorPosition(data, loc, currentLocalLineNumber, currentLocalColumnNumber)
			pos.End.Character++
			currentLocalColumnNumber += loc[1]

			var err error
			if isCurrentTokenSeparatedFromPrevious {
				err = errors.New("character(s) not recognized")

				if data[0] == byte('"') {
					err = errors.New("characters not recognized, did you meant a string ?")
				} else if data[0] == byte('/') {
					err = errors.New("comment syntax error")
				}
			} else {
				err = errors.New("character(s) not recognized, perhaps separate the word ?")
			}

			kindError := NOT_FOUND
			if bytes.Equal(data[:loc[1]], []byte("{{")) || bytes.Equal(data[:loc[1]], []byte("}}")) {
				err = errors.New("Missing matching template delimitator pair")
				kindError = UNEXPECTED
			}

			tokenHandler.appendToken(kindError, pos, data[:loc[1]])
			token := tokenHandler.LastToken
			tokenHandler.appendError(err, token)

			data = data[loc[1]:]
		}
	}

	if len(data) > 0 {
		tokenHandler.appendToken(UNEXPECTED, initialPosition, data)
		token := tokenHandler.LastToken
		tokenHandler.appendError(errors.New("unexpected character(s)"), token)

		data = nil
	}

	if len(tokenHandler.Tokens) == 0 {
		tokenHandler.appendError(errors.New("empty template"), &Token{ID: NOT_FOUND, Range: initialPosition})
	}

	for _, leftParenthesis := range parenthesisUnclosed {
		// tokenHandler.appendError(errors.New("unclosed parenthesis '('"), leftParenthesis)
		tokenHandler.appendError(errors.New("missing closing parenthesis ')'"), leftParenthesis)
	}

	stream := NewStreamToken(tokenHandler.Tokens, tokenHandler.FirstError, initialPosition, indexFirstEqualOperator)

	return stream, tokenHandler.Errs
}

// TODO: Redo comment token detection
func handleExternalWhiteSpaceTrimmer(data []byte, pos Range) ([]byte, bool, [2]bool, *LexerError) {
	isLeftCommentAllowed, isRigthCommentAllowed := false, false
	isRigthTrimmed, isLeftTrimmed := false, false

	var err *LexerError = nil

	if len(data) < 2 {
		isCommentAllowed := isLeftCommentAllowed && isRigthCommentAllowed
		isTrimmed := [2]bool{isLeftTrimmed, isRigthTrimmed}

		return data, isCommentAllowed, isTrimmed, err
	}

	//
	// Simple comment detection : {{/* ... */}}
	//
	// TODO: change compare to 'equal' and use 'byte' instead of slice
	if bytes.Equal(data[:1], []byte("/")) {
		isLeftCommentAllowed = true
	}

	lastElement := len(data) - 1
	if bytes.Equal(data[lastElement:], []byte("/")) {
		isRigthCommentAllowed = true
	}

	//
	// Advanced Comment detection : {{- /* ... */ -}}
	//
	if bytes.Equal(data[lastElement:], []byte("-")) {
		isRigthTrimmed = true
		data = data[:lastElement] // Trim rigth '-'

		lastElement = len(data) - 1
		isOkay := lastElement > 0

		if isOkay && bytes.ContainsAny(data[lastElement:], " \r\n\t\f\v") {
			isOkay = lastElement > 1
			if isOkay && bytes.Equal(data[lastElement-1:lastElement], []byte("/")) {
				isRigthCommentAllowed = true
			}
		} else {
			pos.Start.Character = pos.Start.Character - 1

			err = &LexerError{
				Err:   errors.New("'-' left operator cannot be next to non-white-space"),
				Range: pos,
				Token: &Token{Value: []byte(".-"), ID: SPACE_EATER, Range: pos},
			}
		}
	}

	if bytes.Equal(data[:1], []byte("-")) {
		isLeftTrimmed = true
		data = data[1:] // Trim left '-'

		isOkay := len(data) > 0

		if isOkay && bytes.ContainsAny(data[:1], " \r\n\t\f\v") {
			isOkay = len(data) > 1
			if isOkay && bytes.Equal(data[1:2], []byte("/")) {
				isLeftCommentAllowed = true
			}
		} else {
			pos.End.Character = pos.Start.Character + 2

			err = &LexerError{
				Err:   errors.New("'-' rigth operator cannot be next to non-white-space"),
				Range: pos,
				Token: &Token{Value: []byte("-."), ID: SPACE_EATER, Range: pos},
			}
		}
	}

	isCommentAllowed := isLeftCommentAllowed && isRigthCommentAllowed
	isTrimmed := [2]bool{isLeftTrimmed, isRigthTrimmed}

	return data, isCommentAllowed, isTrimmed, err
}

type patternToken struct {
	Value                string
	ID                   Kind
	CanBeRightAfterToken bool
}

type tokenizer struct {
	PatternToIgnore    string
	PatternToRecognize []patternToken
	Tokens             []Token
	Errs               []Error
	FirstError         *LexerError
	LastToken          *Token
}

func (t *tokenizer) appendToken(id Kind, pos Range, val []byte) {
	to := Token{
		ID:    id,
		Range: pos,
		Value: val,
	}

	t.Tokens = append(t.Tokens, to)
	t.LastToken = &to
}

func (t *tokenizer) appendError(err error, token *Token) {
	if err == nil {
		log.Printf("line tokenizer expected an error but got <nil> while appending error\n")
		panic("line tokenizer expected an error but got <nil> while appending error")
	}

	lexErr := &LexerError{
		Err:   err,
		Token: token,
		Range: token.Range,
	}

	t.Errs = append(t.Errs, lexErr)

	if t.FirstError == nil {
		t.FirstError = lexErr
	}
}

func createTokenizer() *tokenizer {
	// (tokenRecognizerPattern) Tokens' meaning: VariableName, ID (function ?), '==' '=' ':='
	tokenPatterns := []patternToken{
		{
			Value: "if|else|end|range|define|template|block|with|continue|break",
			ID:    KEYWORD,
		},
		{
			Value: `"(?:[^"\n\\]|\\.)*"`,
			ID:    STRING,
		},
		{
			Value: `\x60(?:[^\x60\n\\]|\\.)*\x60`, // \x60 == \`
			ID:    STRING,
		},
		// BUG: what if the user input multiple character within delimitator ? A bug will appear
		// Solve it later
		{
			Value: `'[^'\n\\]'`,
			ID:    CHARACTER,
		},
		{
			Value: `(?:\d+|\d*[.]\d+)i`,
			ID:    COMPLEX_NUMBER,
		},
		{
			Value: `\d*[.]\d+`,
			ID:    DECIMAL,
		},
		{
			Value: `\d+`,
			ID:    NUMBER,
		},
		{
			Value: "true|false",
			ID:    BOOLEAN,
		},
		{
			Value: `[$][.]?\w+(?:[.][a-zA-Z_]\w*)*|[$]`,
			ID:    DOLLAR_VARIABLE,
		},
		{
			Value: `(?:[.][a-zA-Z_]\w*)+|[.]`,
			ID:    DOT_VARIABLE,
		},
		{
			// Value: `\w+`,
			Value: `[[:alpha:]]\w*(?:[.][[:alpha:]]\w*)*`,
			ID:    FUNCTION,
		},
		{
			Value:                "==",
			ID:                   EQUAL_COMPARISON,
			CanBeRightAfterToken: true,
		},
		{
			Value:                "=",
			ID:                   ASSIGNEMENT,
			CanBeRightAfterToken: true,
		},
		{
			Value:                ":=",
			ID:                   DECLARATION_ASSIGNEMENT,
			CanBeRightAfterToken: true,
		},
		{
			Value:                "[|]",
			ID:                   PIPE,
			CanBeRightAfterToken: true,
		},
		{
			Value:                `\(`,
			ID:                   LEFT_PAREN,
			CanBeRightAfterToken: true,
		},
		{
			Value:                `\)`,
			ID:                   RIGTH_PAREN,
			CanBeRightAfterToken: true,
		},
		{
			Value: `\/\*(?:.|\s)*?(?:\*\/)`,
			ID:    COMMENT,
		},
		{
			Value: `,`,
			ID:    COMMA,
		},
	}

	to := &tokenizer{
		PatternToIgnore:    string(`\s+`),
		PatternToRecognize: tokenPatterns,
		Tokens:             nil,
		Errs:               nil,
		FirstError:         nil,
	}

	return to
}

func trimSuperflousCharacter(text []byte, id Kind) []byte {
	switch id {
	case COMMENT:
		lower := 2
		upper := len(text) - 2
		text = text[lower:upper]
	case STRING:
		lower := 1
		upper := len(text) - 1
		text = text[lower:upper]
	}

	return text
}
