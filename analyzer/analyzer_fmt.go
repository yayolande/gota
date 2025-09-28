package analyzer

import (
	"fmt"
)

func (v VariableDefinition) String() string {
	str := fmt.Sprintf(`{
		"node": %s, 
		"parent": %s, 
		"range": %s, 
		"name": %s, 
		"filename": %s, 
		"typ": %s, 
		"TreeImplicitType": %s, 
		"IsUsedOnce": %t 
		}`,
		v.node, v.parent, v.rng, v.name, v.fileName, v.typ.String(), v.TreeImplicitType, v.IsUsedOnce,
	)

	return str
}

func (t nodeImplicitType) String() string {
	children := ""
	for _, child := range t.children {
		children = children + fmt.Sprintf("%s,", child)
	}

	if len(children) > 0 {
		children = children[:len(children)-1]
	}

	children = "[" + children + "]"

	str := fmt.Sprintf(`{"fieldName": %s, "fieldType": %s, "Range": %s, "toDiscard": %t, "isIterable": %t, "children": %s }`,
		t.fieldName, t.fieldType, t.rng, t.toDiscard, t.isIterable, children,
	)

	return str
}
