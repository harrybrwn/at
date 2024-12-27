package lexgen

import (
	"cmp"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
	"unicode"

	"github.com/pkg/errors"

	"github.com/harrybrwn/at/array"
	"github.com/harrybrwn/at/internal/str"
	"github.com/harrybrwn/at/internal/xiter"
	"github.com/harrybrwn/at/lex"
	"github.com/harrybrwn/at/queue"
)

type Generator struct {
	BasePackage string
	Schemas     []*lex.Schema
	Defs        map[string]*lex.TypeSchema
	// maps schema id to generator
	generators map[string]*FileGenerator
}

func NewGenerator(base string) *Generator {
	return &Generator{
		BasePackage: base,
		Schemas:     make([]*lex.Schema, 0),
		Defs:        make(map[string]*lex.TypeSchema),
		generators:  make(map[string]*FileGenerator),
	}
}

func (g *Generator) AddSchemas(schemas []*lex.Schema) {
	for _, sch := range schemas {
		g.AddSchema(sch)
	}
}

func (g *Generator) AddSchema(sch *lex.Schema) {
	parts := strings.Split(sch.ID, ".")
	name := parts[len(parts)-1]
	for key, def := range sch.Defs {
		defname := name
		if key != "main" && def.Type == "object" {
			defname = defname + str.Title(key)
		}
		def.SchemaID = sch.ID
		def.Name = defname
		def.DefName = key

		k := sch.ID
		if key != "main" {
			k = sch.ID + "#" + key
		}
		if d, ok := g.Defs[k]; ok {
			slog.Warn("duplicated definition found", logFields(d)...)
		}
		g.Defs[k] = def
	}
	g.Schemas = append(g.Schemas, sch)
}

func (g *Generator) RPCs() ([]*StructType, error) {
	return g.List(func(st *StructType) bool {
		d := st.def()
		return d == nil || !d.Type.IsRPC()
	})
}

func (g *Generator) List(filters ...func(st *StructType) bool) ([]*StructType, error) {
	types := make([]*StructType, 0)
	for _, sch := range g.Schemas {
		fg, err := g.NewGenerator(sch)
		if err != nil {
			return nil, err
		}
	outer:
		for _, st := range fg.Types {
			for _, filter := range filters {
				if filter(st) {
					continue outer
				}
			}
			st.Name = st.StructName()
			types = append(types, st)
		}
	}
	return types, nil
}

func (g *Generator) ListTypes(filters ...func(st *StructType) bool) iter.Seq2[*StructType, error] {
	return func(yield func(*StructType, error) bool) {
		for _, sch := range g.Schemas {
			fg, err := g.NewGenerator(sch)
			if err != nil {
				if !yield(nil, err) {
					return
				}
			}
		outer:
			for _, st := range fg.Types {
				for _, filter := range filters {
					if filter(st) {
						continue outer
					}
				}
				st.Name = st.StructName()
				if !yield(st, nil) {
					return
				}
			}
		}
	}
}

type Import struct {
	Path string
	Name string
}

type Imports []*Import

func (ims Imports) Len() int           { return len(ims) }
func (ims Imports) Less(i, j int) bool { return ims[i].Path < ims[j].Path }
func (ims Imports) Swap(i, j int)      { ims[i], ims[j] = ims[j], ims[i] }

func (g *Generator) NewGenerator(sch *lex.Schema) (*FileGenerator, error) {
	if gen, ok := g.generators[sch.ID]; ok {
		return gen, nil
	}
	fg := FileGenerator{
		Schema:  sch,
		Imports: make([]*Import, 0),
		Types:   make([]*StructType, 0),
		Inputs:  make([]*StructType, 0),
		Outputs: make([]*StructType, 0),
		g:       g,
	}
	g.generators[sch.ID] = &fg
	pkg := fg.PackageName()

	var (
		imps = make(map[string]*Import)
	)
	for _, def := range IterMap(sch.Defs) {
		fg.Types = append(fg.Types, &StructType{
			Schema:  sch,
			Parent:  nil,
			Def:     def,
			Package: pkg,
		})
	}

	for def, err := range traverse(g, sch) {
		if err != nil {
			return nil, err
		}
		// Process imports
		if shouldImportStrconv(def) {
			addImport(imps, "strconv")
		}
		if isAtProtoFormat(def.Format) {
			addImport(imps, "github.com/bluesky-social/indigo/atproto/syntax")
		}
		if def.Parameters != nil {
			addImport(imps, "net/url")
		}
		if def.Input != nil && def.Input.Schema != nil {
			if def.Input.Encoding == "application/json" && def.Type == "procedure" {
				addImport(imps, "encoding/json")
			}
		}

		// Lift types up out from the tree because we need to generate go types.
		if def.Output != nil {
			fg.Outputs = append(fg.Outputs,
				&StructType{Schema: sch, Parent: def, Out: def.Output, Package: pkg})
		}
		if def.Input != nil {
			fg.Inputs = append(fg.Inputs,
				&StructType{Schema: sch, Parent: def, In: def.Input, Package: pkg})
		}
		if def.Parameters != nil {
			fg.Types = append(fg.Types,
				&StructType{Schema: sch, Parent: def, Param: def.Parameters, Package: pkg})
		}
		if def.Message != nil {
			fg.Types = append(fg.Types,
				&StructType{Schema: sch, Parent: def, Msg: def.Message, Package: pkg})
		}

		switch def.Type {
		case lex.TypeString:
			if def.Format == lex.FmtCID {
				addImport(imps,
					"github.com/ipfs/go-cid")
			}
		case lex.TypeBlob, lex.TypeBytes:
			// TODO blobs will most likely need a special import
			addImport(imps, "github.com/bluesky-social/indigo/lex/util")
		case lex.TypeCIDLink:
			addImport(imps,
				"github.com/bluesky-social/indigo/lex/util",
				"github.com/ipfs/go-cid")
		case lex.TypeQuery, lex.TypeProcedure, lex.TypeSubscription:
			// needed by genRPC
			addImport(
				imps,
				"context",
				"log/slog",
				"net/http",
				"github.com/harrybrwn/at/xrpc",
			)
			if def.Output != nil && (def.Output.Encoding == lex.EncodingANY || def.Output.Encoding == lex.EncodingMP4 || def.Output.Encoding == lex.EncodingCAR) ||
				def.Input != nil && (def.Input.Encoding == lex.EncodingANY || def.Input.Encoding == lex.EncodingMP4 || def.Input.Encoding == lex.EncodingCAR) {
				addImport(imps, "io")
			}
			if (def.Output != nil && def.Output.Encoding == lex.EncodingJSON) ||
				(def.Input != nil && def.Input.Encoding == lex.EncodingJSON) {
				addImport(imps, "encoding/json")
			}
			if def.Message != nil {
				addImport(imps, "github.com/coder/websocket", "iter")
			}

		case lex.TypeRef:
			sub, err := g.Ref(sch.ID, def.Ref)
			if err != nil {
				return nil, err
			}
			sub.NeedsCbor = def.NeedsCbor
			ref := newRef(g.BasePackage, sch.ID, def.Ref)
			if len(ref.Import.Path) > 0 {
				imps[ref.Import.Path] = &ref.Import
			}

		case lex.TypeUnion:
			// for MarshalJSON, UnmarshalJSON, MarshalCBOR, UnmarshalCBOR
			addImport(imps,
				"encoding/json",
				"github.com/fxamacker/cbor/v2",
				"github.com/pkg/errors",
			)
			def.NeedsCbor = true
			for _, ref := range def.Refs {
				r := newRef(g.BasePackage, sch.ID, ref)
				if r.HasImport() {
					imps[r.Import.Path] = &r.Import
				}
				inner, err := g.Ref(sch.ID, ref)
				if err == nil {
					inner.NeedsCbor = true
					// NOTE: this doesn't work when the inner type is imported
					//		 across packages.
					inner.InsideAUnion = true
				} else {
					slog.Warn("failed to find union ref", "ref", ref, "id", sch.ID, "error", err)
				}
			}
			if def.Parent != nil {
				fg.Types = append(fg.Types, &StructType{
					Schema:  sch,
					Parent:  def.Parent,
					Def:     def,
					Package: pkg,
				})
			}
		}
	}
	for _, v := range IterMap(imps) {
		fg.Imports = append(fg.Imports, v)
	}
	return &fg, nil
}

func traverse(g *Generator, sch *lex.Schema) iter.Seq2[*lex.TypeSchema, error] {
	var q queue.Queue[*lex.TypeSchema]
	q.Init()
	for key, def := range IterMap(sch.Defs) {
		def.DefName = key
		def.SchemaID = sch.ID
		q.Push(def)
	}

	return func(yield func(*lex.TypeSchema, error) bool) {
		for !q.Empty() {
			var err error
			def, _ := q.Pop()
			def.SchemaID = sch.ID
			if def.DefName == "main" {
				def.FullID = sch.ID
			} else {
				def.FullID = sch.ID + "#" + def.DefName
			}
			// Queue nested types
			if def.Input != nil {
				if def.Input.Schema != nil {
					def.Input.Schema.Parent = def
					def.Input.Schema.DefName = def.DefName
					q.Push(def.Input.Schema)
				} else if invalidEnc(def.Input.Encoding) {
					err = errors.Errorf("strange input type %q def in %q", def.Input.Encoding, sch.ID)
					goto end
				}
				def.NeedsType = true
			}
			if def.Output != nil {
				if def.Output.Schema != nil {
					def.Output.Schema.Parent = def
					def.Output.Schema.DefName = def.DefName
					q.Push(def.Output.Schema)
				} else if invalidEnc(def.Output.Encoding) {
					err = errors.Errorf("strange output type %q def in %q", def.Output.Encoding, sch.ID)
					goto end
				}
				def.NeedsType = true
			}
			if def.Parameters != nil {
				def.Parameters.Parent = def
				q.Push(def.Parameters)
			}
			if def.Message != nil {
				def.Message.Schema.Parent = def
				q.Push(def.Message.Schema)
			}
			if def.Record != nil {
				def.Record.Parent = def
				def.Record.NeedsType = true
				q.Push(def.Record)
			}
			if def.Items != nil {
				def.Items.Parent = def
				q.Push(def.Items)
			}
			for k, d := range IterMap(def.Properties) {
				d.DefName = k
				d.Parent = def
				q.Push(d)
			}
			if len(def.Ref) > 0 {
				_, err = g.Ref(sch.ID, def.Ref)
				if err != nil {
					goto end
				}
			}
		end:
			if !yield(def, err) {
				return
			}
		}
	}
}

