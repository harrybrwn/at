package lexgen

import (
	"testing"

	"github.com/matryer/is"

	"github.com/harrybrwn/at/lex"
)

func TestNewRef(t *testing.T) {
	is := is.New(t)
	base := "github.com/harrybrwn/at/api"
	id := "app.bsky.graph.list"
	type table struct {
		id, ref string
		exp     LexRef
	}
	for _, ref := range []*LexRef{
		newRef(base, id, "app.bsky.graph.defs#listPurpose"),
		newRef(base, id, "app.bsky.richtext.facet"),
	} {
		is.True(ref != nil)
		is.True(!ref.HasImport())
		is.Equal(len(ref.Import.Name), 0)
		is.Equal(len(ref.Import.Path), 0)
	}
	for _, tt := range []table{
		{
			id,
			"app.bsky.graph.defs#listPurpose",
			LexRef{TypeName: "GraphListPurpose"},
		},
		{
			id,
			"#listPurpose",
			LexRef{TypeName: "GraphListListPurpose"},
		},
		{
			"com.atproto.test.myType",
			"#listItem",
			LexRef{TypeName: "TestMyTypeListItem"},
		},
		{
			id,
			"com.atproto.admin.defs#threatSignature",
			LexRef{
				Import:   Import{Name: "comatproto", Path: base + "/com/atproto"},
				TypeName: "AdminThreatSignature",
			},
		},
		{
			"com.atproto.admin.defs",
			"com.atproto.admin.doStuff#threatSignature",
			LexRef{TypeName: "AdminDoStuffThreatSignature"},
		},
		{
			"com.a.b.c",
			"io.a.b.c.things#x",
			LexRef{
				Import:   Import{Name: "ioa", Path: base + "/io/a"},
				TypeName: "CThingsX",
			},
		},
		{
			"io.a.b.c",
			"io.a.b.c.things#x",
			LexRef{TypeName: "CThingsX"},
		},
		{
			"com.atproto.admin.defs",
			"#threatSignature",
			LexRef{TypeName: "AdminThreatSignature"},
		},
		{
			id,
			"com.atproto.admin.deleteAccount",
			LexRef{
				Import:   Import{Name: "comatproto", Path: base + "/com/atproto"},
				TypeName: "AdminDeleteAccount",
			},
		},
		{
			"com.atproto.moderation.defs",
			"com.atproto.moderation.defs#reasonSpam",
			LexRef{TypeName: "ModerationReasonSpam"},
		},
		{
			"io.test.unions.defs",
			"#c",
			LexRef{TypeName: "UnionsC"},
		},
	} {
		tt.exp.Raw = tt.ref
		tt.exp.SchemaID = tt.id
		ref := newRef(base, tt.id, tt.ref)
		is.True(ref != nil)
		is.Equal(ref.Import, tt.exp.Import)
		is.Equal(ref.TypeName, tt.exp.TypeName)
		is.Equal(*ref, tt.exp)
	}
}

func TestNewRef_Err(t *testing.T) {
	t.Skip()
	is := is.New(t)
	base := "github.com/harrybrwn/at/api"
	type table struct {
		id, ref string
		exp     LexRef
	}
	for _, tt := range []table{
		{
			"io.test.unions.defs",
			"c",
			LexRef{TypeName: "UnionsC"},
		},
	} {
		tt.exp.Raw = tt.ref
		tt.exp.SchemaID = tt.id
		ref := newRef(base, tt.id, tt.ref)
		is.True(ref != nil)
	}
}

func TestNewGenerator(t *testing.T) {
	is := is.New(t)
	schema := &lex.Schema{
		Lexicon: 1,
		ID:      "com.atproto.repo.getRecord",
		Defs: map[string]*lex.TypeSchema{
			"main": {
				Type:     lex.TypeQuery,
				Required: []string{"repo", "collection", "rkey"},
				Properties: map[string]*lex.TypeSchema{
					"repo":       {Type: lex.TypeString, Format: lex.FmtAtIdentifier},
					"collection": {Type: lex.TypeString, Format: lex.FmtNSID},
					"rkey":       {Type: lex.TypeString},
					"cid":        {Type: lex.TypeString, Format: lex.FmtCID},
				},
				Output: &lex.OutputType{
					Encoding: lex.EncodingJSON,
					Schema: &lex.TypeSchema{
						Type:     lex.TypeObject,
						Required: []string{"uri", "value"},
						Properties: map[string]*lex.TypeSchema{
							"repo":  {Type: lex.TypeString, Format: lex.FmtATURI},
							"cid":   {Type: lex.TypeString, Format: lex.FmtCID},
							"value": {Type: lex.TypeUnknown},
						},
					},
				},
			},
		},
	}
	g := NewGenerator("github.com/harrybrwn/at/api")
	g.AddSchema(schema)
	fg, err := g.NewGenerator(schema)
	is.NoErr(err)
	is.Equal(fg.Schema, schema)
	is.Equal(fg.g, g)
	is.Equal(len(fg.Types), 1)
	is.Equal(len(fg.Outputs), 1)
	is.Equal(len(fg.Inputs), 0)
	is.Equal(fg.g.Defs["com.atproto.repo.getRecord"], schema.Defs["main"])
	is.Equal(fg.Types[0].Def, schema.Defs["main"])          // should be the same pointer
	is.Equal(fg.Outputs[0].Out, schema.Defs["main"].Output) // should be the same pointer
	is.Equal(fg.Types[0].Def.SchemaID, schema.ID)
	is.Equal(fg.Outputs[0].def().SchemaID, schema.ID)
	is.Equal(fg.Outputs[0].def().DefName, "main")
	is.Equal(fg.Outputs[0].def().FullID, schema.ID)
	is.Equal(fg.Outputs[0].Out.Encoding, lex.EncodingJSON)
}

