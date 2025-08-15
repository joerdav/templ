package parser

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	_ "embed"

	"github.com/a-h/parse"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/tools/txtar"
)

func TestScriptElementParserPlain(t *testing.T) {
	files, _ := filepath.Glob("scriptparsertestdata/*.txt")
	if len(files) == 0 {
		t.Errorf("no test files found")
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			a, err := txtar.ParseFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if len(a.Files) != 2 {
				t.Fatalf("expected 2 files, got %d", len(a.Files))
			}

			input := parse.NewInput(clean(a.Files[0].Data))
			result, ok, err := scriptElement.Parse(input)
			if err != nil {
				t.Fatalf("parser error: %v", err)
			}
			if !ok {
				t.Fatalf("failed to parse at %d", input.Index())
			}

			se, isScriptElement := result.(*ScriptElement)
			if !isScriptElement {
				t.Fatalf("expected ScriptElement, got %T", result)
			}

			var actual strings.Builder
			for _, content := range se.Contents {
				if content.GoCode != nil {
					t.Fatalf("expected plain text, got GoCode")
				}
				if content.Value == nil {
					t.Fatalf("expected plain text, got nil")
				}
				actual.WriteString(*content.Value)
			}

			expected := clean(a.Files[1].Data)
			if diff := cmp.Diff(actual.String(), string(expected)); diff != "" {
				t.Fatalf("%s:\n%s", file, diff)
			}
		})
	}
}

func TestScriptElementParser(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *ScriptElement
	}{
		{
			name:  "script: no content",
			input: `<script></script>`,
			expected: &ScriptElement{
				Range: Range{
					From: Position{Index: 0, Line: 0, Col: 0},
					To:   Position{Index: 17, Line: 0, Col: 17},
				},
			},
		},
		{
			name:  "script: go expression",
			input: `<script>{{ name }}</script>`,
			expected: &ScriptElement{
				Contents: []ScriptContents{
					NewScriptContentsGo(&GoCode{
						Expression: Expression{
							Value: "name",
							Range: Range{
								From: Position{Index: 11, Line: 0, Col: 11},
								To:   Position{Index: 15, Line: 0, Col: 15},
							},
						},
					}, false),
				},
				Range: Range{
					From: Position{Index: 0, Line: 0, Col: 0},
					To:   Position{Index: 27, Line: 0, Col: 27},
				},
			},
		},
		{
			name:  "script: regex with forward slashes",
			input: `<script>const clientIdMatch = evt.detail.message.match(/data-client-id="([^"]+)"/);</script>`,
			expected: &ScriptElement{
				Contents: []ScriptContents{
					NewScriptContentsScriptCode(`const clientIdMatch = evt.detail.message.match(/data-client-id="([^"]+)"/);`),
				},
				Range: Range{
					From: Position{Index: 0, Line: 0, Col: 0},
					To:   Position{Index: 92, Line: 0, Col: 92},
				},
			},
		},
		{
			name:  "script: division operator with numbers",
			input: `<script>var x = 1 / 2;</script>`,
			expected: &ScriptElement{
				Contents: []ScriptContents{
					NewScriptContentsScriptCode("var x = 1 / 2;"),
				},
				Range: Range{
					From: Position{Index: 0, Line: 0, Col: 0},
					To:   Position{Index: 31, Line: 0, Col: 31},
				},
			},
		},
		{
			name:  "script: division operator with identifiers",
			input: `<script>var z = x / y;</script>`,
			expected: &ScriptElement{
				Contents: []ScriptContents{
					NewScriptContentsScriptCode("var z = x / y;"),
				},
				Range: Range{
					From: Position{Index: 0, Line: 0, Col: 0},
					To:   Position{Index: 31, Line: 0, Col: 31},
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			input := parse.NewInput(tt.input)
			result, ok, err := scriptElement.Parse(input)
			if err != nil {
				t.Fatalf("parser error: %v", err)
			}
			if !ok {
				t.Fatalf("failed to parse at %d", input.Index())
			}
			if diff := cmp.Diff(tt.expected, result); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func FuzzScriptParser(f *testing.F) {
	files, _ := filepath.Glob("scriptparsertestdata/*.txt")
	if len(files) == 0 {
		f.Errorf("no test files found")
	}
	for _, file := range files {
		a, err := txtar.ParseFile(file)
		if err != nil {
			f.Fatal(err)
		}
		if len(a.Files) != 2 {
			f.Fatalf("expected 2 files, got %d", len(a.Files))
		}
		f.Add(clean(a.Files[0].Data))
	}

	f.Fuzz(func(t *testing.T, input string) {
		_, _, _ = scriptElement.Parse(parse.NewInput(input))
	})
}

func clean(b []byte) string {
	b = bytes.ReplaceAll(b, []byte("$\n"), []byte("\n"))
	b = bytes.TrimSuffix(b, []byte("\n"))
	return string(b)
}