func (g *Generator) GenClients(outDir string) error {
	types := xiter.Filter2(
		func(s *StructType, _ error) bool { return s.GetDef() != nil && s.GetDef().Type.IsRPC() },
		g.ListTypes(),
	)
	groupKey := func(st *StructType, _ error) string {
		return strings.Join(strings.Split(st.Schema.ID, ".")[:3], ".")
	}
	groups := xiter.GroupBy2(types, groupKey)
	for _, group := range groups {
		id := strings.Split(group.Key, ".")
		filename := filepath.Join(outDir, id[0], id[1], id[2]+"_client.go")
		f, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		typ := str.Title(id[2])
		imports := make(map[string]*Import)
		for _, pair := range group.Pairs {
			addClientImports(imports, pair.K.GetDef())
		}

		p := printer(f)
		p("// AUTO GENERATED. DO NOT EDIT!\n\n")
		p("package %[1]s\n\n", id[1])
		o := OrganizeImportMap(g.BasePackage, imports)
		err = o.Generate(f)
		if err != nil {
			return err
		}
		p(`
func New%[2]sClient(c *xrpc.Client) *%[2]sClient {
	return &%[2]sClient{c: c}
}

// %[2]sClient is a client for %[3]q.
type %[2]sClient struct {
	c *xrpc.Client
}`+"\n\n", id[1], str.Title(id[2]), strings.Join(id, "."))
		for _, pair := range group.Pairs {
			def := pair.K.GetDef()
			name := pair.K.StructName()
			//p("var _ %[1]s = (*%[2]sClient)(nil)\n\n", name, typ) // static interface check
			p("// %s implementds %q\n//\n", name, pair.K.Schema.ID)
			p("// source: %q\n", pair.K.Schema.Path())
			p(`func (c *%[1]sClient) `, typ)
			err := g.genHandlerFunctionDefinition(f, pair.K.GetDef(), name)
			if err != nil {
				return err
			}
			p(" {\n")
			p(`	var (
		err error
		q = make(url.Values)
	)` + "\n")
			if def.Parameters != nil {
				p(`	err = params.toQuery(q)
	if err != nil {
		return nil, err
	}` + "\n")
			}

			switch def.Type {
			case lex.TypeQuery:
				p(`	resbody, err := c.c.Query(ctx, %[1]q, q)
	if err != nil {
		return nil, err
	}
	defer resbody.Close()`+"\n", pair.K.Schema.ID)
				if def.Output == nil {
					p("\treturn nil, nil\n")
					p("}\n\n")
					continue
				}
				switch def.Output.Encoding {
				case lex.EncodingJSON:
					p("\tvar response %[1]sResponse\n", name)
					p(`	err = json.NewDecoder(resbody).Decode(&response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}` + "\n\n")
				case lex.EncodingANY, lex.EncodingCAR, lex.EncodingMP4:
					p("\treturn resbody, nil\n}\n\n")
				default:
					p("\treturn nil, nil\n")
					p("}\n\n")
				}

			case lex.TypeProcedure:
				if def.Input != nil {
					switch def.Input.Encoding {
					case lex.EncodingJSON:
						p(`	var reqBody bytes.Buffer
	err = json.NewEncoder(&reqBody).Encode(input)
	if err != nil {
		return nil, err
	}` + "\n")
					case lex.EncodingMP4, lex.EncodingANY, lex.EncodingCAR:
						p(`	var reqBody bytes.Buffer
	_, err = io.Copy(&reqBody, body)
	if err != nil {
		return nil, err
	}` + "\n")
					default:
						p(`	resbody, err := c.c.Procedure(ctx, %[1]q, q, nil)`+"\n", pair.K.Schema.ID)
					}
					p(`	resbody, err := c.c.Procedure(ctx, %[1]q, q, &reqBody)`+"\n", pair.K.Schema.ID)
				} else {
					p(`	resbody, err := c.c.Procedure(ctx, %[1]q, q, nil)`+"\n", pair.K.Schema.ID)
				}
				p(`	if err != nil {
		return nil, err
	}` + "\n")
				if def.Output == nil {
					p("\t_ = resbody\n")
					p("\treturn nil, nil\n")
					p("}\n\n")
					continue
				}
				switch def.Output.Encoding {
				case lex.EncodingJSON:
					p("\tvar response %[1]sResponse\n", name)
					p(`	err = json.NewDecoder(resbody).Decode(&response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}` + "\n\n")
				case lex.EncodingANY, lex.EncodingCAR, lex.EncodingMP4:
					p("\treturn resbody, nil\n}\n\n")
				default:
					p("\treturn nil, nil\n")
					p("}\n\n")
				}

			case lex.TypeSubscription:
				p(`	_ = q
	return nil, errors.New("subscriptions not implemented")
}` + "\n\n")
			}
		}
	}
	return nil
}

// TODO rename to SchemaGenerator
type FileGenerator struct {
	Schema  *lex.Schema
	Defs    map[string]*lex.TypeSchema
	Imports []*Import
	Types   []*StructType
	Inputs  []*StructType
	Outputs []*StructType
	g       *Generator
}

type StructType struct {
	Name     string
	Package  string
	Schema   *lex.Schema
	SchemaID string
	Parent   *lex.TypeSchema
	Def      *lex.TypeSchema
	Out      *lex.OutputType
	In       *lex.InputType
	Msg      *lex.MessageType
	Param    *lex.TypeSchema
}

func (st *StructType) GetDef() *lex.TypeSchema { return st.def() }

func (st *StructType) def() *lex.TypeSchema {
	switch {
	case st.Def != nil:
		return st.Def
	case st.In != nil:
		return st.In.Schema
	case st.Out != nil:
		return st.Out.Schema
	case st.Msg != nil:
		return st.Msg.Schema
	case st.Param != nil:
		return st.Param
	default:
		return nil
	}
}

func (st *StructType) StructName() string {
	var schID string
	def := st.def()
	if st.Schema != nil {
		schID = st.Schema.ID
	} else if len(st.SchemaID) > 0 {
		schID = st.SchemaID
	}
	pckage, thing := last2Dots(schID)
	name := pckage
	if len(thing) > 0 && thing != "defs" {
		name = name + str.Title(thing)
	}
	if def == nil {
		key := st.Parent.DefName
		if key == "main" || key == "" {
			return str.Title(name)
		}
		return str.Title(name) + str.Title(key)
	}
	key := def.DefName
	if def.Parent != nil && key == "" {
		key = def.Parent.DefName
	}
	if key == "main" {
		key = ""
	}
	if def.Type == "union" &&
		def.Parent != nil &&
		def.Parent.DefName != key &&
		def.Parent.DefName != "main" {
		name = name + str.Title(def.Parent.DefName)
	}
	name = name + str.Title(key)
	switch {
	case st.In != nil:
		name += "Request"
	case st.Out != nil:
		name += "Response"
	case def.Type == lex.TypeParams || st.Param != nil:
		name += "Params"
	case def.Type == "union":
		name += "Union"
	}
	return str.Title(name)
}

func isOutput(child *lex.TypeSchema) bool {
	if child.Parent == nil {
		return false
	}
	return child.Parent.Output != nil
}

func isInput(child *lex.TypeSchema) bool {
	if child.Parent == nil {
		return false
	}
	return child.Parent.Input != nil
}

func (fg *FileGenerator) GenImports(w io.Writer) error {
	if len(fg.Imports) == 0 {
		return nil
	}
	o := OrganizeImports(fg.g.BasePackage, fg.Imports)
	return o.Generate(w)
}

