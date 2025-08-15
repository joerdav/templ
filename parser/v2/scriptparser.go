package parser

import (
	"strings"
	"unicode"

	"github.com/a-h/parse"
)

var scriptElement = scriptElementParser{}

type jsQuote string

const (
	jsQuoteNone     jsQuote = ""
	jsQuoteSingle   jsQuote = `'`
	jsQuoteDouble   jsQuote = `"`
	jsQuoteBacktick jsQuote = "`"
)

type scriptElementParser struct{}

func (p scriptElementParser) Parse(pi *parse.Input) (n Node, ok bool, err error) {
	start := pi.Position()

	// <
	if _, ok, err = lt.Parse(pi); err != nil || !ok {
		return
	}

	// Element name.
	e := &ScriptElement{}
	var name string
	if name, ok, err = elementNameParser.Parse(pi); err != nil || !ok {
		pi.Seek(int(start.Index))
		return
	}

	if name != "script" {
		pi.Seek(int(start.Index))
		ok = false
		return
	}

	if e.Attributes, ok, err = (attributesParser{}).Parse(pi); err != nil || !ok {
		pi.Seek(int(start.Index))
		return
	}

	// Optional whitespace.
	if _, _, err = parse.OptionalWhitespace.Parse(pi); err != nil {
		pi.Seek(int(start.Index))
		return
	}

	// >
	if _, ok, err = gt.Parse(pi); err != nil || !ok {
		pi.Seek(int(start.Index))
		return
	}

	// If there's a type attribute and it's not a JS attribute (e.g. text/javascript), we need to parse the contents as raw text.
	if !hasJavaScriptType(e.Attributes) {
		var contents string
		if contents, ok, err = parse.StringUntil(jsEndTag).Parse(pi); err != nil || !ok {
			return e, true, parse.Error("<script>: expected end tag not present", pi.Position())
		}
		e.Contents = append(e.Contents, NewScriptContentsScriptCode(contents))
		_, _, _ = jsEndTag.Parse(pi)
		return e, true, nil
	}

	// Parse the contents, we should get script text or Go expressions up until the closing tag.
	var sb strings.Builder
	var stringLiteralDelimiter jsQuote
	var inRegex bool

loop:
	for {
		if _, ok, err = jsEndTag.Parse(pi); err != nil || ok {
			break loop
		}

		// Check for Go expression first.
		var code Node
		code, ok, err = goCodeInJavaScript.Parse(pi)
		if err != nil {
			return nil, false, err
		}
		if ok {
			if sb.Len() > 0 {
				e.Contents = append(e.Contents, NewScriptContentsScriptCode(sb.String()))
				sb.Reset()
			}
			e.Contents = append(e.Contents, NewScriptContentsGo(code.(*GoCode), stringLiteralDelimiter != jsQuoteNone))
			continue loop
		}

		// Then check for comments.
		var comment string
		comment, ok, err = jsComment.Parse(pi)
		if err != nil {
			return nil, false, err
		}
		if ok {
			if sb.Len() > 0 {
				e.Contents = append(e.Contents, NewScriptContentsScriptCode(sb.String()))
				sb.Reset()
			}
			e.Contents = append(e.Contents, NewScriptContentsScriptCode(comment))
			continue loop
		}

		// Finally, parse characters.
		var c string
		c, ok, err = jsCharacter.Parse(pi)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			// Should not happen if not at EOF.
			if _, isEOF, _ := parse.EOF[string]().Parse(pi); isEOF {
				break loop
			}
			return nil, false, parse.Error("failed to parse script content", pi.Position())
		}

		if stringLiteralDelimiter == jsQuoteNone {
			if !inRegex && c == "/" && isStartOfRegex(sb.String()) {
				inRegex = true
			} else if inRegex && c == "/" {
				if sb.Len() > 0 && sb.String()[sb.Len()-1] != '\\' {
					inRegex = false
				}
			}
		}

		if !inRegex {
			if c == `"` || c == "'" || c == "`" {
				if stringLiteralDelimiter == jsQuoteNone {
					stringLiteralDelimiter = jsQuote(c)
				} else if stringLiteralDelimiter == jsQuote(c) {
					stringLiteralDelimiter = jsQuoteNone
				}
			}
		}

		sb.WriteString(c)
	}

	if sb.Len() > 0 {
		e.Contents = append(e.Contents, NewScriptContentsScriptCode(sb.String()))
	}

	e.Range = NewRange(start, pi.Position())
	return e, true, nil
}

