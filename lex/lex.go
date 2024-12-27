package lex

type Type string

const (
	TypeMessage      Type = "message"
	TypeParams       Type = "params"
	TypeNull         Type = "null"
	TypeBool         Type = "boolean"
	TypeInt          Type = "integer"
	TypeString       Type = "string"
	TypeBytes        Type = "bytes"
	TypeCIDLink      Type = "cid-link"
	TypeBlob         Type = "blob"
	TypeArray        Type = "array"
	TypeToken        Type = "token"
	TypeObject       Type = "object"
	TypeRef          Type = "ref"
	TypeUnion        Type = "union"
	TypeUnknown      Type = "unknown"
	TypeRecord       Type = "record"
	TypeQuery        Type = "query"
	TypeProcedure    Type = "procedure"
	TypeSubscription Type = "subscription"
)

func (t Type) IsRPC() bool {
	switch t {
	case TypeQuery, TypeProcedure, TypeSubscription:
		return true
	}
	return false
}

const (
	FmtAtIdentifier = "at-identifier"
	FmtATURI        = "at-uri"
	FmtHandle       = "handle"
	FmtRecordKey    = "record-key"
	FmtDID          = "did"
	FmtNSID         = "nsid"
	FmtCID          = "cid"
	FmtTID          = "tid"
	FmtLang         = "language"
)

var FormatToType = map[string]string{
	FmtAtIdentifier: "syntax.AtIdentifier",
	FmtATURI:        "syntax.ATURI",
	FmtHandle:       "syntax.Handle",
	FmtRecordKey:    "syntax.RecordKey",
	FmtDID:          "syntax.DID",
	FmtNSID:         "syntax.NSID",
	// FmtCID:          "syntax.CID",
	FmtCID:  "cid.Cid",
	FmtTID:  "syntax.TID",
	FmtLang: "syntax.Language",
}

var FormatToParser = map[string]string{
	FmtAtIdentifier: "syntax.ParseAtIdentifier",
	FmtATURI:        "syntax.ParseATURI",
	FmtHandle:       "syntax.ParseHandle",
	FmtDID:          "syntax.ParseDID",
	FmtNSID:         "syntax.ParseNSID",
	// FmtCID:          "syntax.ParseCID",
	FmtCID:       "cid.Decode",
	FmtTID:       "syntax.ParseTID",
	FmtLang:      "syntax.ParseLanguage",
	FmtRecordKey: "syntax.ParseRecordKey",
}