func (fg *FileGenerator) GenTypes(w io.Writer) error {
	p := printer(w)
	seen := make(map[string]struct{})
	types := make([]*StructType, 0)
	types = append(types, fg.Inputs...)
	types = append(types, fg.Outputs...)
	types = append(types, fg.Types...)
	// write the main type first in the file
	array.MoveToFront(types, func(st *StructType) bool {
		d := st.def()
		return d != nil && (d.DefName == "main" || d.Type.IsRPC())
	})
	for _, t := range types {
		var (
			def  = t.def()
			name = t.StructName()
		)
		t.Name = name
		if def == nil {
			if t.Out != nil {
				switch t.Out.Encoding {
				case lex.EncodingANY, lex.EncodingMP4, lex.EncodingCAR:
					p("// encoding %q\n", t.Out.Encoding)
					p("type %sResponse io.ReadCloser\n\n", name)
				default:
					p("// TODO how do I handle this???\n")
					p("// encoding %q\n", t.Out.Encoding)
					p("type %sResponse any\n\n", name)
				}
			}
			if t.In != nil {
				switch t.In.Encoding {
				case lex.EncodingANY, lex.EncodingMP4, lex.EncodingCAR:
					// skip because we just use http.Request.Body
				default:
					slog.Warn("cannot deturmine input type, defaulting to any", "name", name)
					p("// encoding %q\n", t.In.Encoding)
					p("type %sRequest any\n\n", name)
				}
			}
			continue
		}
		seenKey := string(def.Type) + "|" + name
		if _, ok := seen[seenKey]; ok {
			def := t.def()
			slog.Warn("already seen type",
				logFields(def, slog.String("name", name))...)
			continue
		}
		seen[seenKey] = struct{}{}

		switch def.Type {
		case lex.TypeString:
			p("// %s: %s\n", name, def.Description)
			p("//\n")
			p("// SchemaID: %q\n", def.SchemaID)
			p("// DefName:  %q\n", def.DefName)
			p("// FullID:   %q\n", def.FullID)
			p("// Type:     %q\n", def.Type)
			if len(def.KnownValues) > 0 {
				p("type %s interface {\n", name)
				p("\tTokenName() string\n\tTokenValue() string\n")
				p("\t// make sure this interface is private\n")
				p("\tprivate() %s\n", name)
				p("}\n\n")
				for _, v := range def.KnownValues {
					if ix := strings.IndexByte(v, '#'); ix < 0 {
						v = "#" + v
					}
					if _, ok := fg.g.Defs[v]; !ok {
						continue
					}
					v = strings.ReplaceAll(v, "-", "")
					v = strings.ReplaceAll(v, "!", "not")
					ref := newRef("", fg.Schema.ID, v)
					p("func (t *Token%s) private() %s { return t }\n", ref.TypeName, name)
				}
				p("\n")
				p("func %sKnownValueStrings() []string {\n", name)
				p("\treturn []string{\n%s}\n}\n\n", strings.Join(array.Map(def.KnownValues, func(s string) string {
					return fmt.Sprintf("\t\t%q", s)
				}), ",\n"))
			} else {
				p("type %s string\n\n", name)
			}

		case lex.TypeArray:
			p("// %s: %s\n", name, def.Description)
			p("//\n")
			p("// SchemaID: %q\n", def.SchemaID)
			p("// DefName:  %q\n", def.DefName)
			p("// FullID:   %q\n", def.FullID)
			p("// Type:     %q\n", def.Type)
			p("// Items:    %q\n", def.Items.Type)
			p("type %s %s\n\n", name, fg.g.typeName(def, def.DefName))

		case lex.TypeParams:
			p("// %s: %s\n", name, def.Description)
			p("//\n")
			p("// SchemaID: %q\n", def.SchemaID)
			p("// DefName:  %q\n", def.DefName)
			p("// FullID:   %q\n", def.FullID)
			p("// Type:     %q\n", def.Type)
			p("type %s struct {\n", t.Name)
			err := fg.genStruct(w, t, def.FullID, false)
			if err != nil {
				return err
			}
			p("}\n\n")
			err = fg.genFromQueryFn(w, def, name)
			if err != nil {
				return err
			}
			if err = fg.genToQueryFn(w, def, name); err != nil {
				return err
			}

		case lex.TypeObject:
			p("// %s: %s\n", name, def.Description)
			p("//\n")
			p("// SchemaID: %q\n", def.SchemaID)
			p("// DefName:  %q\n", def.DefName)
			p("// FullID:   %q\n", def.FullID)
			p("// Type:     %q\n", def.Type)
			p("type %s struct {\n", t.Name)
			err := fg.genStruct(w, t, def.FullID, true)
			if err != nil {
				return err
			}
			p("}\n\n")

		case lex.TypeRecord:
			p("// %s: %s\n", name, def.Description)
			p("//\n")
			p("// SchemaID: %q\n", def.SchemaID)
			p("// DefName:  %q\n", def.DefName)
			p("// FullID:   %q\n", def.FullID)
			p("// Type:     %q\n", def.Type)
			p("// key: %s\n", def.Key)
			p("type %s struct {\n", name)
			err := fg.genStruct(w, &StructType{
				Schema: fg.Schema,
				Def:    def.Record,
			}, def.FullID, true)
			if err != nil {
				return err
			}
			p("}\n\n")

		case lex.TypeRef:
			p("// %s: %s\n", name, def.Description)
			p("//\n")
			p("// SchemaID: %q\n", def.SchemaID)
			p("// DefName:  %q\n", def.DefName)
			p("// FullID:   %q\n", def.FullID)
			p("// Type:     %q\n", def.Type)
			p("// Ref:      %q\n", def.Ref)
			im := newRef(fg.g.BasePackage, fg.Schema.ID, def.Ref)
			p("type %s %s\n\n", name, im.GenName())

		case lex.TypeUnion:
			p("// %s: %s\n", name, def.Description)
			p("//\n")
			p("// SchemaID: %q\n", def.SchemaID)
			p("// DefName:  %q\n", def.DefName)
			p("// FullID:   %q\n", def.FullID)
			p("// Type:     %q\n", def.Type)
			p("// ref count: %d\n", len(def.Refs))
			p("type %s struct {\n", name)
			// p("\t// LexiconTypeID is the \"$type\" field and will always equal %q\n", fg.Schema.ID)
			// p("\t%s %s `json:\"$type\"`\n", "LexiconTypeID", "string")
			for _, ref := range def.Refs {
				refdef := &lex.TypeSchema{
					SchemaID: fg.Schema.ID,
					Type:     "ref",
					Ref:      ref,
				}
				typeName := fg.g.typeName(refdef, def.DefName)
				fieldName := typeName
				if ix := strings.IndexByte(typeName, '.'); ix >= 0 {
					fieldName = typeName[ix+1:]
				}
				p("\t// ref: %s\n", ref)
				p("\t%s *%s\n", fieldName, typeName)
			}
			p("}\n\n")
			// Generate MarshalJSON for the union type
			err := fg.genUnionMarshalJSON(w, def, name)
			if err != nil {
				return err
			}
			err = fg.genUnionUnmarshalJSON(w, def, name)
			if err != nil {
				return err
			}
			fg.genCborMarshalUnion(w, def, name)
			fg.genCborUnmarshalUnion(w, def, name)

		case lex.TypeQuery, lex.TypeProcedure, lex.TypeSubscription:
			_, err := fg.genHandlerDataSourceInterface(w, def, name)
			if err != nil {
				return err
			}
			err = fg.genHandlerType(w, def, name)
			if err != nil {
				return err
			}
			err = fg.genHTTPHandlerFunc(w, def, name)
			if err != nil {
				return err
			}
			err = fg.genAutoAddHandler(w, def, name)
			if err != nil {
				return err
			}

		case lex.TypeToken:
			p("// Token%s %s\n//\n", name, def.Description)
			p("// id: %s\n", def.FullID)
			p("type Token%s struct{}\n\n", name)
			// p("func (t Token%s) private()  string   { return %q }\n", name, def.DefName)
			p("func (t Token%s) TokenName()  string { return %q }\n", name, def.DefName)
			p("func (t Token%s) TokenValue() string { return %q }\n\n", name, def.Description)

		default:
			slog.Warn("unknown type",
				logFields(def, slog.String("name", name))...)
		}
	}
	return nil
}

func (fg *FileGenerator) GenCborGenerator(w io.Writer) (err error) {
	return GenCborCmd(w, array.Append(
		fg.Inputs,
		fg.Outputs,
		fg.Types,
	))
}

func GenCborCmd(w io.Writer, types []*StructType) (err error) {
	t := template.New("main").Funcs(templateFuncs)
	type Data struct {
		Types []*StructType
	}
	t, err = t.Parse(`// Auto generated. DO NOT EDIT

package main

func main() {
	{{- range $i, $tp := .Types }}
	{{- $def := $tp.GetDef }}
	{{- if or (not $def) (not $def.NeedsCbor) }}
		{{- continue }}
	{{- end }}
	// {{ $tp.Package }}.{{ $tp.StructName }}{},
	{{- end }}
}
`)
	if err != nil {
		return err
	}
	data := Data{Types: make([]*StructType, 0)}
	data.Types = types
	return t.Execute(w, &data)
}

func (g *FileGenerator) Ref(ref string) (*lex.TypeSchema, error) {
	return g.g.Ref(g.Schema.ID, ref)
}

func (g *Generator) Ref(currentSchemaID, ref string) (*lex.TypeSchema, error) {
	if len(ref) == 0 {
		return nil, errors.New("empty ref")
	}
	if ref[0] == '#' {
		ref = currentSchemaID + ref
	}
	def, ok := g.Defs[ref]
	if !ok {
		return nil, errors.Errorf("ref %q could not be resolved", ref)
	}
	return def, nil
}

func (fg *FileGenerator) PackageName() string {
	parts := strings.Split(fg.Schema.ID, ".")
	return parts[1]
}

func (fg *FileGenerator) PackageImport() Import {
	parts := strings.Split(fg.Schema.ID, ".")
	return Import{
		Name: parts[1],
		Path: filepath.Join(fg.g.BasePackage, parts[0], parts[1]),
	}
}

type SchemaTemplateData struct {
	Types []Type
}

type Type struct {
	Name      string
	Pkg       string
	Type      lex.Type
	Fields    []Field
	NeedsCbor bool
	st        *StructType
}

type Field struct {
	Name    string
	TypePkg string
	Type    string
	Pointer bool
	// Meta data fields
	OmitEmpty bool
	Required  bool
	Array     bool
	LexID     string
}

func (fg *FileGenerator) genStruct(w io.Writer, st *StructType, typ string, hasType bool) error {
	def := st.def()
	p := printer(w)
	kl, tl := fg.g.structAlign(def)
	if hasType {
		// TODO we should be skipping more types than just params
		kl = max(kl, len("LexiconTypeID"))
		tl = max(tl, len("string"))
		fmt.Fprintf(w, "\t// LexiconTypeID is the \"$type\" field and will always equal %q\n", typ)
		fmt.Fprintf(w, "\t%-*[2]s %-*[4]s `json:\"$type,omitempty\" cborgen:\"$type\" cbor:\"$type\"`\n",
			kl, "LexiconTypeID", tl, "string",
		)
	}
	for key, prop := range IterMap(def.Properties) {
		if prop.Parent != def {
			return errors.New("property has wrong parent")
		}
		omit := ",omitempty"
		ptr := ""
		if def.IsRequired(key) {
			omit = ""
		} else if st.Param != nil && prop.Type != lex.TypeArray {
			ptr = "*"
		}
		switch prop.Type {
		case "union":
			ptr = "*"
			typeName := fg.g.typeName(prop, def.DefName)
			tl = max(tl, len(typeName)) // adjust alignment
			p("\t%-*[2]s %-*[4]s `json:\"%[5]s%[6]s\" cborgen:\"%[5]s%[6]s\" cbor:\"%[5]s%[6]s\"`\n",
				kl, str.Title(key),
				tl, typeName,
				key, omit,
			)
			continue
		case lex.TypeBlob, lex.TypeBytes, lex.TypeCIDLink:
			// ptr = "*"
		case lex.TypeString:
			if prop.Format == lex.FmtAtIdentifier {
				ptr = "*"
			}
		}
		gotype := ptr + fg.g.typeName(prop, key)
		p("\t%-*[2]s %-*[4]s `json:\"%[5]s%[6]s\" cborgen:\"%[5]s%[6]s\" cbor:\"%[5]s%[6]s\"`\n",
			kl, str.Title(key),
			tl, gotype,
			key, omit,
		)
	}
	return nil
}

