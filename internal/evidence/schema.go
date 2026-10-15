package evidence

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed schema/report-v1.json
var reportSchema []byte

var compiledStructure = sync.OnceValues(func() (*jsonschema.Schema, error) {
	var document any
	if err := json.Unmarshal(reportSchema, &document); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(offlineLoader{})
	const uri = "https://ci-trust.invalid/schema/report-v1.json"
	if err := compiler.AddResource(uri, document); err != nil {
		return nil, err
	}
	return compiler.Compile(uri)
})

type offlineLoader struct{}

func (offlineLoader) Load(string) (any, error) {
	return nil, errors.New("external schema resources disabled")
}

// ValidateStructure validates JSON and the embedded v1 shape without network or
// filesystem resource loading. It does not establish semantic evidence validity.
// Diagnostics deliberately omit library error text, which can include input.
func ValidateStructure(data []byte) []Diagnostic {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return []Diagnostic{{Code: "json.invalid", Message: "Invalid JSON document."}}
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return []Diagnostic{{Code: "json.invalid", Message: "Expected exactly one JSON document."}}
	}
	if object, ok := value.(map[string]any); ok {
		if version, exists := object["schema_version"]; exists && version != "1" {
			return []Diagnostic{{Code: "schema.version", Path: "/schema_version", Message: "Unsupported schema version."}}
		}
	}
	schema, err := compiledStructure()
	if err != nil {
		return []Diagnostic{{Code: "schema.invalid", Message: "Embedded schema unavailable."}}
	}
	if err := schema.Validate(value); err != nil {
		var validation *jsonschema.ValidationError
		if !errors.As(err, &validation) {
			return []Diagnostic{{Code: "schema.invalid", Message: "Report does not match schema."}}
		}
		paths := map[string]bool{}
		var collect func(*jsonschema.ValidationError)
		collect = func(e *jsonschema.ValidationError) {
			if len(e.Causes) > 0 {
				for _, cause := range e.Causes {
					collect(cause)
				}
				return
			}
			path := ""
			for _, part := range e.InstanceLocation {
				path += "/" + strings.ReplaceAll(strings.ReplaceAll(part, "~", "~0"), "/", "~1")
			}
			paths[path] = true
		}
		collect(validation)
		ordered := make([]string, 0, len(paths))
		for path := range paths {
			ordered = append(ordered, path)
		}
		sort.Strings(ordered)
		diagnostics := make([]Diagnostic, 0, len(ordered))
		for _, path := range ordered {
			diagnostics = append(diagnostics, Diagnostic{Code: "schema.invalid", Path: path, Message: "Value does not match report schema."})
		}
		return diagnostics
	}
	return nil
}