func isStartOfRegex(s string) bool {
	s = strings.TrimRight(s, " \t\r\n")
	if len(s) == 0 {
		return true
	}

	// Check for keywords that can precede a regex.
	keywords := []string{"return", "yield", "case", "delete", "do", "else", "in", "instanceof", "new", "throw", "typeof", "void"}
	for _, kw := range keywords {
		if strings.HasSuffix(s, kw) {
			if len(s) == len(kw) || unicode.IsSpace(rune(s[len(s)-len(kw)-1])) {
				return true
			}
		}
	}

	// Check for characters that can precede a regex.
	lastChar := s[len(s)-1]
	switch lastChar {
	case '(', ',', '=', ':', '[', '!', '&', '|', '?', '{', ';':
		return true
	}

	return false
}

var javaScriptTypeAttributeValues = []string{
	"", // If the type is not set, it is JavaScript.
	"text/javascript",
	"javascript", // Obsolete, but still used.
	"module",
}

func hasJavaScriptType(attrs []Attribute) bool {
	for _, attr := range attrs {
		ca, isCA := attr.(*ConstantAttribute)
		if !isCA {
			continue
		}
		caKey, isCAKey := ca.Key.(ConstantAttributeKey)
		if !isCAKey {
			continue
		}
		if !strings.EqualFold(caKey.Name, "type") {
			continue
		}
		for _, v := range javaScriptTypeAttributeValues {
			if strings.EqualFold(ca.Value, v) {
				return true
			}
		}
		return false
	}
	return true
}

var (
	jsEndTag    = parse.String("</script>")
	endTagStart = parse.String("</")
)

var jsCharacter = parse.Any(jsEscapedCharacter, parse.AnyRune)

var (
	hexDigit        = parse.Any(parse.ZeroToNine, parse.RuneIn("abcdef"), parse.RuneIn("ABCDEF"))
	jsUnicodeEscape = parse.StringFrom(parse.String("\\u"), hexDigit, hexDigit, hexDigit, hexDigit)
)

var jsExtendedUnicodeEscape = parse.StringFrom(parse.String("\\u{"), hexDigit, parse.StringFrom(parse.AtLeast(1, parse.ZeroOrMore(hexDigit))), parse.String("}"))

var jsHexEscape = parse.StringFrom(parse.String("\\x"), hexDigit, hexDigit)

var jsBackslashEscape = parse.StringFrom(parse.String("\\"), parse.AnyRune)

var jsEscapedCharacter = parse.Any(jsBackslashEscape, jsUnicodeEscape, jsHexEscape, jsExtendedUnicodeEscape)

var jsComment = parse.Any(jsSingleLineComment, jsMultiLineComment)

var (
	jsStartSingleLineComment = parse.String("//")
	jsEndOfSingleLineComment = parse.StringFrom(parse.Or(parse.NewLine, parse.EOF[string]()))
	jsSingleLineComment      = parse.StringFrom(jsStartSingleLineComment, parse.StringUntil(jsEndOfSingleLineComment), jsEndOfSingleLineComment)
)

var (
	jsStartMultiLineComment = parse.String("/*")
	jsEndOfMultiLineComment = parse.StringFrom(parse.Or(parse.String("*/"), parse.EOF[string]()))
	jsMultiLineComment      = parse.StringFrom(jsStartMultiLineComment, parse.StringUntil(jsEndOfMultiLineComment), jsEndOfMultiLineComment, parse.OptionalWhitespace)
)
