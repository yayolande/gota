package analyzer

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/yayolande/gota/lexer"
	"golang.org/x/tools/go/packages"
)

func TestGetTypeOfDollarVariableWithinFile(t *testing.T) {
	data := []struct {
		Varname					string
		ExpectedType			types.Type
		ExpectedTypeString	string
		IsExpectingError		bool
	} {
		{
			Varname: "atomicInt",
			ExpectedType: types.Typ[types.Int],
		},
		{
			Varname: "atomicString",
			ExpectedType: types.Typ[types.String],
		},
		{
			Varname: "atomicBool",
			ExpectedType: types.Typ[types.Bool],
		},
		{
			Varname: "varFunction",
			ExpectedType: nil,
			ExpectedTypeString: "func (int, bool) string",
		},
		{
			Varname: "varAnyFunction",
			ExpectedType: types.Universe.Lookup("any").Type(),
		},
		{
			Varname: "varStructSimple.Kind",
			ExpectedType: types.Typ[types.Int],
		},
		{
			Varname: "varStructSimple.Data",
			ExpectedType: types.Universe.Lookup("any").Type(),
		},
		{
			Varname: "varStructWithMethods.Age",
			ExpectedType: types.Typ[types.Int],
		},
		{
			Varname: "varStructWithMethods.Name",
			ExpectedType: nil,
			ExpectedTypeString: "func () string",	// signature type
		},
		{
			Varname: "varStructWithMethods.GetAge",
			ExpectedType: nil,
			ExpectedTypeString: "func () int",
		},
		{
			Varname: "varStructWithMethods.SetAge",
			ExpectedType: nil,
			ExpectedTypeString: "func (int)",	// signature type
		},
		{
			Varname: "varStructWithMethods.name",
			ExpectedType: types.Typ[types.String],
		},
		{
			Varname: "varStructWithMethods.GetAst.Data",
			ExpectedType: nil,
			// ExpectedTypeString: "struct { Kind int; Data any; }",
			IsExpectingError: true,		// bc method call can only be at the end of varname
		},
		/*
		{
			Varname: "varStructWithMethods.GetAst",
			ExpectedType: nil,
			ExpectedTypeString: "func () struct { Kind int; Data any; }",
		},
		*/
		{
			Varname: "varSliceSimple",
			ExpectedType: nil,
			ExpectedTypeString: "[]int",
		},
		{
			Varname: "varMapSimple",
			ExpectedType: nil,
			ExpectedTypeString: "map[string]*int",
		},
		{
			Varname: "varMapStruct.table",
			ExpectedType: nil,
			ExpectedTypeString: "map[string]*int",
		},
		{
			Varname: "varMapStruct.table.ast",
			ExpectedType: nil,
			IsExpectingError: true,
		},
		{
			Varname: "varPointerSimple",
			ExpectedTypeString: "*int",
		},
		{
			Varname: "varInterfaceError",
			ExpectedType: types.Universe.Lookup("error").Type(),
		},
		{
			Varname: "varInterfaceError.Error",
			ExpectedTypeString: "func () string",
		},
		{
			Varname: "varInterfaceEmbeded.Error",
			ExpectedTypeString: "func () string",
		},
		{
			Varname: "varInterfaceEmbeded.Err.Error",
			// ExpectedTypeString: "func () string",
			IsExpectingError: true,	// method call can only be at the end of varname
		},
		{
			Varname: "varChannelSimple",
			ExpectedTypeString: "chan int",
		},
		{
			Varname: "varAdvancedData.Counter",
			ExpectedTypeString: "*int",
		},
		{
			Varname: "varAdvancedData.Channel",
			ExpectedTypeString: "chan string",
		},
		{
			Varname: "varAdvancedData.EmbederFace.Err",
			ExpectedTypeString: "func () error",
		},
		{
			Varname: "varAdvancedData.person.Age",
			ExpectedTypeString: "int",
		},
		{
			Varname: "varAdvancedData.person.message",
			ExpectedTypeString: "chan string",
		},
		{
			Varname: "varAdvancedData.person.message.data",
			// ExpectedTypeString: "chan string",
			IsExpectingError: true,
		},
	}

	fileLocation := filepath.Join("..", "testdata", "source_code.go")

	config := &packages.Config{
		Mode: 
		// packages.NeedSyntax |
			packages.NeedDeps |
		// packages.NeedImports |
		// packages.LoadImports |
		// packages.LoadAllSyntax |
			packages.NeedTypes,
	}

	pkgs, err := packages.Load(config, fileLocation)
	if err != nil {
		t.Fatalf("go source code couldn't be loaded ..., %s", err.Error())
	}

	if len(pkgs) != 1 {
		t.Fatalf("test data must come from a single source file instead of many/any")
	}

	pkg := pkgs[0]

	if pkg.Types == nil {
		t.Fatalf("no types information found for the testdata source file")
	}

	for index, datium := range data {
		testName := "getTypeVariable_" + strconv.Itoa(index) + "_" + datium.Varname

		t.Run(testName, func(t *testing.T) {
			// 0. prelude
			variable := &lexer.Token{
				Value: []byte(datium.Varname),
				Range: lexer.Range{},
			}

			fields, _, _ := splitVariableNameFields(variable)
			rootName := fields[0]

			obj := pkg.Types.Scope().Lookup(rootName)
			if obj == nil {
				t.Fatalf("type/symbol is undefined in the source file\n" + 
					"=> varname = %s\n=> rootName = %s",
					datium.Varname, rootName)
			}

			varDef := &VariableDefinition{
				Name: rootName,
				Type: obj.Type(),
			}

			// 1. here come the function to test
			typ, err := getTypeOfDollarVariableWithinFile(variable, varDef)

			if err != nil && ! datium.IsExpectingError || 
				err == nil && datium.IsExpectingError {

				t.Fatalf("mismatch between error expectation and reality\n" + 
					"-> err = %s\n-> isErrorExpected = %t",
					err, datium.IsExpectingError,
				)
			} 

			if err != nil && datium.IsExpectingError {
				return
			}

			// start: skip type check for type difficult to create manually
			if datium.ExpectedType == nil {
				if datium.ExpectedTypeString == "" {
					t.Skipf("type check for 'expected' and 'computed' type is skipped !\n" +
						"Only skip type checking for type that are complex to create\n" + 
						"skipped type check for test = '%s'\n",
						testName,
					)
				}

				fset := token.NewFileSet()
				expr, err := parser.ParseExpr(datium.ExpectedTypeString)
				if err != nil {
					t.Fatalf("unable to parse expression '%s' ::: err = %s",
						datium.ExpectedTypeString, err.Error(),
					)
				}

				info := &types.Info{
					Types: make(map[ast.Expr]types.TypeAndValue),
				}

				err = types.CheckExpr(fset, nil, token.NoPos, expr, info)
				if err != nil {
					t.Fatalf("error while type checking expression '%s' ::: err = %s",
						expr, err.Error(),
					)
				}

				if info.Types == nil {
					t.Fatalf("no type defintion found")
				}

				datium.ExpectedType = info.Types[expr].Type
			}
			// end

			if ! types.Identical(typ, datium.ExpectedType) {
				t.Fatalf("type mismatch between expectation and reality\n" + 
					"-> got type = %#v\n-> expected type = %#v",
					typ, datium.ExpectedType,
				)
			}
			
		})
	}

}