func (fg *FileGenerator) genHandlerDataSourceInterface(w io.Writer, def *lex.TypeSchema, name string) (string, error) {
	p := printer(w)
	interfaceFnName := str.Title(lastDot(def.SchemaID))
	p(`// %[1]s is an abstract type that provides data for the server
// handler %[1]sHandler.
//
// id: %[2]s`+"\n", name, def.SchemaID)
	p("type %s interface {\n", name)
	retType := "any"
	if def.Output != nil {
		switch def.Output.Encoding {
		case lex.EncodingANY, lex.EncodingMP4, lex.EncodingCAR:
			retType = "io.ReadCloser"
		default:
			retType = "*" + name + "Response"
		}
	} else if def.Message != nil && def.Type == lex.TypeSubscription {
		retType = fmt.Sprintf("iter.Seq[*%s]", fg.g.typeName(def.Message.Schema, ""))
	}
	switch def.Type {
	case lex.TypeQuery:
		p("\t%s(", interfaceFnName)
		if def.Parameters != nil {
			p("ctx context.Context, params *%sParams) (%s, error)\n}\n\n", name, retType)
		} else {
			p("ctx context.Context) (%s, error)\n}\n\n", retType)
		}
	case lex.TypeProcedure:
		if def.Input != nil {
			if def.Input.Schema != nil {
				p("\t%[1]s(ctx context.Context, input *%[2]sRequest) (%[3]s, error)\n}\n\n",
					interfaceFnName,
					name,
					retType,
				)
			} else {
				p("\t%[1]s(ctx context.Context, %[2]s) (%[3]s, error)\n}\n\n",
					interfaceFnName,
					"body io.Reader",
					retType,
				)
			}
		} else {
			p("\t%[1]s(ctx context.Context) (%[2]s, error)\n}\n\n", interfaceFnName, retType)
		}
	case lex.TypeSubscription:
		if def.Parameters != nil {
			p("\t%s(ctx context.Context, params *%sParams) (%s, error)\n}\n\n",
				interfaceFnName,
				name,
				retType)
		} else {
			p("\t%s(ctx context.Context) (%s, error)\n}\n\n", interfaceFnName, retType)
		}
	}
	return interfaceFnName, nil
}

func (g *Generator) genHandlerFunctionDefinition(w io.Writer, def *lex.TypeSchema, name string) error {
	retType := "any"
	if def.Output != nil {
		switch def.Output.Encoding {
		case lex.EncodingANY, lex.EncodingMP4, lex.EncodingCAR:
			retType = "io.ReadCloser"
		default:
			retType = "*" + name + "Response"
		}
	} else if def.Message != nil && def.Type == lex.TypeSubscription {
		retType = fmt.Sprintf("iter.Seq[*%s]", g.typeName(def.Message.Schema, ""))
	}
	interfaceFnName := str.Title(lastDot(def.SchemaID))
	p := printer(w)
	switch def.Type {
	case lex.TypeQuery:
		p("%s(", interfaceFnName)
		if def.Parameters != nil {
			p("ctx context.Context, params *%sParams) (%s, error)", name, retType)
		} else {
			p("ctx context.Context) (%s, error)", retType)
		}
	case lex.TypeProcedure:
		if def.Input != nil {
			if def.Input.Schema != nil {
				p("%[1]s(ctx context.Context, input *%[2]sRequest) (%[3]s, error)",
					interfaceFnName,
					name,
					retType,
				)
			} else {
				p("%[1]s(ctx context.Context, %[2]s) (%[3]s, error)",
					interfaceFnName,
					"body io.Reader",
					retType,
				)
			}
		} else {
			p("%[1]s(ctx context.Context) (%[2]s, error)", interfaceFnName, retType)
		}
	case lex.TypeSubscription:
		if def.Parameters != nil {
			p("%s(ctx context.Context, params *%sParams) (%s, error)",
				interfaceFnName,
				name,
				retType)
		} else {
			p("%s(ctx context.Context) (%s, error)", interfaceFnName, retType)
		}
	}
	return nil
}

func (fg *FileGenerator) genHandlerType(w io.Writer, def *lex.TypeSchema, name string) error {
	p := printer(w)
	p("// New%[1]sHandler creates a new %[1]sHandler.\n//\n", name)
	p("// id:   %s\n", def.FullID)
	p("// type: %s\n", def.Type)
	if def.Parent != nil {
		p("// parent id: %q\n", def.Parent.FullID)
	} else {
		p("// parent: nil\n")
	}
	p(`func New%[1]sHandler(db %[1]s, middleware ...func(http.Handler) http.Handler) *%[1]sHandler {
	return &%[1]sHandler{
		db:         db,
		logger:     slog.Default(),
		middleware: middleware,
	}
}

// type: %[1]q
type %[1]sHandler struct {
	logger     *slog.Logger
	db         %[1]s
	middleware []func(http.Handler) http.Handler
}
`+"\n", name)
	return nil
}

