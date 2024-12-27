package str

import (
	"strings"
	"unicode"
)

func Title[T ~string](s T) string {
	switch s {
	case "id":
		return "ID"
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
			if IsSeparator(prev) {
				prev = r
				return unicode.ToTitle(r)
			}
			prev = r
			return r
		}, string(s))
}

func IsSeparator(r rune) bool {
	// ASCII alphanumerics and underscore are not separators
	if r <= 0x7F {
		switch {
		case '0' <= r && r <= '9':
			return false
		case 'a' <= r && r <= 'z':
			return false
		case 'A' <= r && r <= 'Z':
			return false
		case r == '_':
			return false
		}
		return true
	}
	// Letters and digits are not separators
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return false
	}
	// Otherwise, all we can do for now is treat spaces as separators.
	return unicode.IsSpace(r)
}
