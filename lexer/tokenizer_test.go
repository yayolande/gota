package lexer

import (
	"fmt"
	"strconv"
	"testing"
)

func TestConvertToTextEditorPosition(t *testing.T) {
	type BufferPartition struct {
		Content                     []byte
		SelectionRange              [2]int
		ContentPositionWithinBuffer Position
	}

	// TODO: add a string expecged DS to compare will with the original
	// eg. input[SelectionRange[0]:SelectionRange[1] == expectedString
	data := []struct {
		input          BufferPartition
		expect         Range
		expectBuffered Range
	}{
		{
			input: BufferPartition{
				Content:                     []byte("aaaaaa\n\nH\n"),
				SelectionRange:              [2]int{1, 6},
				ContentPositionWithinBuffer: Position{Line: 3, Character: 40},
			},
			expect: Range{
				Start: Position{Line: 0, Character: 1},
				End:   Position{Line: 0, Character: 6},
			},
			expectBuffered: Range{
				Start: Position{Line: 3, Character: 41},
				End:   Position{Line: 3, Character: 46},
			},
		},
		{
			input: BufferPartition{
				Content:                     []byte("\n\nH\n"),
				SelectionRange:              [2]int{2, 2},
				ContentPositionWithinBuffer: Position{Line: 5, Character: 4},
			},
			expect: Range{
				Start: Position{Line: 2, Character: 0},
				End:   Position{Line: 2, Character: 0},
			},
			expectBuffered: Range{
				Start: Position{Line: 7, Character: 0},
				End:   Position{Line: 7, Character: 0},
			},
		},
		{
			input: BufferPartition{
				Content:                     []byte("\n\nHHHHH\n"),
				SelectionRange:              [2]int{3, 3},
				ContentPositionWithinBuffer: Position{Line: 5, Character: 4},
			},
			expect: Range{
				Start: Position{Line: 2, Character: 1},
				End:   Position{Line: 2, Character: 1},
			},
			expectBuffered: Range{
				Start: Position{Line: 7, Character: 1},
				End:   Position{Line: 7, Character: 1},
			},
		},
		{
			input: BufferPartition{
				Content:                     []byte("\n\nH\n"),
				SelectionRange:              [2]int{2, 3},
				ContentPositionWithinBuffer: Position{Line: 7, Character: 9},
			},
			expect: Range{
				Start: Position{Line: 2, Character: 0},
				End:   Position{Line: 2, Character: 1},
			},
			expectBuffered: Range{
				Start: Position{Line: 9, Character: 0},
				End:   Position{Line: 9, Character: 1},
			},
		},
		{
			input: BufferPartition{
				Content:                     []byte("\n\n\n"),
				SelectionRange:              [2]int{3, 4},
				ContentPositionWithinBuffer: Position{Line: 2, Character: 1},
			},
			expect: Range{
				Start: Position{Line: 3, Character: 0},
				End:   Position{Line: 3, Character: 0},
			},
			expectBuffered: Range{
				Start: Position{Line: 5, Character: 0},
				End:   Position{Line: 5, Character: 0},
			},
		},
		{
			input: BufferPartition{
				Content:                     []byte("H\n\nH\n"),
				SelectionRange:              [2]int{0, 4},
				ContentPositionWithinBuffer: Position{Line: 1, Character: 1},
			},
			expect: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: 2, Character: 1},
			},
			expectBuffered: Range{
				Start: Position{Line: 1, Character: 1},
				End:   Position{Line: 3, Character: 1},
			},
		},
		{
			input: BufferPartition{
				Content:                     []byte("aaaH\n\nH\n"),
				SelectionRange:              [2]int{3, 7},
				ContentPositionWithinBuffer: Position{Line: 3, Character: 1},
			},
			expect: Range{
				Start: Position{Line: 0, Character: 3},
				End:   Position{Line: 2, Character: 1},
			},
			expectBuffered: Range{
				Start: Position{Line: 3, Character: 4},
				End:   Position{Line: 5, Character: 1},
			},
		},
		{
			input: BufferPartition{
				Content:                     []byte("H\n\naaaH\n"),
				SelectionRange:              [2]int{0, 7},
				ContentPositionWithinBuffer: Position{Line: 0, Character: 4},
			},
			expect: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: 2, Character: 4},
			},
			expectBuffered: Range{
				Start: Position{Line: 0, Character: 4},
				End:   Position{Line: 2, Character: 4},
			},
		},
		{
			input: BufferPartition{
				Content:                     []byte("aaaH\n\ndddddH\n"),
				SelectionRange:              [2]int{3, 12},
				ContentPositionWithinBuffer: Position{Line: 21, Character: 97},
			},
			expect: Range{
				Start: Position{Line: 0, Character: 3},
				End:   Position{Line: 2, Character: 6},
			},
			expectBuffered: Range{
				Start: Position{Line: 21, Character: 100},
				End:   Position{Line: 23, Character: 6},
			},
		},
		{
			input: BufferPartition{
				Content:                     []byte("aaa H\n\n  dddddH\n"),
				SelectionRange:              [2]int{4, 15},
				ContentPositionWithinBuffer: Position{Line: 6, Character: 9},
			},
			expect: Range{
				Start: Position{Line: 0, Character: 4},
				End:   Position{Line: 2, Character: 8},
			},
			expectBuffered: Range{
				Start: Position{Line: 6, Character: 13},
				End:   Position{Line: 8, Character: 8},
			},
		},
		{
			input: BufferPartition{
				Content:                     []byte("\n\n\ndd"),
				SelectionRange:              [2]int{5, 6},
				ContentPositionWithinBuffer: Position{Line: 6, Character: 9},
			},
			expect: Range{
				Start: Position{Line: 3, Character: 2},
				End:   Position{Line: 3, Character: 2},
			},
			expectBuffered: Range{
				Start: Position{Line: 9, Character: 2},
				End:   Position{Line: 9, Character: 2},
			},
		},
		{
			input: BufferPartition{
				Content:                     []byte("\n\n\nddd"),
				SelectionRange:              [2]int{6, 7},
				ContentPositionWithinBuffer: Position{Line: 6, Character: 9},
			},
			expect: Range{
				Start: Position{Line: 3, Character: 3},
				End:   Position{Line: 3, Character: 3},
			},
			expectBuffered: Range{
				Start: Position{Line: 9, Character: 3},
				End:   Position{Line: 9, Character: 3},
			},
		},
		{
			input: BufferPartition{
				Content:                     []byte("\n\n\nddd"),
				SelectionRange:              [2]int{0, 2},
				ContentPositionWithinBuffer: Position{Line: 6, Character: 9},
			},
			expect: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: 2, Character: 0},
			},
			expectBuffered: Range{
				Start: Position{Line: 6, Character: 9},
				End:   Position{Line: 8, Character: 0},
			},
		},
	}

	for count, datium := range data {
		testName := fmt.Sprintf("Test_Single %d", count)

		input := datium.input
		expected := datium.expect

		t.Run(testName, func(t *testing.T) {
			start := ConvertSingleIndexToTextEditorPosition(input.Content, input.SelectionRange[0])
			end := ConvertSingleIndexToTextEditorPosition(input.Content, input.SelectionRange[1])

			got := Range{Start: start, End: end}

			if got != expected {
				t.Errorf("\n Expected : %#v\n But got: %#v\n", expected, got)
			}
		})

		testName = fmt.Sprintf("Test_Range_Editor_Position_%d", count)
		expected = datium.expectBuffered

		t.Run(testName, func(t *testing.T) {
			loc := input.SelectionRange[:]

			// This is done because 'convertRangeIndexToTextEditorPosition()'
			// expect a half-open range [a, b[
			// instead of the close range provided [a, b]
			loc[1] += 1

			col := input.ContentPositionWithinBuffer.Character
			line := input.ContentPositionWithinBuffer.Line

			got := convertRangeIndexToTextEditorPosition(input.Content, loc, line, col)

			if got != expected {
				t.Errorf("\n Expected : %#v\n But got: %#v\n", expected, got)
			}
		})
	}
}

