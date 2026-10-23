package gateway

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"

	"github.com/google/jsonschema-go/jsonschema"
)

//go:embed catalog.json
var catalogJSON []byte

func CatalogRevision() string {
	return digest(append(append([]byte{}, catalogJSON...), encode(eventReadOperation())...))
}

type Operation struct {
	QueryFields             []string               `json:"query_fields,omitempty"`
	ParameterDefaults       map[string]any         `json:"parameter_defaults,omitempty"`
	Kind                    string                 `json:"kind"`
	Method                  string                 `json:"method"`
	Path                    string                 `json:"path"`
	FileFields              []string               `json:"file_fields,omitempty"`
	EgressPrefixes          []string               `json:"egress_prefixes,omitempty"`
	FlagBindings            map[string]FlagBinding `json:"flag_bindings,omitempty"`
	ConditionalScopes       []string               `json:"conditional_app_scopes,omitempty"`
	ConditionalConfirmation bool                   `json:"conditional_confirmation,omitempty"`
	Hidden                  bool                   `json:"hidden,omitempty"`
	HasDryRun               bool                   `json:"has_dry_run,omitempty"`
	ID                      string                 `json:"id"`
	Tool                    string                 `json:"tool"`
	Description             string                 `json:"description"`
	Pack                    string                 `json:"pack"`
	Risk                    string                 `json:"risk"`
	Command                 []string               `json:"command"`
	InputSchema             map[string]any         `json:"input_schema"`
	DataSchema              map[string]any         `json:"data_schema"`
	AppScopes               []string               `json:"app_scopes"`
	ResourceBindings        map[string]string      `json:"resource_bindings"`
	DocURL                  string                 `json:"doc_url"`
	input                   *jsonschema.Resolved
	output                  *jsonschema.Resolved
}

type FlagBinding struct {
	JSON   bool     `json:"json,omitempty"`
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Input  []string `json:"input"`
	File   bool     `json:"file"`
	Output bool     `json:"output"`
}

type Catalog struct {
	SourceCommit string      `json:"source_commit"`
	Operations   []Operation `json:"operations"`
	byID         map[string]Operation
}

func compileSchema(raw map[string]any) (*jsonschema.Resolved, error) {
	var s jsonschema.Schema
	if err := json.Unmarshal(encode(raw), &s); err != nil {
		return nil, err
	}
	return s.Resolve(nil)
}

func LoadCatalog() (*Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(catalogJSON, &c); err != nil {
		return nil, err
	}
	c.Operations = append(c.Operations, eventReadOperation())
	c.byID = map[string]Operation{}
	names := map[string]bool{}
	valid := regexp.MustCompile(`^[a-z][a-z0-9_.]*$`)
	validCommand := regexp.MustCompile(`^[a-z+][a-z0-9_.+-]*$`)
	for i := range c.Operations {
		op := &c.Operations[i]
		if !validCommand.MatchString(op.ID) || !valid.MatchString(op.Tool) || len(op.Command) < 2 || names[op.Tool] || len(op.Tool) > 128 {
			return nil, fmt.Errorf("invalid operation: %s", op.ID)
		}
		names[op.Tool] = true
		for _, arg := range op.Command {
			if !validCommand.MatchString(arg) {
				return nil, fmt.Errorf("invalid command mapping: %s", op.ID)
			}
		}
		if op.Risk != "read" && op.Risk != "write" && op.Risk != "high-risk-write" {
			return nil, fmt.Errorf("unsupported risk: %s", op.ID)
		}
		var err error
		op.input, err = compileSchema(op.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("%s input: %w", op.ID, err)
		}
		op.output, err = compileSchema(op.DataSchema)
		if err != nil {
			return nil, fmt.Errorf("%s output: %w", op.ID, err)
		}
		c.byID[op.ID] = *op
	}
	sort.Slice(c.Operations, func(i, j int) bool { return c.Operations[i].Tool < c.Operations[j].Tool })
	return &c, nil
}

func (c *Catalog) Operation(id string) (Operation, bool) { op, ok := c.byID[id]; return op, ok }
func (op Operation) Validate(args map[string]any) error {
	if err := op.input.Validate(args); err != nil {
		return fault("invalid_arguments", err.Error())
	}
	return nil
}

func resultSchema(op *Operation) map[string]any {
	props := map[string]any{
		"status":        map[string]any{"type": "string"},
		"invocation_id": map[string]any{"type": "string"},
		"operation_id":  map[string]any{"type": "string"},
		"job_id":        map[string]any{"type": "string"},
		"files":         map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
		"error":         map[string]any{"type": "object"},
		"data":          map[string]any{},
		"meta":          map[string]any{"type": "object"},
	}
	if op != nil {
		props["data"] = op.DataSchema
	}
	return map[string]any{"type": "object", "properties": props, "required": []string{"status"}, "additionalProperties": false}
}
