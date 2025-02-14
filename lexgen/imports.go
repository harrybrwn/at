package lexgen

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/harrybrwn/at/lex"
)

type OrganizedImports struct {
	Std        []*Import
	ThirdParty []*Import
	Self       []*Import
}

func OrganizeImports(selfPackage string, imports []*Import) *OrganizedImports {
	var o OrganizedImports
	for i, im := range imports {
		if strings.HasPrefix(im.Path, filepath.Dir(selfPackage)) {
			o.Self = append(o.Self, imports[i])
		} else if isStdLib(im.Path) {
			o.Std = append(o.Std, imports[i])
		} else {
			o.ThirdParty = append(o.ThirdParty, imports[i])
		}
	}
	sort.Sort(Imports(o.Std))
	sort.Sort(Imports(o.ThirdParty))
	sort.Sort(Imports(o.Self))
	return &o
}

func OrganizeImportMap(selfPackage string, imports map[string]*Import) *OrganizedImports {
	imps := make([]*Import, 0)
	for _, i := range imports {
		imps = append(imps, i)
	}
	return OrganizeImports(selfPackage, imps)
}

func (oi *OrganizedImports) Generate(w io.Writer) error {
	all := [][]*Import{oi.Std, oi.ThirdParty, oi.Self}
	fmt.Fprintf(w, "import (\n")
	for i, imports := range all {
		_ = i
		for _, im := range imports {
			if len(im.Name) > 0 {
				fmt.Fprintf(w, "\t%s %q\n", im.Name, im.Path)
			} else {
				fmt.Fprintf(w, "\t%q\n", im.Path)
			}
		}
		if i < len(all)-1 && len(imports) > 0 {
			fmt.Fprintf(w, "\n")
		}
	}
	fmt.Fprintf(w, ")\n\n")
	return nil
}

func addRequiredImports(imports map[string]*Import, def *lex.TypeSchema) {
	if shouldImportStrconv(def) {
		addImport(imports, "strconv")
	}
	if isAtProtoFormat(def.Format) {
		addImport(imports, "github.com/bluesky-social/indigo/atproto/syntax")
	}
	if def.Parameters != nil {
		addImport(imports, "net/url")
	}
	if def.Input != nil && def.Input.Schema != nil {
		if def.Input.Encoding == "application/json" && def.Type == "procedure" {
			addImport(imports, "encoding/json")
		}
	}
	switch def.Type {
	case lex.TypeBlob, lex.TypeBytes, lex.TypeCIDLink:
		// TODO blobs will most likely need a special import
		addImport(imports, "github.com/bluesky-social/indigo/lex/util")
	case lex.TypeQuery, lex.TypeProcedure, lex.TypeSubscription:
		// needed by genRPC
		addImport(
			imports,
			"context",
			"log/slog",
			"net/http",
			"github.com/harrybrwn/at/xrpc",
		)
		if def.Output != nil && (def.Output.Encoding == lex.EncodingANY || def.Output.Encoding == lex.EncodingMP4 || def.Output.Encoding == lex.EncodingCAR) ||
			def.Input != nil && (def.Input.Encoding == lex.EncodingANY || def.Input.Encoding == lex.EncodingMP4 || def.Input.Encoding == lex.EncodingCAR) {
			addImport(imports, "io")
		}
		if (def.Output != nil && def.Output.Encoding == lex.EncodingJSON) ||
			(def.Input != nil && def.Input.Encoding == lex.EncodingJSON) {
			addImport(imports, "encoding/json")
		}
		if def.Message != nil {
			addImport(imports, "github.com/coder/websocket", "iter")
		}
	case lex.TypeUnion:
		addImport(imports, "encoding/json", "fmt") // for automatic MarshalJSON
	}
}