func TestRangeContains(t *testing.T) {
	data := []struct {
		inputPosition Position
		inputRange    Range
		expect        bool
	}{
		{
			inputPosition: Position{Line: 2, Character: 3},
			inputRange: Range{
				Start: Position{Line: 2, Character: 0},
				End:   Position{Line: 2, Character: 4},
			},
			expect: true,
		},
		{
			inputPosition: Position{Line: 2, Character: 0},
			inputRange: Range{
				Start: Position{Line: 2, Character: 0},
				End:   Position{Line: 2, Character: 4},
			},
			expect: true,
		},
		{
			inputPosition: Position{Line: 2, Character: 4},
			inputRange: Range{
				Start: Position{Line: 2, Character: 0},
				End:   Position{Line: 2, Character: 4},
			},
			expect: true,
		},
		{
			inputPosition: Position{Line: 2, Character: 4},
			inputRange: Range{
				Start: Position{Line: 1, Character: 10},
				End:   Position{Line: 1, Character: 14},
			},
			expect: false,
		},
		{
			inputPosition: Position{Line: 2, Character: 4},
			inputRange: Range{
				Start: Position{Line: 2, Character: 5},
				End:   Position{Line: 2, Character: 7},
			},
			expect: false,
		},
		{
			inputPosition: Position{Line: 2, Character: 4},
			inputRange: Range{
				Start: Position{Line: 3, Character: 0},
				End:   Position{Line: 3, Character: 4},
			},
			expect: false,
		},
		{
			inputPosition: Position{Line: 2, Character: 4},
			inputRange: Range{
				Start: Position{Line: 1, Character: 0},
				End:   Position{Line: 3, Character: 4},
			},
			expect: true,
		},
		{
			inputPosition: Position{Line: 2, Character: 4},
			inputRange: Range{
				Start: Position{Line: 3, Character: 0},
				End:   Position{Line: 2, Character: 4},
			},
			expect: false,
		},
	}

	for count, datium := range data {
		name := "Range_Contains_Test_" + strconv.Itoa(count)
		t.Run(name, func(t *testing.T) {
			got := datium.inputRange.Contains(datium.inputPosition)

			if got != datium.expect {
				t.Errorf("\n Expected %v ::: Got = %v\n", datium.expect, got)
			}
		})
	}
}

func TestExtractTemplateCode(t *testing.T) {
	data := []struct {
		text   []byte
		expect Range
	}{
		{
			text: []byte("dd {{ $var := 23.3 }} tt"),
			expect: Range{
				Start: Position{Line: 0, Character: 5},
				End:   Position{Line: 0, Character: 19}, // exclusive
			},
		},
	}

	for index, datium := range data {
		name := fmt.Sprintf("Test_%d", index)

		t.Run(name, func(t *testing.T) {
			templateCode, positions := extractTemplateCode(datium.text)

			_ = templateCode
			got := positions[0]

			if got != datium.expect {
				t.Errorf("\n --> Expected %v \n --> Got = %v\n", datium.expect, got)
			}
		})
	}
}