func (fg *FileGenerator) genRPC(w io.Writer, def *lex.TypeSchema, name string) {
	p := printer(w)
	interfaceFnName := str.Title(lastDot(def.SchemaID))
	p("func (h *%[1]sHandler) Apply(srv *xrpc.Server) {\n", name)
	p("\tsrv.AddRoute(\n\t\txrpc.NewMethod(%q, xrpc.%s),\n\t\txrpc.HandlerFunc(h.%s),\n\t)\n}\n\n", fg.Schema.ID, str.Title(def.Type), name)

	bodyVar := "_"
	if def.Input != nil {
		bodyVar = "body"
	}
	p("func (h *%[1]sHandler) %[1]s(ctx context.Context, rpcUrlQuery url.Values, %[2]s io.Reader) (any, error) {\n", name, bodyVar)

	if def.Parameters != nil && def.Parameters.Type == "params" {
		p("\tvar req %sParams\n\terr := req.fromQuery(rpcUrlQuery)\n", name)
		p("\tif err != nil {\n\t\treturn nil, err\n\t}\n")
		p("\tres, err := h.db.%s(ctx, &req)\n", interfaceFnName)
	} else if def.Input != nil {
		switch def.Input.Encoding {
		case lex.EncodingJSON:
			typeID := def.SchemaID
			if def.DefName != "main" {
				typeID += "#" + def.DefName
			}
			p("\tinput := %sRequest{LexiconTypeID: %q}\n", name, typeID)
			p("\terr := json.NewDecoder(body).Decode(&input)\n")
			p("\tif err != nil {\n\t\treturn nil, err\n\t}\n")
			p("\tres, err := h.db.%s(ctx, %s)\n",
				interfaceFnName,
				"&input",
			)
		case lex.EncodingMP4, lex.EncodingANY:
			fallthrough
		default:
			p("\t// pass raw %q encoded data\n", def.Input.Encoding)
			p("\tres, err := h.db.%s(ctx, body)\n", interfaceFnName)
		}
	} else {
		p("\tres, err := h.db.%s(ctx)\n", interfaceFnName)
	}

	p("\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	if def.Output != nil && def.Output.Schema != nil &&
		(def.Output.Schema.Type == "object" || def.Output.Schema.Type == "ref" || def.Output.Schema.Type == "record") {
		typeID := def.SchemaID
		if def.DefName != "main" {
			typeID += "#" + def.DefName
		}
		p("\tres.LexiconTypeID = %q\n", typeID)
	}
	p("\treturn res, nil\n")
	p("}\n\n")
}

func (fg *FileGenerator) genHTTPHandlerFunc(w io.Writer, def *lex.TypeSchema, name string) error {
	p := printer(w)
	interfaceFnName := str.Title(lastDot(def.SchemaID))
	typeID := def.SchemaID
	if def.DefName != "main" {
		typeID += "#" + def.DefName
	}
	p("var _ xrpc.Server\n")
	if len(def.Errors) > 0 {
		p("\n// errors\n")
		p("var (\n")
		for _, e := range def.Errors {
			p(`	// %[1]s %[2]s
	Err%[3]s%[1]s = &xrpc.ErrorResponse{
		Code: %[1]q,
		Message: "",
	}`+"\n", e.Name, e.Description, name)
		}
		p(")\n")
	}
	p("\n")
	p("// ServeHTTP fulfills the http.Handler interface and executes the xrpc %s.\n", def.Type)
	p("//\n")
	p("// id:     %s\n", typeID)
	p("// type:   %s\n", def.Type)
	if def.Parameters != nil {
		p("// params: ")
		keys := slices.Sorted(maps.Keys(def.Parameters.Properties))
		p("%s\n", strings.Join(keys, ", "))
	}
	if def.Input != nil {
		p("// input:  %s\n", def.Input.Encoding)
	} else {
		p("// input:  nil\n")
	}
	if def.Output != nil {
		p("// output: %s\n", def.Output.Encoding)
	} else {
		p("// output: nil\n")
	}
	if len(def.Errors) > 0 {
		p("// errors:\n")
		for _, e := range def.Errors {
			p("// - %s %s\n", e.Name, e.Description)
		}
	}

	p("func (h *%[1]sHandler) ServeHTTP(_w http.ResponseWriter, _r *http.Request) {\n", name)
	p(`	_ctx := _r.Context()
	_ctx = context.WithValue(_ctx, xrpc.MetaContextKey("xrpc-lex-schema-id"), %[1]q)
	_ctx = context.WithValue(_ctx, xrpc.MetaContextKey("xrpc-lex-type"), xrpc.%[2]s)
	h.logger.InfoContext(_ctx, "xrpc request", "type", %[3]q, "id", %[1]q)`+"\n", def.SchemaID, str.Title(def.Type), def.Type)
	if def.Parameters != nil {
		p("\tvar req %sParams\n", name)
		if len(def.Parameters.Properties) > 0 {
			p("\terr := req.fromQuery(_r.URL.Query())\n")
			p("\tif err != nil {\n\t\txrpc.WriteError(h.logger, _w, err, xrpc.InvalidRequest)\n\t\treturn\n\t}\n")
		}
		p("\tres, err := h.db.%s(_ctx, &req)\n", interfaceFnName)
		p("\tif err != nil {\n\t\txrpc.WriteError(h.logger, _w, err, xrpc.InvalidRequest)\n\t\treturn\n\t}\n")
	} else if def.Input != nil {
		switch def.Input.Encoding {
		case lex.EncodingJSON:
			p("\tinput := %sRequest{LexiconTypeID: %q}\n", name, typeID)
			// p("\tvar input %sRequest\n", name)
			p("\terr := json.NewDecoder(_r.Body).Decode(&input)\n")
			p("\tif err != nil {\n")
			p("\t\th.logger.Error(\"failed to decode json request body\", \"error\", err)\n")
			p("\t\t_w.WriteHeader(http.StatusBadRequest)\n")
			p("\t\treturn\n\t}\n")
			p("\tres, err := h.db.%s(_ctx, %s)\n",
				interfaceFnName,
				"&input",
			)
			p(`	if err != nil {
		xrpc.WriteError(h.logger, _w, err, xrpc.InvalidRequest)
		return
	}` + "\n")
		case lex.EncodingANY, lex.EncodingMP4, lex.EncodingCAR:
			p("\tres, err := h.db.%s(_ctx, _r.Body)\n", interfaceFnName)
			p("\tif err != nil {\n\t\txrpc.WriteError(h.logger, _w, err, xrpc.InvalidRequest)\n\t\treturn\n\t}\n")
		default:
			return errors.Errorf("cannot handle %q", def.Input.Encoding)
		}
	} else {
		p(`	res, err := h.db.%s(_ctx)
	if err != nil {
		xrpc.WriteError(h.logger, _w, err, xrpc.InvalidRequest)
		return
	}`+"\n", interfaceFnName)
	}

	if def.Message != nil {
		p("\t_wsconn, err := websocket.Accept(_w, _r, &websocket.AcceptOptions{})\n")
		p("\tif err != nil {\n\t\txrpc.WriteError(h.logger, _w, err, xrpc.InvalidRequest)\n\t\treturn\n\t}\n")
		p("\tdefer func() { _wsconn.Close(websocket.StatusInternalError, \"stopping websocket connection\") }()\n")
		p("\t// TODO check output content type, some websocket endpoints marshal to cbor some use json\n")
		p("\terr = xrpc.Stream(_ctx, _wsconn, res)\n")
		p("\tif err != nil {\n\t\txrpc.WriteError(h.logger, _w, err, xrpc.InvalidRequest)\n\t\treturn\n\t}\n")
		p("\t// TODO open a websocket\n")
		if def.Parameters != nil {
			p("\t_ = req\n")
		}
		p("\t_w.WriteHeader(http.StatusNotImplemented)\n")
	} else if def.Output != nil {
		switch def.Output.Encoding {
		case lex.EncodingJSON:
			p(`	err = json.NewEncoder(_w).Encode(res)
	if err != nil {
		h.logger.Error("failed to encode json response", "error", err)
		_w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_w.Header().Set("Content-Type", %q)`+"\n", def.Output.Encoding)
		case lex.EncodingANY, lex.EncodingMP4, lex.EncodingCAR:
			p(`	_, err = io.Copy(_w, res)
	if err != nil {
		h.logger.Error("failed to copy data to response", "error", err)
		_w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if err = res.Close(); err != nil {
		h.logger.Error("failed to close response data", "error", err)
		_w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_w.Header().Set("Content-Type", %q)`+"\n", def.Output.Encoding)
			// p("\t_w.Header().Set(\"Content-Type\", %q)\n", def.Output.Encoding)
		default:
			p("\t_ = res\n")
			p("\tpanic(\"cannot handle \\\"%s\\\"\")\n", def.Output.Encoding)
		}
	} else {
		p("\t// no output\n\t_ = res\n")
		p("\t_w.WriteHeader(http.StatusNoContent)\n")
		// p("\tpanic(`cannot handle`)\n")
	}
	p("}\n\n")
	p("var _ http.Handler = (*%sHandler)(nil)\n\n", name)
	return nil
}

func (fg *FileGenerator) genAutoAddHandler(w io.Writer, def *lex.TypeSchema, name string) error {
	typeID := def.SchemaID
	if def.DefName != "main" {
		typeID += "#" + def.DefName
	}
	fmt.Fprintf(w, `func (h *%[1]sHandler) Apply(srv *xrpc.Server, middleware ...func(http.Handler) http.Handler) {
	middleware = append(h.middleware, middleware...)
	srv.AddHandler(xrpc.NewMethod(%[2]q, xrpc.%[3]s), h, middleware...)
}

func (h *%[1]sHandler) Method() xrpc.Method {
	return xrpc.NewMethod(%[2]q, xrpc.%[3]s)
}`+"\n\n", name, typeID, str.Title(def.Type))
	return nil
}

func (fg *FileGenerator) dbStubParamsDecl(def *lex.TypeSchema, name string) string {
	switch def.Type {
	case "query":
		if def.Parameters != nil {
			return fmt.Sprintf("ctx context.Context, %s", "*"+name+"Request")
		}
		return "ctx context.Context"
	case "procedure":
		if def.Input != nil {
			if def.Input.Schema != nil {
				return fmt.Sprintf("ctx context.Context, %s", "*"+name+"Request")
			}
			switch def.Input.Encoding {
			case lex.EncodingANY, lex.EncodingMP4, lex.EncodingCAR:
				return "ctx context.Context, blob io.Reader"
			default:
				return "ctx context.Context, blob any"
			}
		}
		return "ctx context.Context"
	}
	return ""
}

func (g *Generator) GenStub(w io.Writer, st *StructType, typename string) error {
	p := printer(w)
	def := st.def()
	name := st.StructName()
	pkg := strings.Join(strings.Split(st.Schema.ID, ".")[:2], "")
	interfaceFnName := str.Title(lastDot(def.SchemaID))
	p("func (s *%s) ", typename)

	retType := "any"
	if def.Output != nil {
		switch def.Output.Encoding {
		case lex.EncodingANY, lex.EncodingMP4, lex.EncodingCAR:
			retType = "io.ReadCloser"
		default:
			retType = fmt.Sprintf("*%s.%sResponse", pkg, name)
		}
	} else if def.Message != nil && def.Type == lex.TypeSubscription {
		retType = fmt.Sprintf("iter.Seq[*%s.%s]", pkg, g.typeName(def.Message.Schema, ""))
	}

	switch def.Type {
	case lex.TypeQuery:
		p("%s(", interfaceFnName)
		if def.Parameters != nil {
			p("ctx context.Context, params *%s.%sParams) (%s, error)", pkg, name, retType)
		} else {
			p("ctx context.Context) (%s, error)", retType)
		}
	case lex.TypeProcedure:
		if def.Input != nil {
			if def.Input.Schema != nil {
				p("%s(ctx context.Context, input *%s.%sRequest) (%s, error)",
					interfaceFnName,
					pkg,
					name,
					retType,
				)
			} else {
				p("%s(ctx context.Context, %s) (%s, error)",
					interfaceFnName,
					"body io.Reader",
					retType,
				)
			}
		} else {
			p("%s(ctx context.Context) (%s, error)", interfaceFnName, retType)
		}
	case lex.TypeSubscription:
		if def.Parameters != nil {
			p("%s(ctx context.Context, params *%s.%sParams) (%s, error)",
				interfaceFnName,
				pkg,
				name,
				retType)
		} else {
			p("%s(ctx context.Context) (%s, error)", interfaceFnName, retType)
		}
	}
	p(" { return nil, nil }\n\n")
	return nil
}

func (fg *FileGenerator) genUnionMarshalJSON(w io.Writer, def *lex.TypeSchema, name string) error {
	tp := Type{
		Name:      name,
		Type:      def.Type,
		NeedsCbor: def.NeedsCbor,
	}
	for _, r := range def.Refs {
		if strings.HasPrefix(r, "#") {
			r = def.SchemaID + r
		}
		refdef := &lex.TypeSchema{
			SchemaID: fg.Schema.ID,
			Type:     "ref",
			Ref:      r,
		}
		typeName := fg.g.typeName(refdef, def.DefName)
		field := Field{
			Name:  typeName,
			Type:  typeName,
			LexID: r,
		}
		if ix := strings.IndexByte(typeName, '.'); ix >= 0 {
			field.TypePkg = typeName[:ix]
			field.Name = typeName[ix+1:]
		}
		tp.Fields = append(tp.Fields, field)
	}
	tmpl, err := template.New("marshalJSON").Funcs(templateFuncs).Parse(
		`func (t *{{ .Name }}) MarshalJSON() ([]byte, error) {
	{{- range .Fields }}
	if t.{{ .Name }} != nil {
		t.{{ .Name }}.LexiconTypeID = {{ .LexID | quote }}
		result, err := json.Marshal(t.{{ .Name }})
		return result, errors.WithStack(err)
	}
	{{- end }}
	return nil, errors.New("cannot marshal empty union")
}` + "\n\n")
	if err != nil {
		return err
	}
	return tmpl.Execute(w, &tp)
}

func (fg *FileGenerator) genUnionUnmarshalJSON(w io.Writer, def *lex.TypeSchema, name string) error {
	p := printer(w)
	p("func (t *%s) UnmarshalJSON(b []byte) (err error) {\n", name)
	p("\ttype typeSniffer struct {\n\t\tType string `json:\"$type\" cborgen:\"$type\" cbor:\"$type\"`\n\t}\n")
	p("\tvar sniff typeSniffer\n")
	p("\terr = json.Unmarshal(b, &sniff)\n")
	p("\tif err != nil {\n\t\treturn errors.WithStack(err)\n\t}\n")
	p("\tswitch sniff.Type {\n")
	for _, e := range def.Refs {
		if strings.HasPrefix(e, "#") {
			e = def.SchemaID + e
		}
		refdef := &lex.TypeSchema{
			SchemaID: fg.Schema.ID,
			Type:     "ref",
			Ref:      e,
		}
		typeName := fg.g.typeName(refdef, def.DefName)
		fname := typeName
		// trim package name
		if ix := strings.IndexByte(typeName, '.'); ix >= 0 {
			fname = typeName[ix+1:]
		}
		p(`	case %[1]q:
		t.%[2]s = new(%[3]s)
		return errors.WithStack(json.Unmarshal(b, t.%[2]s))`+"\n", e, fname, typeName)
		// p("\tcase %q:\n", e)
		// p("\t\tt.%s = new(%s)\n", fname, typeName)
		// p("\t\treturn json.Unmarshal(b, t.%s)\n", fname)
	}
	p("\tdefault:\n")
	if def.Closed {
		// TODO return error
		p("\t\treturn errors.New(\"cannot unmarshal empty union\")\n")
	} else {
		p("\t\treturn nil\n")
	}
	p("\t}\n")
	p("}\n\n")
	return nil
}

func (fg *FileGenerator) genCborMarshalUnion(w io.Writer, def *lex.TypeSchema, name string) {
	p := printer(w)
	p("func (t *%s) MarshalCBOR() ([]byte, error) {\n", name)
	p("\tswitch {\n")
	for _, e := range def.Refs {
		if strings.HasPrefix(e, "#") {
			e = def.SchemaID + e
		}
		refdef := &lex.TypeSchema{
			SchemaID: fg.Schema.ID,
			Type:     "ref",
			Ref:      e,
		}
		typeName := fg.g.typeName(refdef, def.DefName)
		fname := typeName
		// trim package name
		if ix := strings.IndexByte(typeName, '.'); ix >= 0 {
			fname = typeName[ix+1:]
		}
		p(`	case t.%[1]s != nil:
		t.%[1]s.LexiconTypeID = %[2]q
		result, err := cbor.Marshal(t.%[1]s)
		return result, errors.WithStack(err)`+"\n", fname, e)
	}
	if def.Closed {
		p("\tdefault:\n\t\treturn nil, errors.New(\"cannot marshal empty union\")\n\t}\n")
	} else {
		p("\tdefault:\n\t\treturn nil, nil\n\t}\n")
	}
	p("}\n\n")
}

func (fg *FileGenerator) genCborUnmarshalUnion(w io.Writer, def *lex.TypeSchema, name string) {
	p := printer(w)
	p(`func (t *%s) UnmarshalCBOR(b []byte) error {
	type typeSniffer struct {
		Type string `+"`cbor:\"$type\"`"+`
	}
	var sniffer typeSniffer
	err := cbor.Unmarshal(b, &sniffer)
	if err != nil {
		return errors.WithStack(err)
	}
	switch sniffer.Type {`+"\n", name)
	for _, e := range def.Refs {
		if strings.HasPrefix(e, "#") {
			e = def.SchemaID + e
		}
		refdef := &lex.TypeSchema{
			SchemaID: fg.Schema.ID,
			Type:     "ref",
			Ref:      e,
		}
		typeName := fg.g.typeName(refdef, def.DefName)
		fname := typeName
		// trim package name
		if ix := strings.IndexByte(typeName, '.'); ix >= 0 {
			fname = typeName[ix+1:]
		}
		p("\tcase %[1]q:\n\t\tt.%[2]s.LexiconTypeID = %[1]q\n", e, fname)
		p("\t\tt.%[1]s = new(%[2]s)\n", fname, typeName)
		p("\t\treturn errors.WithStack(cbor.Unmarshal(b, t.%[1]s))\n", fname)
	}
	p("\t}\n\treturn nil\n}\n\n")
}

// only when using cbor-gen
func (fg *FileGenerator) genCborGenMarhsalUnion(w io.Writer, def *lex.TypeSchema, name string) {
	p := printer(w)
	p("func (t *%s) MarshalCBOR(w io.Writer) error {\n", name)
	p("\tif t == nil {\n")
	p("\t\t_, err := w.Write(cborgen.CborNull)\n\t\treturn err\n")
	p("\t}\n")
	for _, e := range def.Refs {
		refdef := &lex.TypeSchema{
			SchemaID: fg.Schema.ID,
			Type:     "ref",
			Ref:      e,
		}
		vname := fg.g.typeName(refdef, def.DefName)
		if ix := strings.IndexByte(vname, '.'); ix >= 0 {
			vname = vname[ix+1:]
		}
		p("\tif t.%s != nil {\n", vname)
		p("\t\treturn t.%s.MarshalCBOR(w)\n\t}\n", vname)
	}
	p("\treturn fmt.Errorf(\"cannot cbor marshal empty enum\")\n}\n\n")
}

// only when using cbor-gen
func (fg *FileGenerator) writeCborGenUnmarshalerEnum(w io.Writer, def *lex.TypeSchema, name string) error {
	p := printer(w)
	p("func (t *%s) UnmarshalCBOR(r io.Reader) (err error) {\n", name)
	p("\ttyp, b, err := util.CborTypeExtractReader(r)\n")
	p("\tif err != nil {\n\t\treturn err\n\t}\n\n")
	p("\tswitch typ {\n")
	for _, e := range def.Refs {
		if strings.HasPrefix(e, "#") {
			e = def.SchemaID + e
		}

		// vname, goname := ts.namesFromRef(e)
		refdef := &lex.TypeSchema{
			SchemaID: fg.Schema.ID,
			Type:     "ref",
			Ref:      e,
		}
		goname := fg.g.typeName(refdef, def.DefName)
		vname := goname
		if ix := strings.IndexByte(goname, '.'); ix >= 0 {
			vname = goname[ix+1:]
		}

		p("\tcase \"%s\":\n", e)
		p("\t\tt.%s = new(%s)\n", vname, goname)
		p("\t\treturn t.%s.UnmarshalCBOR(bytes.NewReader(b))\n", vname)
	}
	p("\tdefault:\n")
	if def.Closed {
		p("\t\treturn fmt.Errorf(\"closed enums must have a matching value\")\n")
	} else {
		p("\t\treturn nil\n")
	}
	p("\t}\n")
	p("}\n\n")
	return nil
}

func (fg *FileGenerator) genFromQueryFn(w io.Writer, def *lex.TypeSchema, name string) error {
	p := printer(w)
	argname := "rpcUrlQuery"
	if len(def.Properties) == 0 {
		argname = "_"
	}
	p("func (_p *%[1]s) fromQuery(%[2]s url.Values) error {\n", name, argname)

	for key, prop := range IterMap(def.Properties) {
		switch prop.Type {
		case "string":
			fieldType := fg.g.typeName(prop, def.DefName)
			parser, ok := lex.FormatToParser[prop.Format]
			if len(prop.Format) > 0 && ok {
				if def.IsRequired(key) {
					// p("\traw%[1]s := rpcUrlQuery.Get(%[1]q)\n", key)
					// p("\tif len(raw%[1]s) == 0 {\n\t\treturn &xrpc.ErrorResponse{Code: xrpc.InvalidRequest, Message: \"\\\"%[1]s\\\" is required\"}\n\t}\n", key)
					// p("\t__%[1]s, err := %[2]s(raw%[1]s)\n", key, parser)
					// p("\tif err != nil {\n\t\treturn err\n\t}\n")
					p(`	raw%[1]s := rpcUrlQuery.Get(%[1]q)
	if len(raw%[1]s) == 0 {
		return &xrpc.ErrorResponse{Code: xrpc.InvalidRequest, Message: "\"%[1]s\" is required"}
	}
	__%[1]s, err := %[2]s(raw%[1]s)
	if err != nil {
		return err
	}
`, key, parser)
				} else {
					addr := "&"
					if prop.Format == lex.FmtAtIdentifier {
						addr = ""
					}
					p(`	var __%[1]sPtr *%[3]s = nil
	raw%[1]s := rpcUrlQuery.Get(%[1]q)
	if len(raw%[1]s) > 0 {
		__%[1]s, err := %[2]s(raw%[1]s)
		if err != nil {
			return &xrpc.ErrorResponse{Code: xrpc.InvalidRequest, Message: "faild to parse \"%[1]s\""}
		}
		__%[1]sPtr = %[4]s__%[1]s
	}`+"\n", key, parser, fieldType, addr)
				}
			} else {
				p("\t__%[1]s := rpcUrlQuery.Get(%[1]q)\n", key)
				if def.IsRequired(key) {
					p(`	if len(__%[1]s) == 0 {
		return &xrpc.ErrorResponse{
			Code:    xrpc.InvalidRequest,
			Message: "\"%[1]s\" is required",
		}
	}`+"\n", key)
				} else {
					p(`	var __%[1]sPtr *string = nil
	raw%[1]s := rpcUrlQuery.Get(%[1]q)
	if len(raw%[1]s) > 0 {
		__%[1]sPtr = &__%[1]s
	}`+"\n", key)
				}
			}
			if prop.MaxLength != nil {
				p(`	if len(__%[1]s) > %d {
		return &xrpc.ErrorResponse{Code: xrpc.InvalidRequest, Message: "%[1]s is too long"}
	}`+"\n", *prop.MaxLength)
			}

		case "integer":
			if def.IsRequired(key) {
				p(`	raw%[1]s := rpcUrlQuery.Get(%[1]q)
	if len(raw%[1]s) == 0 {
		return &xrpc.ErrorResponse{Code: xrpc.InvalidRequest, Message: "\"%[1]s\" is required"}	
	}
	__%[1]s, err := strconv.ParseInt(raw%[1]s, 10, 64)
	if err != nil {
		return err`+"\n", key)
			} else {
				p(`	var __%[1]sPtr *int64
	raw%[1]s := rpcUrlQuery.Get(%[1]q)
	if len(raw%[1]s) > 0 {
		__%[1]s, err := strconv.ParseInt(raw%[1]s, 10, 64)
		if err != nil {
			return &xrpc.ErrorResponse{
				Code: xrpc.InvalidRequest,
				Message: "faild to parse \"%[1]s\"",
			}
		}
		__%[1]sPtr = &__%[1]s`+"\n", key)
			}
			if v, ok := prop.Maximum.(float64); ok {
				p(`
		if __%[1]s > %[2]v {
			return &xrpc.ErrorResponse{
				Code: xrpc.InvalidRequest,
				Message: "%[1]s is too large",
			}
		}
`, key, int64(v))
			}
			if v, ok := prop.Minimum.(float64); ok {
				p(`
		if __%[1]s < %[2]v {
			return &xrpc.ErrorResponse{
				Code: xrpc.InvalidRequest,
				Message: "%[1]s is too small",
			}
		}
`, key, int64(v))
			}
			p("\t}\n")

		case "boolean":
			if def.IsRequired(key) {
				p(`
	raw%[1]s := rpcUrlQuery.Get(%[1]q)
	__%[1]s, err := strconv.ParseBool(raw%[1]s)
	if err != nil {
		return err
	}
`)
			} else {
				p(`
	var __%[1]sPtr *bool
	raw%[1]s := rpcUrlQuery.Get(%[1]q)
	if len(raw%[1]s) > 0 {
		__%[1]s, err := strconv.ParseBool(raw%[1]s)
		if err != nil {
			return &xrpc.ErrorResponse{Code: xrpc.InvalidRequest, Message: "faild to parse \"%[1]s\""}
		}
		__%[1]sPtr = &__%[1]s
	}`+"\n", key)
			}

		case "array":
			if def.IsRequired(key) {
				p(`	raw%[1]s, ok := rpcUrlQuery[%[1]q]
	if !ok || len(raw%[1]s) == 0 {
		return &xrpc.ErrorResponse{Code: xrpc.InvalidRequest, Message: "\"%[1]s\" is required"}
	}`+"\n", key)
			} else {
				p("\traw%[1]s := rpcUrlQuery[%[1]q]\n", key)
			}
			if prop.Items != nil {
				genArrayParse(w, key, prop.Items, false)
			}

		default:
			panic(fmt.Sprintf("can't handle %q as an rpc property", prop.Type))
		}
	}
	for key, prop := range IterMap(def.Properties) {
		if def.IsRequired(key) {
			p("\t_p.%[1]s = __%[2]s\n", str.Title(key), key)
		} else if prop.Type != lex.TypeArray {
			p("\t_p.%[1]s = __%[2]sPtr\n", str.Title(key), key)
		} else {
			p("\t_p.%[1]s = __%[2]s\n", str.Title(key), key)
		}
	}
	p("\treturn nil\n")
	p("}\n\n")
	return nil
}

func (fg *FileGenerator) genToQueryFn(w io.Writer, def *lex.TypeSchema, name string) error {
	p := printer(w)
	argname := "rpcUrlQuery"
	if len(def.Properties) == 0 {
		argname = "_"
	}
	p("func (_p *%[1]s) toQuery(%[2]s url.Values) error {\n", name, argname)
	for key, prop := range IterMap(def.Properties) {
		switch prop.Type {
		case lex.TypeString:
			if def.IsRequired(key) {
				switch prop.Format {
				case lex.FmtAtIdentifier, lex.FmtCID:
					p(`	rpcUrlQuery.Set(%[2]q, _p.%[1]s.String())`+"\n", str.Title(key), key)
				default:
					p(`	rpcUrlQuery.Set(%[2]q, string(_p.%[1]s))`+"\n", str.Title(key), key)
				}
			} else {
				switch prop.Format {
				case lex.FmtAtIdentifier, lex.FmtCID:
					p(`	if _p.%[1]s != nil {
		rpcUrlQuery.Set(%[2]q, _p.%[1]s.String())
	}`+"\n", str.Title(key), key)
				default:
					p(`	if _p.%[1]s != nil {
		rpcUrlQuery.Set(%[2]q, string(*_p.%[1]s))
	}`+"\n", str.Title(key), key)
				}
			}
		case lex.TypeInt:
			if def.IsRequired(key) {
				p(`rpcUrlQuery.Set(%[2]q, strconv.FormatInt(_p.%[1]s, 10))`+"\n", str.Title(key), key)
			} else {
				p(`	if _p.%[1]s != nil {
		rpcUrlQuery.Set(%[2]q, strconv.FormatInt(*_p.%[1]s, 10))
	}`+"\n", str.Title(key), key)
			}
		case lex.TypeBool:
			if def.IsRequired(key) {
				p(`rpcUrlQuery.Set(%[2]q, strconv.FormatBool(_p.%[1]s))`+"\n", str.Title(key), key)
			} else {
				p(`	if _p.%[1]s != nil {
		rpcUrlQuery.Set(%[2]q, strconv.FormatBool(*_p.%[1]s))
	}`+"\n", str.Title(key), key)
			}
		case lex.TypeArray:
			if prop.Items == nil {
				continue
			}
			switch prop.Items.Type {
			case lex.TypeString:
				switch prop.Items.Format {
				case lex.FmtAtIdentifier:
					p(`	for _, item := range _p.%[1]s {
		rpcUrlQuery.Add(%[2]q, item.String())
	}`+"\n", str.Title(key), key)
				case lex.FmtCID:
					p(`	for _, item := range _p.%[1]s {
		rpcUrlQuery.Add(%[2]q, item.String())
	}`+"\n", str.Title(key), key)
				default:
					p(`	for _, item := range _p.%[1]s {
		rpcUrlQuery.Add(%[2]q, string(item))
	}`+"\n", str.Title(key), key)
				}
			case lex.TypeInt:
				p(`	for _, item := range _p.%[1]s {
		rpcUrlQuery.Add(%[2]q, strconv.FormatInt(item, 10))
	}`+"\n", str.Title(key), key)
			case lex.TypeBool:
				p(`	for _, item := range _p.%[1]s {
		rpcUrlQuery.Add(%[2]q, strconv.FormatBool(item))
	}`+"\n", str.Title(key), key)
			default:
				slog.Info("unknown array items type while generating toQuery", logFields(prop)...)
			}
		default:
			slog.Info("unknown type while generating toQuery", logFields(prop)...)
		}
	}
	p("\treturn nil\n")
	p("}\n\n")
	return nil
}

// buildPropertiesMap will build a map containing a mapping of structs' field
// names (lowercase) to go types (camelcase). A calculation of the maximum key length and maximum type
// length is also returned.
func (fg *FileGenerator) buildPropertiesMap(def *lex.TypeSchema) (m map[string]string, kl, tl int) {
	m = make(map[string]string)
	for key, prop := range def.Properties {
		var gotype string
		switch prop.Type {
		case lex.TypeUnion:
			ns, schema := last2Dots(def.SchemaID)
			if schema != "defs" {
				ns = ns + str.Title(schema)
			}
			if def.DefName != "main" && def.DefName != key {
				gotype = str.Title(ns) + str.Title(def.DefName) + str.Title(key)
			} else {
				gotype = str.Title(ns) + str.Title(key)
			}
			gotype += "Union"
		case lex.TypeBlob, lex.TypeBytes, lex.TypeCIDLink:
			gotype = "*" + fg.g.typeName(prop, key)
		case lex.TypeString:
			gotype = fg.g.typeName(prop, key)
			if prop.Format == lex.FmtAtIdentifier {
				gotype = "*" + gotype
			}
		default:
			gotype = fg.g.typeName(prop, key)
		}
		kl = max(kl, len(key))
		tl = max(tl, len(gotype))
		m[key] = gotype
	}
	return m, kl, tl
}

func (g *Generator) structAlign(def *lex.TypeSchema) (kl, tl int) {
	for key, prop := range def.Properties {
		kl = max(kl, len(key))
		var l int
		switch prop.Type {
		case lex.TypeString:
			l = len(g.typeName(prop, key))
			if prop.Format == lex.FmtAtIdentifier {
				l++
			}
		case lex.TypeBlob, lex.TypeBytes, lex.TypeCIDLink:
			l = 1 + len(g.typeName(prop, key))
		case lex.TypeUnion:
			ns, schemaName := last2Dots(def.SchemaID)
			if schemaName != "defs" {
				ns = ns + str.Title(schemaName)
			}
			var typeName string
			if def.DefName != "main" && def.DefName != key {
				typeName = str.Title(ns) + str.Title(def.DefName) + str.Title(key)
			} else {
				typeName = str.Title(ns) + str.Title(key)
			}
			typeName += "Union"
			l = len(typeName)
		default:
			l = len(g.typeName(prop, key))
		}
		tl = max(tl, l)
	}
	return
}

func IterMap[K cmp.Ordered, V any](m map[K]V) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for _, k := range slices.Sorted(maps.Keys(m)) {
			if !yield(k, m[k]) {
				return
			}
		}
	}
}

