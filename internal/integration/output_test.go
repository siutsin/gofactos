// This file protects representative exact blueprint JSON output.
package integration

import (
	"encoding/json"
	"flag"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

var updateExpectedOutputs = flag.Bool(
	"update-expected-outputs",
	false,
	"rewrite all expected output files",
)

// TestBlueprintExpectedOutput checks blueprint-command JSON bytes for selected
// cases.
// Only update mode generates the set twice.
func TestBlueprintExpectedOutput(t *testing.T) {
	root := projectRoot(t)
	cases := expectedOutputCases()
	if *updateExpectedOutputs {
		jsonOutputs := make(map[string][]byte, len(cases))
		for _, c := range cases {
			jsonOutput := generateBlueprintJSON(t, root, c)
			require.NoError(t, validateGeneratedCase(c, jsonOutput))
			jsonOutputs[c.name] = jsonOutput

			//nolint:testifylint // Exact JSON bytes, not semantic equality, matter.
			require.Equal(
				t,
				jsonOutput,
				generateBlueprintJSON(t, root, c),
			)
		}
		require.NoError(
			t,
			writeExpectedOutputs(root, jsonOutputs),
		)
		return
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := generateBlueprintJSON(t, root, c)
			require.NoError(t, validateGeneratedCase(c, got))
			want, err := c.readExpectedOutput(root)
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
}

// TestValidateGeneratedCaseRejectsMalformedOutput prevents update mode from
// replacing all expected output with consistently invalid CLI output.
func TestValidateGeneratedCaseRejectsMalformedOutput(t *testing.T) {
	c := blueprintCase{name: "test", function: "add"}
	valid := blueprintJSON{}
	valid.Blueprint.Item = "blueprint"
	valid.Blueprint.Label = "add"
	valid.Blueprint.Version = 1
	valid.Blueprint.Entities = []blueprintEntity{
		{
			EntityNumber: 1,
			Name:         "arithmetic-combinator",
		},
		{
			EntityNumber: 2,
			Name:         "decider-combinator",
			Position:     blueprintPosition{X: 1},
		},
	}
	valid.Blueprint.Wires = []blueprintWire{{1, 1, 2, 1}}

	encode := func(doc blueprintJSON) []byte {
		data, err := json.Marshal(doc)
		require.NoError(t, err)
		return append(data, '\n')
	}
	require.NoError(t, validateGeneratedCase(c, encode(valid)))
	require.ErrorContains(
		t,
		validateGeneratedCase(c, []byte("not JSON\n")),
		"decode test JSON",
	)

	for _, tc := range []struct {
		name    string
		mutate  func(*blueprintJSON)
		wantErr string
	}{
		{
			name: "wrong item",
			mutate: func(doc *blueprintJSON) {
				doc.Blueprint.Item = "book"
			},
			wantErr: "blueprint item",
		},
		{
			name: "wrong label",
			mutate: func(doc *blueprintJSON) {
				doc.Blueprint.Label = "wrong"
			},
			wantErr: "blueprint label",
		},
		{
			name: "no version",
			mutate: func(doc *blueprintJSON) {
				doc.Blueprint.Version = 0
			},
			wantErr: "version is not positive",
		},
		{
			name: "no entities",
			mutate: func(doc *blueprintJSON) {
				doc.Blueprint.Entities = nil
			},
			wantErr: "no entities",
		},
		{
			name: "no wires",
			mutate: func(doc *blueprintJSON) {
				doc.Blueprint.Wires = nil
			},
			wantErr: "no wires",
		},
		{
			name: "invalid entity number",
			mutate: func(doc *blueprintJSON) {
				doc.Blueprint.Entities[0].EntityNumber = 0
			},
			wantErr: "entity number is not positive",
		},
		{
			name: "missing entity name",
			mutate: func(doc *blueprintJSON) {
				doc.Blueprint.Entities[0].Name = ""
			},
			wantErr: "entity 1 has no name",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := valid
			doc.Blueprint.Entities = append(
				[]blueprintEntity(nil),
				valid.Blueprint.Entities...,
			)
			doc.Blueprint.Wires = append(
				[]blueprintWire(nil),
				valid.Blueprint.Wires...,
			)
			tc.mutate(&doc)
			require.ErrorContains(
				t,
				validateGeneratedCase(c, encode(doc)),
				tc.wantErr,
			)
		})
	}
}

// validateGeneratedCase prevents malformed output from replacing trusted
// expected output files.
func validateGeneratedCase(
	c blueprintCase,
	output []byte,
) error {
	var doc blueprintJSON
	if err := json.Unmarshal(output, &doc); err != nil {
		return fmt.Errorf("decode %s JSON: %w", c.name, err)
	}
	switch {
	case doc.Blueprint.Item != "blueprint":
		return fmt.Errorf("%s: blueprint item is %q", c.name, doc.Blueprint.Item)
	case doc.Blueprint.Label != c.function:
		return fmt.Errorf(
			"%s: blueprint label is %q, want %q",
			c.name,
			doc.Blueprint.Label,
			c.function,
		)
	case doc.Blueprint.Version <= 0:
		return fmt.Errorf("%s: blueprint version is not positive", c.name)
	case len(doc.Blueprint.Entities) == 0:
		return fmt.Errorf("%s: blueprint has no entities", c.name)
	case len(doc.Blueprint.Wires) == 0:
		return fmt.Errorf("%s: blueprint has no wires", c.name)
	}
	seen := make(map[int]bool, len(doc.Blueprint.Entities))
	for _, ent := range doc.Blueprint.Entities {
		switch {
		case ent.EntityNumber <= 0:
			return fmt.Errorf("%s: entity number is not positive", c.name)
		case ent.Name == "":
			return fmt.Errorf(
				"%s: entity %d has no name",
				c.name,
				ent.EntityNumber,
			)
		case seen[ent.EntityNumber]:
			return fmt.Errorf(
				"%s: duplicate entity number %d",
				c.name,
				ent.EntityNumber,
			)
		}
		seen[ent.EntityNumber] = true
	}
	return nil
}
