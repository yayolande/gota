
// TODO: later on, move this file to 'testdata/analyzer' directory

package gota

import (
	"fmt"
)

var atomicInt int = 23
var atomicString string = "hello"
var atomicBool bool = true

var varFunction = func (int, bool) string { return "" }
var varAnyFunction any = func (int, bool) string { return "" }
var varStructSimple Ast
var varStructWithMethods Person
var varSliceSimple []int
var varMapSimple map[string]*int
var varMapStruct MapStruct
var varPointerSimple *int
var varInterfaceError error
var varInterfaceEmbeded EmbededInterface
var varChannelSimple chan int
var varAdvancedData *AdvancedData

// TODO: map, slice, array, struct, channel, function, method, var

type Ast struct {
	Kind	int
	Data	any
}

type DeathAst Ast
type CopeAst = Ast

func (c CopeAst) String() string {
	return fmt.Sprintln("type of CopeAst struct {}")
}

type Person struct {
	name	string
	Age	int
	message chan string
}

func (p Person) Name() string
func (p Person) GetAge() int
func (p *Person) SetAge(int)
func (p Person) GetAst() Ast

type textWriter interface {
	WriteText(string) int
	Err() error
}

type EmbededInterface interface {
	error
	textWriter

	ReadText() string
}

type MapStruct struct {
	table		map[string]*int
}

type AdvancedData struct {
	person		*Person
	Counter		*int
	Channel		chan string
	EmbederFace	EmbededInterface
}

func init() {
	varInterfaceError.Error()
}