func (g *Generator) typeName(sch *lex.TypeSchema, parentDefName string) string {
	if sch.Parent != nil {
		parentDefName = sch.Parent.DefName
	}
	if parentDefName == "main" {
		parentDefName = ""
	}
	switch sch.Type {
	case "string":
		if len(sch.Format) > 0 {
			tp, ok := lex.FormatToType[sch.Format]
			if ok {
				return tp
			}
		}
		return "string"

	case "integer":
		return "int64"
	case "boolean":
		return "bool"
	case "datetime":
		return "time.Time"
	case "array":
		if sch.Items == nil {
			slog.Warn("no items for array",
				logFields(sch, slog.String("parentDefName", parentDefName))...)
			return "[]any"
		}
		if sch.Items.Format == lex.FmtAtIdentifier {
			return "[]*" + g.typeName(sch.Items, parentDefName)
		}
		return "[]" + g.typeName(sch.Items, parentDefName)

	case "ref":
		ref := newRef(g.BasePackage, sch.SchemaID, sch.Ref)
		resolved, err := g.Ref(sch.SchemaID, sch.Ref)
		if err != nil {
			slog.Warn("couldn't resolve reference",
				logFields(sch, slog.String("ref", sch.Ref), slog.Any("error", err))...)
			// fallback option
			return ref.GenName()
		}

		pckage, thing := last2Dots(resolved.SchemaID)
		name := pckage
		if len(thing) > 0 && thing != "defs" {
			name = name + str.Title(thing)
		}
		key := resolved.DefName
		if resolved.Parent != nil && key == "" {
			key = resolved.Parent.DefName
		}
		if key == "main" {
			key = ""
		}
		if resolved.Type == "union" &&
			resolved.Parent != nil &&
			resolved.Parent.DefName != key &&
			resolved.Parent.DefName != "main" {
			name = name + str.Title(resolved.Parent.DefName)
		}
		name = name + str.Title(key)
		if isInput(resolved) {
			name += "Response"
		} else if isOutput(resolved) {
			name += "Response"
		} else if resolved.Type == "union" {
			name += "Union"
		}
		if len(ref.Import.Name) > 0 {
			if len(name) == 0 {
				name = lastDot(sch.Ref)
			}
			return ref.Import.Name + "." + str.Title(name)
		}
		return str.Title(name)

	case "union":
		ns, schemaName := last2Dots(sch.SchemaID)
		if schemaName != "defs" {
			ns = ns + str.Title(schemaName)
		}
		var typeName string
		if sch.DefName != "main" && sch.DefName != parentDefName {
			// typeName = str.Title(ns) + str.Title(sch.DefName) + str.Title(parentDefName)
			typeName = str.Title(ns) + str.Title(parentDefName) + str.Title(sch.DefName)
		} else if parentDefName != "main" {
			typeName = str.Title(ns) + str.Title(parentDefName)
		}
		return typeName + "Union"

	case "object":
		fallthrough
	case "unknown", "null":
		return "any"
	case lex.TypeBlob:
		return "util.LexBlob"
	case lex.TypeBytes:
		return "util.LexBytes"
	case lex.TypeCIDLink:
		return "cid.Cid"
		// return "util.LexLink"
	default:
		slog.Warn("unknown type name found", logFields(sch)...)
		return "any"
	}
}

