package lexer

import "fmt"

func (e LexerError) String() string {
	return fmt.Sprintf(`{ "Err": "%s", "Range": %s, "Token": %s }`, e.Err.Error(), e.Range, e.Token)
}

func (p Position) String() string {
	return fmt.Sprintf("{ \"Line\": %d, \"Character\": %d }", p.Line, p.Character)
}

func (r Range) String() string {
	return fmt.Sprintf("{ \"Start\": %s, \"End\": %s }", r.Start, r.End)
}

func (t Token) String() string {
	return fmt.Sprintf("{ \"ID\": \"%s\", \"Range\": %s, \"Value\": %q }", t.ID, t.Range, t.Value)
}

func (k Kind) String() string {
	str := "NOT FOUND ID"

	switch k {
	case DOT_VARIABLE:
		str = "DOT_VARIABLE"
	case DOLLAR_VARIABLE:
		str = "DOLLAR_VARIABLE"
	case KEYWORD:
		str = "KEYWORD"
	case FUNCTION:
		str = "FUNCTION"
	case IDENTIFIER:
		str = "IDENTIFIER"
	case ASSIGNEMENT:
		str = "ASSIGNEMENT"
	case DECLARATION_ASSIGNEMENT:
		str = "DECLARATION_ASSIGNEMENT"
	case STRING:
		str = "STRING"
	case NUMBER:
		str = "NUMBER"
	case EQUAL_COMPARISON:
		str = "EQUAL_COMPARISON"
	case PIPE:
		str = "PIPE"
	case LEFT_PAREN:
		str = "LEFT_PAREN"
	case RIGTH_PAREN:
		str = "RIGTH_PAREN"
	case COMMENT:
		str = "COMMENT"
	case EOL:
		str = "EOL"
	case NOT_FOUND:
		str = "NOT_FOUND"
	case UNEXPECTED:
		str = "UNEXPECTED"
	case COMMA:
		str = "COMMA"
	case STATIC_GROUP:
		str = "STATIC_GROUP"
	case EXPANDABLE_GROUP:
		str = "EXPANDABLE_GROUP"
	case CHARACTER:
		str = "CHARACTER"
	case COMPLEX_NUMBER:
		str = "COMPLEX_NUMBER"
	default:
		str = fmt.Sprintf("stringer() for 'lexer.Kind' type have found an unpected value: %d", k)
		panic(str)
	}

	return fmt.Sprintf(`%s`, str)
}

// TODO: change name of this function -- array_to_string() ?
func PrettyFormater[T fmt.Stringer](arr []T) string {
	if len(arr) == 0 {
		return "[]"
	}

	str := "["
	for _, el := range arr {
		str += fmt.Sprintf("%s,", el)
	}

	str = str[:len(str)-1]
	str += "]"

	return str
}

func Print(tokens ...Token) {
	str := PrettyFormater(tokens)
	fmt.Println(str)
}
