package lexgen

import (
	"strings"

	"github.com/harrybrwn/at/lex"
)

func trimLastDot(id string) string {
	parts := strings.Split(id, ".")
	return strings.Join(parts[:len(parts)-1], ".")
}

func lastDot(id string) string {
	parts := strings.Split(id, ".")
	return parts[len(parts)-1]
}

func last2Dots(id string) (string, string) {
	parts := strings.Split(id, ".")
	if len(parts) < 2 {
		return id, ""
	}
	return parts[len(parts)-2], parts[len(parts)-1]
}

func invalidEnc(encoding string) bool {
	switch strings.Trim(encoding, " \t\r\n") {
	case lex.EncodingANY,
		lex.EncodingCBOR,
		lex.EncodingMP4,
		lex.EncodingCAR,
		lex.EncodingJSON,
		lex.EncodingJSONL:
		return false
	}
	return true
}

func cutPrefix(s, prefix string) string {
	cut, _ := strings.CutPrefix(s, prefix)
	return cut
}