func TestUnionNames(t *testing.T) {
	is := is.New(t)
	schema := &lex.Schema{
		ID: "io.test.unions.defs",
		Defs: map[string]*lex.TypeSchema{
			"things": {Type: lex.TypeUnion, Refs: []string{"#a", "#b", "#c"}},
			"a":      {Type: lex.TypeString},
			"b":      {Type: lex.TypeString},
			"c":      {Type: lex.TypeString},
			"d":      {Type: lex.TypeUnion, Refs: []string{"#a", "#b", "#c"}},
			"e": {
				Type: lex.TypeObject,
				Properties: map[string]*lex.TypeSchema{
					"inner": {Type: lex.TypeUnion, Refs: []string{"#a", "#b"}},
				},
			},
		},
	}
	g := NewGenerator("github.com/harrybrwn/at/api")
	g.AddSchema(schema)
	fg, err := g.NewGenerator(schema)
	is.NoErr(err)
	is.Equal(len(fg.Types), 7)
	is.Equal(g.Defs["io.test.unions.defs#things"], fg.Types[5].Def)
	is.Equal(g.Defs["io.test.unions.defs#e"], fg.Types[4].Def)
	is.Equal(g.Defs["io.test.unions.defs#d"], fg.Types[3].Def)
	is.Equal(g.Defs["io.test.unions.defs#c"], fg.Types[2].Def)
	is.Equal(g.Defs["io.test.unions.defs#b"], fg.Types[1].Def)
	is.Equal(g.Defs["io.test.unions.defs#a"], fg.Types[0].Def)
	inner := fg.Types[6]
	is.Equal(g.Defs["io.test.unions.defs#things"].Type, lex.TypeUnion)
	is.Equal(g.Defs["io.test.unions.defs#e"].Type, lex.TypeObject)
	is.Equal(g.Defs["io.test.unions.defs#d"].Type, lex.TypeUnion)
	is.Equal(inner.Def.Type, lex.TypeUnion)
	n := g.typeName(g.Defs["io.test.unions.defs#things"], "")
	is.True(n == "UnionsThingsUnion" && n == fg.Types[5].StructName())
	n = g.typeName(g.Defs["io.test.unions.defs#e"], "")
	is.Equal(n, "any")
	is.Equal("UnionsE", fg.Types[4].StructName())
	n = g.typeName(g.Defs["io.test.unions.defs#d"], "")
	is.Equal(n, "UnionsDUnion")
	is.Equal(n, fg.Types[3].StructName())
	n = g.typeName(inner.Def, "")
	is.Equal(n, "UnionsEInnerUnion")
	is.Equal("UnionsEInnerUnion", inner.StructName())
}

func TestDuplicateInnerUnion(t *testing.T) {
	is := is.New(t)
	schema := &lex.Schema{
		ID: "app.bsky.feed.defs",
		Defs: map[string]*lex.TypeSchema{
			"postView": {
				Type: lex.TypeObject,
				Properties: map[string]*lex.TypeSchema{
					"uri":         {Type: lex.TypeString, Format: lex.FmtATURI},
					"cid":         {Type: lex.TypeString, Format: lex.FmtCID},
					"replyCount":  {Type: lex.TypeInt},
					"repostCount": {Type: lex.TypeInt},
					"likeCount":   {Type: lex.TypeInt},
				},
			},
			"replyRef": {
				Type: lex.TypeObject,
				Properties: map[string]*lex.TypeSchema{
					"root": {
						Type: lex.TypeUnion,
						Refs: []string{"#postView", "#notFoundPost", "#blockedPost"},
					},
					"parent": {
						Type: lex.TypeUnion,
						Refs: []string{"#postView", "#notFoundPost", "#blockedPost"},
					},
				},
			},
			"notFoundPost": {
				Type: lex.TypeObject,
				Properties: map[string]*lex.TypeSchema{
					"uri":      {Type: "string", Format: "at-uri"},
					"notFound": {Type: "boolean"},
				},
			},
			"blockedPost": {
				Type:     "object",
				Required: []string{"uri", "blocked", "author"},
				Properties: map[string]*lex.TypeSchema{
					"uri":     {Type: "string", Format: "at-uri"},
					"blocked": {Type: "boolean"},
					// "author":  {Type: "ref", Ref: "#blockedAuthor"},
				},
			},
		},
	}
	g := NewGenerator("github.com/harrybrwn/at/api")
	is.True(g != nil)
	g.AddSchema(schema)
	fg, err := g.NewGenerator(schema)
	is.NoErr(err)
	is.Equal(len(fg.Types), 6)
	replyRef, err := g.Ref(schema.ID, "app.bsky.feed.defs#replyRef")
	is.NoErr(err)
	// is.Equal("FeedReplyRefRootUnion", g.typeName(replyRef.Properties["root"], "root"))
	// is.Equal("FeedReplyRefParentUnion", g.typeName(replyRef.Properties["parent"], "parent"))
	is.Equal("FeedReplyRefRootUnion", g.typeName(replyRef.Properties["root"], replyRef.DefName))
	is.Equal("FeedReplyRefParentUnion", g.typeName(replyRef.Properties["parent"], replyRef.DefName))
}