func addClientImports(imports map[string]*Import, def *lex.TypeSchema) {
	addImport(imports,
		"context",
		"github.com/harrybrwn/at/xrpc",
		"github.com/pkg/errors",
	)
	switch def.Type {
	case lex.TypeQuery:
		addImport(
			imports,
			"context",
			"net/url",
		)
		if def.Output != nil {
			switch def.Output.Encoding {
			case lex.EncodingJSON:
				addImport(imports, "encoding/json")
			case lex.EncodingCAR:
				addImport(imports, "io")
			}
		}
	case lex.TypeProcedure:
		addImport(
			imports,
			"net/url",
			"context",
		)
		if def.Input != nil {
			addImport(imports, "bytes")
			switch def.Input.Encoding {
			case lex.EncodingCAR, lex.EncodingMP4:
				addImport(imports, "io")
			case lex.EncodingJSON:
				addImport(imports, "encoding/json")
			}
		}
		if def.Output != nil {
			switch def.Output.Encoding {
			case lex.EncodingJSON:
				addImport(imports, "encoding/json")
			case lex.EncodingCAR:
				addImport(imports, "io")
			}
		}
	case lex.TypeSubscription:
		addImport(
			imports,
			"context",
			"iter",
		)
	}

}

func addImport(m map[string]*Import, paths ...string) {
	for _, path := range paths {
		if _, ok := m[path]; !ok {
			m[path] = &Import{Path: path}
		}
	}
}

func isStdLib(pkg string) bool {
	switch pkg {
	case "debug", "debug/dwarf",
		"debug/macho", "debug/pe", "debug/gosym", "debug/elf", "debug/buildinfo", "debug/plan9obj", "encoding",
		"encoding/base64", "encoding/binary", "encoding/pem", "encoding/ascii85", "encoding/base32", "encoding/gob",
		"encoding/json", "encoding/asn1", "encoding/csv", "encoding/hex", "encoding/xml", "regexp", "regexp/syntax",
		"strings", "testing", "testing/slogtest", "testing/quick", "testing/fstest", "testing/iotest", "math",
		"math/bits", "math/rand", "math/rand/v2", "math/big", "math/cmplx", "time", "time/tzdata", "text",
		"text/template", "text/template/parse", "text/tabwriter", "text/scanner", "os", "os/signal", "os/exec",
		"os/user", "html", "html/template", "hash", "hash/crc32", "hash/fnv", "hash/crc64", "hash/adler32",
		"hash/maphash", "compress", "compress/gzip", "compress/flate", "compress/bzip2", "compress/lzw",
		"compress/zlib", "builtin", "io", "io/ioutil", "io/fs", "flag", "container", "container/list",
		"container/heap", "container/ring", "bytes", "runtime", "runtime/debug", "runtime/asan",
		"runtime/cgo", "runtime/race", "runtime/trace", "runtime/msan", "runtime/pprof", "runtime/metrics",
		"runtime/coverage", "plugin", "net", "net/url", "net/http", "net/http/cookiejar", "net/http/httptrace",
		"net/http/httptest", "net/http/httputil", "net/http/pprof", "net/http/cgi", "net/http/fcgi", "net/smtp",
		"net/mail", "net/textproto", "net/netip", "net/rpc", "net/rpc/jsonrpc", "iter", "errors",
		"unicode", "unicode/utf8", "unicode/utf16", "arena", "strconv", "bufio", "context", "structs",
		"image", "image/draw", "image/color", "image/color/palette", "image/jpeg", "image/gif", "image/png",
		"slices", "embed", "syscall", "syscall/js", "expvar", "mime", "mime/quotedprintable", "mime/multipart",
		"cmp", "index", "index/suffixarray", "database", "database/sql", "database/sql/driver", "archive",
		"archive/zip", "archive/tar", "crypto", "crypto/hmac", "crypto/rsa", "crypto/rand", "crypto/ecdsa",
		"crypto/des", "crypto/aes", "crypto/sha256", "crypto/boring", "crypto/ed25519", "crypto/rc4",
		"crypto/sha1", "crypto/ecdh", "crypto/x509", "crypto/x509/pkix", "crypto/tls", "crypto/tls/fipsonly",
		"crypto/cipher", "crypto/md5", "crypto/dsa", "crypto/sha512", "crypto/elliptic", "crypto/subtle",
		"go/scanner", "go/types", "go/constant", "path", "path/filepath", "log", "log/syslog",
		"log/slog", "unsafe", "sync", "sync/atomic", "sort", "unique", "fmt", "reflect", "maps":
		return true
	}
	return false
}