// TODO: later on, add Range to the test suite
func TestSplitVariableNameFields(t *testing.T) {
	data := []struct {
		Name				string
		Offset			[2]int
		ExpectedOffset	[2]int
		ExpectedNames	[]string
		ExpectedError	bool
	} {
		{
			Name: "name",
			ExpectedNames: []string{"name"},
		},
		{
			Name: "wonder.land",
			ExpectedNames: []string{"wonder", "land"},
		},
		{
			Name: "car.wheel..fault",
			ExpectedNames: []string{"car", "wheel"},
			ExpectedError: true,
		},
		{
			Name: "car.wheel.fault.",
			ExpectedNames: []string{"car", "wheel", "fault"},
			ExpectedError: true,
		},
		{
			Name: ".person.address.city",
			ExpectedNames: []string{".", "person", "address", "city"},
		},
		{
			Name: "..department.class.student",
			ExpectedNames: []string{"."},
			ExpectedError: true,
		},
		{
			Name: ".name",
			ExpectedNames: []string{".", "name"},
		},
		{
			Name: "..name..",
			ExpectedNames: []string{"."},
			ExpectedError: true,
		},
		{
			Name: "$.name..",
			ExpectedNames: []string{"$", "name"},
			ExpectedError: true,
		},
		{
			Name: "$family.guy.",
			ExpectedNames: []string{"$family", "guy"},
			ExpectedError: true,
		},
		{
			Name: ".",
			ExpectedNames: []string{"."},
			ExpectedError: false,
		},
		{
			Name: "$.",
			ExpectedNames: []string{"$"},
			ExpectedError: true,
		},
	}

	for _, datium := range data {
		testName := "split_varname_" + datium.Name

		t.Run(testName, func(t *testing.T) {
			reach := lexer.Range{
				Start: lexer.Position{
					Line: 0,
					Character: datium.Offset[0],
				},
				End: lexer.Position{
					Line: 0,
					Character: datium.Offset[1],
				},
			}

			variable := &lexer.Token {
				Value: []byte(datium.Name),
				Range: reach,
			}

			fields, fieldsLocalPosition, err := splitVariableNameFields(variable)
			if datium.ExpectedError {
				if err == nil {
					t.Fatalf("varialbe name spliting was expecting an error but got nil." + 
						"\n varname = %s\n expect = %q\n got = %q", 
						datium.Name, datium.ExpectedNames, fields)
				}
			} else {
				if err != nil {
					t.Fatalf("varialbe name spliting didn't expect an error ::: %s." + 
						"\n varname = %s\n expect = %q\n got = %q\n err = %s", 
						err.Err.Error(), datium.Name, datium.ExpectedNames, fields, err.Err.Error())
				}
			}

			if err != nil {
				if ! datium.ExpectedError {
					t.Fatalf("varialbe name spliting error detected ::: %s." + 
						"\n varname = %s\n expect = %q\n got = %q", 
						err.Err.Error(), datium.Name, datium.ExpectedNames, fields)
				}
			}

			if len(datium.ExpectedNames) != len(fields) {
				t.Fatalf("length mismatch for coputed fields.\n expect = %q\n got = %q",
					datium.ExpectedNames, fields)
			}

			for index, field := range fields {
				if field != datium.ExpectedNames[index] {
					t.Errorf("field name mismatch.\n expect = %q\n got = %q",
						datium.ExpectedNames, fields)
				}
			}

			_ = fieldsLocalPosition
		})
	}
}