type LexRef struct {
	SchemaID string
	Raw      string
	Import   Import
	TypeName string
}

func (lr *LexRef) GenName() string {
	if len(lr.Import.Name) > 0 {
		return lr.Import.Name + "." + str.Title(lr.TypeName)
	}
	return str.Title(lr.TypeName)
}

func (lr *LexRef) HasImport() bool {
	return len(lr.Import.Path) > 0
}

// this is awful
func newRef(basePackage, schemaID, ref string) *LexRef {
	r := LexRef{SchemaID: schemaID, Raw: ref}
	schemaIDParts := strings.Split(schemaID, ".")
	var schemaImportName string
	if len(schemaIDParts) > 2 {
		schemaImportName = strings.Join(schemaIDParts[:2], "")
	}
	ix := strings.IndexByte(ref, '#')
	if ix == 0 {
		p := schemaIDParts
		if strings.ToLower(p[len(p)-1]) == "defs" {
			r.TypeName = str.Title(p[len(p)-2]) + str.Title(ref[1:])
		} else if strings.ToLower(p[len(p)-2]) == "defs" {
			r.TypeName = str.Title(p[len(p)-3]) + str.Title(p[len(p)-1]) + str.Title(ref[1:])
		} else {
			r.TypeName = str.Title(p[len(p)-2]) + str.Title(p[len(p)-1]) + str.Title(ref[1:])
		}
	} else if ix > 0 {
		key := ref[ix+1:]
		nsid := ref[:ix]
		if len(nsid) > 0 {
			samePackage := trimLastDot(schemaID) == trimLastDot(nsid)
			parts := strings.Split(nsid, ".")
			if !samePackage {
				if len(parts) >= 2 {
					r.Import.Name = strings.Join(parts[:2], "")
					r.Import.Path = filepath.Join(array.Append([]string{basePackage}, parts[:2])...)
				}
			}
			if strings.ToLower(parts[len(parts)-1]) == "defs" {
				r.TypeName = str.Title(parts[len(parts)-2]) + str.Title(key)
			} else if strings.ToLower(parts[len(parts)-2]) == "defs" {
				r.TypeName = str.Title(parts[len(parts)-3]) + str.Title(parts[len(parts)-1]) + str.Title(key)
			} else {
				r.TypeName = str.Title(parts[len(parts)-2]) + str.Title(parts[len(parts)-1]) + str.Title(key)
			}
		}
	} else {
		parts := strings.Split(ref, ".")
		// key is "main"
		if ref != schemaID {
			r.Import.Name = strings.Join(parts[:2], "")
			r.Import.Path = filepath.Join(array.Append(
				[]string{basePackage},
				parts[:2],
			)...)
		}
		r.TypeName = str.Title(parts[len(parts)-2]) + str.Title(parts[len(parts)-1])
	}
	// Delete self imports
	if schemaImportName == r.Import.Name {
		r.Import.Name = ""
		r.Import.Path = ""
	}
	return &r
}

