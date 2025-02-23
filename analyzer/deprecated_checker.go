//go:build ignore

package analyzer

import (
	"log"
	"errors"
)

type TypeKind int

type Type interface {
	Kind() TypeKind
}

const (
	// TYPE_INT TypeKind
	TYPE_INT string = "int"
	TYPE_FLOAT = "float"
	TYPE_BOOL
	TYPE_CHAR
	TYPE_STRING = "string"
	TYPE_ANY = "any"
	TYPE_ERROR = "error"
	TYPE_VOID = "void"
	TYPE_MAP = "map"
	TYPE_ARRAY = "array"
	TYPE_STRUCT = "struct"
	TYPE_POINTER = "pointer"
	TYPE_ALIAS = "alias"
	TYPE_INVALID = "invalid type"
	// TYPE_IMPORTED = "coming from other package"
)

// int, float, string, bool, imported_type
type AtomicType struct {
	Type	TypeKind
}

func (a AtomicType) GetType() TypeKind {
	return a.Type
}

type MapType struct {
	Type	TypeKind
	Key	Type
	Val	Type
}

type StructType struct {
	Type	TypeKind
	Val	Type
}

func (s StructType) GetType() TypeKind {
	return s.Type
}

type PointerType struct {
	Type			TypeKind
	Reference	Type
}

type AliasType struct {
	Type		TypeKind
	Origin	TypeKind
}

func typeChecker(first, second any) (TypeKind, error) {

	first = resolveAliasType(first)
	second = resolveAliasType(second)

	if first.Kind() == TYPE_ANY || second.Kind() == TYPE_ANY {
		return TYPE_ANY, nil
	}

	if first.Kind() != second.Kind() {
		return TYPE_INVALID, errors.New("type mismatch")
	}

	switch first.Kind() {
	case TYPE_INT, TYPE_FLOAT, TYPE_BOOL, TYPE_CHAR, TYPE_STRING, TYPE_ANY, TYPE_ERROR:
		return first.Kind(), nil

	case TYPE_MAP:
		keyType, keyErr := typeChecker(first.Key, second.Key)
		valType, valErr := typeChecker(first.Key, second.Key)
		
		if keyErr != nil {
			return keyType, keyErr
		} else if valErr != nil {
			return valType, valErr
		}

		return first.Kind(), nil

	case TYPE_ARRAY:
		_, err := typeChecker(first.Val, second.Val)
		
		if err != nil {
			return TYPE_INVALID, err
		}

		return first.Kind(), nil

	case TYPE_STRUCT:
		var err error

		for index, field := range first.Fields {
			_, err = typeChecker(first.Fields[index], second.Fields[index])

			if err != nil {
				return TYPE_INVALID, err
			}
		}

		return first.Kind(), nil

	case TYPE_POINTER:
		_, err := typeChecker(first.Reference, second.Reference)

		if err != nil {
			return TYPE_INVALID, err
		}

		return first.Kind(), nil

	case TYPE_INVALID:
		// return TYPE_INVALID, errors.New("invalid type to compare")
		// ignore as this type should not be compared
		return TYPE_INVALID, nil

	case TYPE_VOID, TYPE_ALIAS:
		panic("helper type should not be used in the type checker, " + first.Kind().String())

	default:
		log.Printf("type checker found unknown type:\n first = %#v\n second = %#v\n", first, second)
		panic("unknown type")
	}

	panic("not implemented yet. first = " + first.Kind().String())
}