func TestMakeTypeInference(t *testing.T) {
	type Field struct {
		Varname		string
		TypeString	string
	}

	data := []struct {
		Expression		[]Field		// expr implies template expression ==> expr1 expr2 expr3 ...
		Expect			[2]string
		ExpectedError	error
	} {
		{
			Expression: []Field{	// expr ==> "num"
				{
					Varname: "num",
					TypeString: "int",
				},
			},
			Expect: [2]string { "int", "nil" },
		},
		{
			Expression: []Field{
				{
					Varname: "num",
					TypeString: "int",
				},
				{
					Varname: "num",
					TypeString: "int",
				},
			},
			// Expect: [2]string { "invalid", "nil" },
			ExpectedError: errArgumentsOnlyForFunction,
		},
		{
			Expression: []Field{
				{
					Varname: "num",
					TypeString: "func () string",
				},
			},
			Expect: [2]string { "string", "nil" },
			// ExpectedError: errArgumentsOnlyForFunction,
		},
		{
			Expression: []Field{
				{
					Varname: "num",
					TypeString: "func ()",
				},
			},
			// Expect: [2]string { "nil", "nil" },
			ExpectedError: errFunctionVoidReturn,
		},
		{
			Expression: []Field{
				{
					Varname: "coktail",
					TypeString: "func () (int, int, int)",
				},
			},
			// Expect: [2]string { "nil", "nil" },
			ExpectedError: errFunctionMaxReturn,
		},
		{
			Expression: []Field{
				{
					Varname: "coktail",
					TypeString: "func () (int, int)",
				},
			},
			// Expect: [2]string { "nil", "nil" },
			ExpectedError: errFunctionSecondReturnNotError,
		},
		{
			Expression: []Field{
				{
					Varname: "coktail",
					TypeString: "func () (int, error)",
				},
			},
			Expect: [2]string { "int", "error" },
			// ExpectedError: errFunctionSecondReturnNotError,
		},
		{
			Expression: []Field{
				{
					Varname: "coktail",
					TypeString: "func () (int, error, error)",
				},
			},
			// Expect: [2]string { "int", "error" },
			ExpectedError: errFunctionMaxReturn,
		},
		{
			Expression: []Field{	// expr => dat str num
				{
					Varname: "dat",
					TypeString: "func (string, int) string",
				},
				{
					Varname: "str",
					TypeString: "string",
				},
				{
					Varname: "num",
					TypeString: "int",
				},
			},
			Expect: [2]string { "string", "nil" },
			// ExpectedError: errArgumentsOnlyForFunction,
		},
		{
			Expression: []Field{
				{
					Varname: "dat",
					TypeString: "func (string, int)",
				},
				{
					Varname: "str",
					TypeString: "string",
				},
				{
					Varname: "num",
					TypeString: "int",
				},
			},
			// Expect: [2]string { "string", "nil" },
			ExpectedError: errFunctionVoidReturn,
		},
		{
			Expression: []Field{
				{
					Varname: "dat",
					TypeString: "func (string, ...int) *int",
				},
				{
					Varname: "str",
					TypeString: "string",
				},
				{
					Varname: "num",
					TypeString: "int",
				},
			},
			// Expect: [2]string { "*int", "nil" },
			ExpectedError: errTypeMismatch,
		},
		{
			Expression: []Field{
				{
					Varname: "dat",
					TypeString: "func (string, ...int) *int",
				},
				{
					Varname: "str",
					TypeString: "string",
				},
				{
					Varname: "num",
					TypeString: "[]int",
				},
			},
			Expect: [2]string { "*int", "nil" },
			// ExpectedError: errFunctionVoidReturn,
		},
		{
			Expression: []Field{
				{
					Varname: "dat",
					TypeString: "func (string, int, int, error) (int, error)",
				},
				{
					Varname: "makeStr",
					TypeString: "func () string",
				},
				{
					Varname: "genNumber",
					TypeString: "func () int",
				},
				{
					Varname: "counter",
					TypeString: "int",
				},
				{
					Varname: "errMsg",
					TypeString: "error",
				},
			},
			Expect: [2]string { "int", "error" },
			// ExpectedError: errFunctionVoidReturn,
		},
		{
			Expression: []Field{
				{
					Varname: "dat",
					TypeString: "func (string, int, int, error) (int, error)",
				},
				{
					Varname: "makeStr",
					TypeString: "func () string",
				},
				{
					Varname: "makeNumberFromSeed",
					TypeString: "func (int) int",
				},
				{
					Varname: "seed",
					TypeString: "int",
				},
				{
					Varname: "counter",
					TypeString: "int",
				},
				{
					Varname: "errMsg",
					TypeString: "error",
				},
			},
			// Expect: [2]string { "int", "error" },
			ExpectedError: errFunctionParameterSizeMismatch,
		},
		{
			Expression: []Field{
				{
					Varname: "dat",
					TypeString: "func (string, map[int]string) (int, error)",
				},
				{
					Varname: "makeStr",
					TypeString: "func () string",
				},
				{
					Varname: "seeder",
					TypeString: "map[int]string",
				},
			},
			Expect: [2]string { "int", "error" },
			// ExpectedError: errFunctionParameterSizeMismatch,
		},
	}

	computeExpressionType := func (exprString string, t *testing.T) (types.Type) {
		info := &types.Info{
			Types: make(map[ast.Expr]types.TypeAndValue),
			Defs: make(map[*ast.Ident]types.Object),
			Uses: make(map[*ast.Ident]types.Object),
		}

		expr, err := parser.ParseExpr(exprString)
		if err != nil {
			fmt.Printf("--> exprString = '%s'", exprString)
			t.Fatalf("unable to parse expression ::: %s", err.Error())
		}

		err = types.CheckExpr(token.NewFileSet(), nil, token.NoPos, expr, info)
		if err != nil {
			t.Fatalf("unable to type check expression ::: %s", err.Error())
		}

		typ := info.Types[expr].Type
		if typ == types.Typ[types.UntypedNil] {
			typ = nil
		}

		return typ
	}

	for index, datium := range data {
		testName := "testMakeTypeInference_" + strconv.Itoa(index)

		t.Run(testName, func(t *testing.T) {
			var symbols []*lexer.Token
			var typs []types.Type

			for _, field := range datium.Expression {
				/*
				if field.TypeString == "" {
					fmt.Println("empty 'typeString' for field = ", field)
				}
				*/

				typ := computeExpressionType(field.TypeString, t)

				symbol := &lexer.Token{
					Value: []byte(field.Varname),
				}

				symbols = append(symbols, symbol)
				typs = append(typs, typ)
			}

			inferedTypes, err := makeTypeInference(symbols, typs)
			if err != nil {
				if ! errors.Is(err.Err, datium.ExpectedError) {
					t.Fatalf("error while making expression type inference ::: %s", err.Err.Error())
				}

				return
			} else if datium.ExpectedError != nil {	// this mean that err == nil, but we expected otherwise
				t.Fatalf("expected an error but got noting\n" +
					" expected = %s\n got = %s", 
					datium.ExpectedError.Error(), err,
				)
			}

			expectFirst := computeExpressionType(datium.Expect[0], t)
			expectSecond := computeExpressionType(datium.Expect[1], t)

			if ! (types.Identical(inferedTypes[0], expectFirst) && 
				types.Identical(inferedTypes[1], expectSecond)) {
				t.Fatalf("type mismatch\n expected = %q\n got = %q", datium.Expect , inferedTypes)
			}
		})
	}
}