func genArrayParse(w io.Writer, name string, items *lex.TypeSchema, retNil bool) {
	p := printer(w)
	switch items.Type {
	case "string":
		var (
			syntaxTypeName string
			ptr            = ""
		)
		if len(items.Format) == 0 {
			p("\t__%[1]s := raw%[1]s\n", name)
			return
		}
		switch items.Format {
		case lex.FmtCID:
			p(`	__%[1]s := make([]%[3]s%[2]s, len(raw%[1]s))
	for i := range raw%[1]s {
		var e error
		__%[1]s[i], e = cid.Parse(raw%[1]s)
		if e != nil {
			return e
		}
	}`+"\n", name, "cid.Cid", ptr)
			return
		case "did", "nsid", "tid", "at-uri":
			syntaxTypeName = strings.ToUpper(strings.ReplaceAll(items.Format, "-", ""))
		case "handle", "language":
			syntaxTypeName = str.Title(items.Format)
		case "at-identifier":
			syntaxTypeName = strings.ReplaceAll(str.Title(items.Format), "-", "")
			ptr = "*"
		default:
			// TODO "datetime"
			p("\t__%[1]s := raw%[1]s\n", name)
			return
		}
		p("\t__%[1]s := make([]%[3]ssyntax.%[2]s, len(raw%[1]s))\n", name, syntaxTypeName, ptr)
		p("\tfor i := range raw%[1]s {\n", name)
		p("\t\tvar e error\n")
		p("\t\t__%[1]s[i], e = syntax.Parse%[2]s(raw%[1]s[i])\n", name, syntaxTypeName)
		if retNil {
			p("\t\tif e != nil {\n\t\t\treturn nil, e\n\t\t}\n")
		} else {
			p("\t\tif e != nil {\n\t\t\treturn e\n\t\t}\n")
		}
		p("\t}\n")
		return
	case "integer":
		p("\t__%[1]s := make([]int64, len(raw%[1]s))\n", name)
		p("\tfor i := range raw%[1]s {\n", name)
		p("\t\tvar e error\n")
		p("\t\t__%[1]s[i], e = strconv.ParseInt(raw%[1]s[i], 10, 64)\n", name)
		if retNil {
			p("\t\tif e != nil {\n\t\t\treturn nil, e\n\t\t}\n")
		} else {
			p("\t\tif e != nil {\n\t\t\treturn e\n\t\t}\n")
		}
	case "boolean":
		p("\t__%[1]s := make([]int64, len(raw%[1]s))\n", name)
		p("\tfor i := range raw%[1]s {\n", name)
		p("\t\tvar e error\n")
		p("\t\t__%[1]s[i], e = strconv.ParseBool(raw%[1]s[i])\n", name)
		// p("\t\tif e != nil {\n\t\t\treturn nil, e\n\t\t}\n")
		if retNil {
			p("\t\tif e != nil {\n\t\t\treturn nil, e\n\t\t}\n")
		} else {
			p("\t\tif e != nil {\n\t\t\treturn e\n\t\t}\n")
		}
	}
}

func printer(w io.Writer) func(format string, args ...any) {
	return func(format string, args ...any) {
		fmt.Fprintf(w, format, args...)
	}
}

func shouldImportStrconv(sch *lex.TypeSchema) bool {
	if sch.Parameters == nil {
		return false
	}
	if sch.Parameters != nil {
		for _, prop := range sch.Parameters.Properties {
			switch prop.Type {
			case "boolean", "integer":
				return true
			}
		}
	}
	return false
}

func isAtProtoFormat(s string) bool {
	switch s {
	case
		lex.FmtAtIdentifier,
		lex.FmtATURI,
		lex.FmtHandle,
		lex.FmtRecordKey,
		lex.FmtDID,
		lex.FmtNSID,
		// lex.FmtCID,
		lex.FmtTID,
		lex.FmtLang:
		return true
	}
	return false
}

func logFields(d *lex.TypeSchema, extras ...any) []any {
	fields := append([]any{
		slog.String("type", string(d.Type)),
		slog.String("id", d.FullID),
	}, extras...)
	if d.Output != nil {
		g := []any{
			slog.String("encoding", d.Output.Encoding),
		}
		if d.Output.Schema != nil {
			g = append(g, slog.String("type", string(d.Output.Schema.Type)))
			g = append(g, slog.String("id", string(d.Output.Schema.FullID)))
		}
		fields = append(fields, slog.Group("output", g...))
	}
	if d.Input != nil {
		g := []any{
			slog.String("encoding", d.Input.Encoding),
		}
		if d.Input.Schema != nil {
			g = append(g, slog.String("type", string(d.Input.Schema.Type)))
			g = append(g, slog.String("id", string(d.Input.Schema.FullID)))
		}
		fields = append(fields, slog.Group("input", g...))
	}
	if len(d.Ref) > 0 {
		fields = append(fields, slog.String("ref", d.Ref))
	}
	p := d.Parent
	if p != nil {
		fields = append(fields, slog.Group("parent",
			slog.String("type", string(p.Type)),
			slog.String("id", p.FullID),
		))
	} else {
		fields = append(fields, slog.Any("parent", nil))
	}
	return fields
}

var templateFuncs = map[string]any{
	"quote": func(s any) string { return fmt.Sprintf("%q", s) },
	"title": func(in any) string {
		s := in.(string)
		switch s {
		case "did":
			return "DID"
		case "rkey":
			return "RKey"
		case "uri":
			return "URI"
		case "cid":
			return "CID"
		case "nsid":
			return "NSID"
		case "url":
			return "URL"
		}
		prev := ' '
		return strings.Map(
			func(r rune) rune {
				if str.IsSeparator(prev) {
					prev = r
					return unicode.ToTitle(r)
				}
				prev = r
				return r
			}, s)
	},
}
