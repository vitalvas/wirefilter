package wirefilter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func BenchmarkLexer(b *testing.B) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "simple expression",
			input: `http.host == "example.com"`,
		},
		{
			name:  "complex expression",
			input: `http.host == "example.com" and http.status >= 400 or http.path contains "/api"`,
		},
		{
			name:  "array expression",
			input: `http.status in {200, 201, 204, 301, 302, 304}`,
		},
		{
			name:  "range expression",
			input: `port in {80..100, 443, 8000..9000}`,
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				lexer := NewLexer(tt.input)
				for {
					tok := lexer.NextToken()
					if tok.Type == TokenEOF {
						break
					}
				}
			}
		})
	}
}

func FuzzLexer(f *testing.F) {
	f.Add(`http.host == "example.com"`)
	f.Add(`http.status >= 400`)
	f.Add(`http.host == "example.com" and http.status >= 400`)
	f.Add(`(http.host == "test.com" or http.path contains "/api") and http.status < 500`)
	f.Add(`http.status in {200, 201, 204, 301, 302, 304}`)
	f.Add(`port in {80..100, 443, 8000..9000}`)
	f.Add(`ip.src in "192.168.0.0/16"`)
	f.Add(`http.path matches "^/api/v[0-9]+/"`)
	f.Add(`not http.host == "blocked.com"`)
	f.Add(`true and false`)
	f.Add(`""`)
	f.Add(`"string with \"escape\""`)
	f.Add(`field === "value"`)
	f.Add(`field !== "value"`)
	f.Add(`ip not in $blocked`)
	f.Add(`name not contains "admin"`)
	f.Add(`$list_ref`)
	f.Add(`$table[field]`)
	f.Add(`$geo[ip.src]`)
	f.Add(`192.168.0.0/24`)
	f.Add(`2001:db8::/32`)
	f.Add(`r"raw\nstring"`)
	f.Add(`strict wildcard "*.com"`)
	f.Add(`strict other`)
	f.Add(`-42`)
	f.Add(`a xor b`)
	f.Add(`cidr(ip, 24)`)
	f.Add(`cidr6(ip, 64)`)

	f.Fuzz(func(_ *testing.T, input string) {
		lexer := NewLexer(input)
		for {
			tok := lexer.NextToken()
			if tok.Type == TokenEOF || tok.Type == TokenError {
				break
			}
		}
	})
}

func TestLexer(t *testing.T) {
	t.Run("operators", func(t *testing.T) {
		input := "== != === !== < > <= >= && || and or not ^^ xor ~ !"
		lexer := NewLexer(input)

		tests := []TokenType{
			TokenEq, TokenNe, TokenAllEq, TokenAnyNe, TokenLt, TokenGt, TokenLe, TokenGe,
			TokenAnd, TokenOr, TokenAnd, TokenOr, TokenNot, TokenXor, TokenXor, TokenMatches, TokenNot, TokenEOF,
		}

		for _, expected := range tests {
			tok := lexer.NextToken()
			assert.Equal(t, expected, tok.Type)
		}
	})

	t.Run("keywords", func(t *testing.T) {
		input := "contains matches in"
		lexer := NewLexer(input)

		tests := []TokenType{TokenContains, TokenMatches, TokenIn, TokenEOF}

		for _, expected := range tests {
			tok := lexer.NextToken()
			assert.Equal(t, expected, tok.Type)
		}
	})

	t.Run("literals", func(t *testing.T) {
		input := `"test string" 42 -10 true false`
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenString, tok.Type)
		assert.Equal(t, "test string", tok.Literal)

		tok = lexer.NextToken()
		assert.Equal(t, TokenInt, tok.Type)
		assert.Equal(t, int64(42), tok.Value)

		tok = lexer.NextToken()
		assert.Equal(t, TokenInt, tok.Type)
		assert.Equal(t, int64(-10), tok.Value)

		tok = lexer.NextToken()
		assert.Equal(t, TokenBool, tok.Type)
		assert.Equal(t, true, tok.Value)

		tok = lexer.NextToken()
		assert.Equal(t, TokenBool, tok.Type)
		assert.Equal(t, false, tok.Value)
	})

	t.Run("identifiers", func(t *testing.T) {
		input := "http.method user.name field_name"
		lexer := NewLexer(input)

		tests := []string{"http.method", "user.name", "field_name"}

		for _, expected := range tests {
			tok := lexer.NextToken()
			assert.Equal(t, TokenIdent, tok.Type)
			assert.Equal(t, expected, tok.Literal)
		}
	})

	t.Run("delimiters", func(t *testing.T) {
		input := "( ) { } ,"
		lexer := NewLexer(input)

		tests := []TokenType{
			TokenLParen, TokenRParen, TokenLBrace, TokenRBrace, TokenComma, TokenEOF,
		}

		for _, expected := range tests {
			tok := lexer.NextToken()
			assert.Equal(t, expected, tok.Type)
		}
	})

	t.Run("complex expression", func(t *testing.T) {
		input := `http.method == "GET" && port in {80, 443}`
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
		assert.Equal(t, "http.method", tok.Literal)

		tok = lexer.NextToken()
		assert.Equal(t, TokenEq, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenString, tok.Type)
		assert.Equal(t, "GET", tok.Literal)

		tok = lexer.NextToken()
		assert.Equal(t, TokenAnd, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
		assert.Equal(t, "port", tok.Literal)

		tok = lexer.NextToken()
		assert.Equal(t, TokenIn, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenLBrace, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenInt, tok.Type)
		assert.Equal(t, int64(80), tok.Value)

		tok = lexer.NextToken()
		assert.Equal(t, TokenComma, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenInt, tok.Type)
		assert.Equal(t, int64(443), tok.Value)

		tok = lexer.NextToken()
		assert.Equal(t, TokenRBrace, tok.Type)
	})

	t.Run("string escape sequences", func(t *testing.T) {
		input := `"hello\nworld\t\r\\\"test"`
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenString, tok.Type)
		assert.Equal(t, "hello\nworld\t\r\\\"test", tok.Literal)
	})

	t.Run("string with unknown escape", func(t *testing.T) {
		input := `"test\xvalue"`
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenString, tok.Type)
		assert.Equal(t, "testxvalue", tok.Literal)
	})

	t.Run("unterminated string", func(t *testing.T) {
		input := `"unterminated`
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenError, tok.Type)
	})

	t.Run("identifier with colon not ip", func(t *testing.T) {
		input := "field:value"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
		assert.Equal(t, "field:value", tok.Literal)
	})

	t.Run("identifier that looks like ip but isnt", func(t *testing.T) {
		input := "abc:def:ghi"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
	})

	t.Run("range token", func(t *testing.T) {
		input := "1..10"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenInt, tok.Type)
		assert.Equal(t, int64(1), tok.Value)

		tok = lexer.NextToken()
		assert.Equal(t, TokenRange, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenInt, tok.Type)
		assert.Equal(t, int64(10), tok.Value)
	})

	t.Run("error method", func(t *testing.T) {
		lexer := NewLexer("test")
		err := lexer.Error("test error %s", "message")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "lexer error")
		assert.Contains(t, err.Error(), "test error message")
	})

	t.Run("single dot not range", func(t *testing.T) {
		input := "field.name"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
		assert.Equal(t, "field.name", tok.Literal)
	})

	t.Run("looksLikeIP empty string", func(t *testing.T) {
		input := `field == ""`
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
	})

	t.Run("peek char at end", func(t *testing.T) {
		input := "a"
		lexer := NewLexer(input)
		lexer.NextToken()
		tok := lexer.NextToken()
		assert.Equal(t, TokenEOF, tok.Type)
	})

	t.Run("single ampersand", func(t *testing.T) {
		input := "a & b"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenError, tok.Type) // Unknown character
	})

	t.Run("single pipe", func(t *testing.T) {
		input := "a | b"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenError, tok.Type) // Unknown character
	})

	t.Run("single equals", func(t *testing.T) {
		input := "a = b"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenError, tok.Type) // Unknown character
	})

	t.Run("single exclamation as not operator", func(t *testing.T) {
		input := "a ! b"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenNot, tok.Type)
		assert.Equal(t, "!", tok.Literal)

		tok = lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
	})

	t.Run("looksLikeIP with empty string check", func(t *testing.T) {
		result := looksLikeIP("")
		assert.False(t, result)
	})

	t.Run("looksLikeIP with letter only", func(t *testing.T) {
		result := looksLikeIP("hostname")
		assert.False(t, result)
	})

	t.Run("looksLikeIP with digit start", func(t *testing.T) {
		result := looksLikeIP("192")
		assert.True(t, result)
	})

	t.Run("looksLikeIP with colon", func(t *testing.T) {
		result := looksLikeIP("abc:def")
		assert.True(t, result)
	})

	t.Run("single dot", func(t *testing.T) {
		input := "."
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenError, tok.Type) // Unknown character
	})

	t.Run("identifier with underscore", func(t *testing.T) {
		input := "field_name"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
		assert.Equal(t, "field_name", tok.Literal)
	})

	t.Run("identifier with hyphen", func(t *testing.T) {
		input := "field-name"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
		assert.Equal(t, "field-name", tok.Literal)
	})

	t.Run("identifier with slash", func(t *testing.T) {
		input := "path/name"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
		assert.Equal(t, "path/name", tok.Literal)
	})

	t.Run("uppercase keywords", func(t *testing.T) {
		input := "AND OR NOT CONTAINS MATCHES IN TRUE FALSE"
		lexer := NewLexer(input)

		tests := []TokenType{TokenAnd, TokenOr, TokenNot, TokenContains, TokenMatches, TokenIn, TokenBool, TokenBool, TokenEOF}

		for _, expected := range tests {
			tok := lexer.NextToken()
			assert.Equal(t, expected, tok.Type)
		}
	})

	t.Run("tilde as matches alias", func(t *testing.T) {
		input := `field ~ "pattern"`
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenMatches, tok.Type)
		assert.Equal(t, "~", tok.Literal)

		tok = lexer.NextToken()
		assert.Equal(t, TokenString, tok.Type)
	})

	t.Run("xor operator symbol", func(t *testing.T) {
		input := "a ^^ b"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenXor, tok.Type)
		assert.Equal(t, "^^", tok.Literal)

		tok = lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
	})

	t.Run("xor operator keyword", func(t *testing.T) {
		input := "a xor b"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenXor, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
	})

	t.Run("wildcard operator", func(t *testing.T) {
		input := `field wildcard "*.example.com"`
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenWildcard, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenString, tok.Type)
	})

	t.Run("strict wildcard operator", func(t *testing.T) {
		input := `field strict wildcard "*.Example.com"`
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
		assert.Equal(t, "field", tok.Literal)

		tok = lexer.NextToken()
		assert.Equal(t, TokenStrictWildcard, tok.Type)
		assert.Equal(t, "strict wildcard", tok.Literal)

		tok = lexer.NextToken()
		assert.Equal(t, TokenString, tok.Type)
	})

	t.Run("strict alone is identifier", func(t *testing.T) {
		input := "strict"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
		assert.Equal(t, "strict", tok.Literal)
	})

	t.Run("strict followed by non-wildcard is identifier", func(t *testing.T) {
		input := "strict other"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
		assert.Equal(t, "strict", tok.Literal)

		tok = lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
		assert.Equal(t, "other", tok.Literal)
	})

	t.Run("single caret", func(t *testing.T) {
		input := "a ^ b"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenError, tok.Type) // Unknown character
	})

	t.Run("uppercase wildcard keywords", func(t *testing.T) {
		input := "WILDCARD XOR"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenWildcard, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenXor, tok.Type)
	})

	t.Run("strict wildcard case insensitive", func(t *testing.T) {
		input := "STRICT WILDCARD"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenStrictWildcard, tok.Type)
	})

	t.Run("raw string basic", func(t *testing.T) {
		input := `r"path\to\file"`
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenRawString, tok.Type)
		assert.Equal(t, `path\to\file`, tok.Literal)
		assert.Equal(t, `path\to\file`, tok.Value)
	})

	t.Run("raw string with regex", func(t *testing.T) {
		input := `r"^\d+\.\d+\.\d+\.\d+$"`
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenRawString, tok.Type)
		assert.Equal(t, `^\d+\.\d+\.\d+\.\d+$`, tok.Literal)
	})

	t.Run("raw string empty", func(t *testing.T) {
		input := `r""`
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenRawString, tok.Type)
		assert.Equal(t, "", tok.Literal)
	})

	t.Run("asterisk token", func(t *testing.T) {
		input := "*"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenAsterisk, tok.Type)
		assert.Equal(t, "*", tok.Literal)
	})

	t.Run("list reference basic", func(t *testing.T) {
		input := "$blocked_ips"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenListRef, tok.Type)
		assert.Equal(t, "blocked_ips", tok.Literal)
		assert.Equal(t, "blocked_ips", tok.Value)
	})

	t.Run("list reference with hyphen", func(t *testing.T) {
		input := "$admin-roles"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenListRef, tok.Type)
		assert.Equal(t, "admin-roles", tok.Literal)
	})

	t.Run("array unpack syntax", func(t *testing.T) {
		input := "tags[*]"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
		assert.Equal(t, "tags", tok.Literal)

		tok = lexer.NextToken()
		assert.Equal(t, TokenLBracket, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenAsterisk, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenRBracket, tok.Type)
	})

	t.Run("array index syntax", func(t *testing.T) {
		input := "tags[0]"
		lexer := NewLexer(input)

		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
		assert.Equal(t, "tags", tok.Literal)

		tok = lexer.NextToken()
		assert.Equal(t, TokenLBracket, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenInt, tok.Type)
		assert.Equal(t, int64(0), tok.Value)

		tok = lexer.NextToken()
		assert.Equal(t, TokenRBracket, tok.Type)
	})

	t.Run("unterminated string", func(t *testing.T) {
		lexer := NewLexer(`"unterminated`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenError, tok.Type)
	})

	t.Run("unterminated raw string", func(t *testing.T) {
		lexer := NewLexer(`r"unterminated`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenError, tok.Type)
	})

	t.Run("integer overflow", func(t *testing.T) {
		lexer := NewLexer(`99999999999999999999999`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenError, tok.Type)
		assert.Contains(t, tok.Value.(string), "integer overflow")
	})

	t.Run("invalid number or IP", func(t *testing.T) {
		lexer := NewLexer(`123.456.789.0`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenError, tok.Type)
		assert.Contains(t, tok.Value.(string), "invalid number or IP")
	})

	t.Run("strict without wildcard", func(t *testing.T) {
		lexer := NewLexer(`strict == "test"`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
		assert.Equal(t, "strict", tok.Literal)
	})

	t.Run("strict at end of input", func(t *testing.T) {
		lexer := NewLexer(`strict`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
		assert.Equal(t, "strict", tok.Literal)
	})

	t.Run("strict followed by non-wildcard identifier", func(t *testing.T) {
		lexer := NewLexer(`strict other`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
		assert.Equal(t, "strict", tok.Literal)
	})

	t.Run("negative integer", func(t *testing.T) {
		lexer := NewLexer(`-42`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenInt, tok.Type)
		assert.Equal(t, int64(-42), tok.Value)
	})

	t.Run("unexpected character", func(t *testing.T) {
		lexer := NewLexer(`@`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenError, tok.Type)
	})

	t.Run("float literal", func(t *testing.T) {
		lexer := NewLexer(`3.14`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenFloat, tok.Type)
		assert.Equal(t, "3.14", tok.Literal)
		assert.Equal(t, 3.14, tok.Value)
	})

	t.Run("negative float literal", func(t *testing.T) {
		lexer := NewLexer(`-2.5`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenFloat, tok.Type)
		assert.Equal(t, "-2.5", tok.Literal)
		assert.Equal(t, -2.5, tok.Value)
	})

	t.Run("float with leading zero", func(t *testing.T) {
		lexer := NewLexer(`0.5`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenFloat, tok.Type)
		assert.Equal(t, 0.5, tok.Value)
	})

	t.Run("float vs IP disambiguation", func(t *testing.T) {
		// IP has multiple dots
		lexer := NewLexer(`192.168.1.1`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenIP, tok.Type)

		// Float has single dot
		lexer2 := NewLexer(`99.5`)
		tok2 := lexer2.NextToken()
		assert.Equal(t, TokenFloat, tok2.Type)
	})

	t.Run("float in expression", func(t *testing.T) {
		lexer := NewLexer(`score > 3.14`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
		assert.Equal(t, "score", tok.Literal)

		tok = lexer.NextToken()
		assert.Equal(t, TokenGt, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenFloat, tok.Type)
		assert.Equal(t, 3.14, tok.Value)
	})

	t.Run("float token string", func(t *testing.T) {
		assert.Equal(t, "FLOAT", TokenFloat.String())
	})

	t.Run("list ref followed by bracket", func(t *testing.T) {
		lexer := NewLexer(`$geo[ip.src]`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenListRef, tok.Type)
		assert.Equal(t, "geo", tok.Literal)

		tok = lexer.NextToken()
		assert.Equal(t, TokenLBracket, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
		assert.Equal(t, "ip.src", tok.Literal)

		tok = lexer.NextToken()
		assert.Equal(t, TokenRBracket, tok.Type)
	})

	t.Run("list ref without bracket", func(t *testing.T) {
		lexer := NewLexer(`$blocked_ips`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenListRef, tok.Type)
		assert.Equal(t, "blocked_ips", tok.Literal)

		tok = lexer.NextToken()
		assert.Equal(t, TokenEOF, tok.Type)
	})

	t.Run("list ref in expression", func(t *testing.T) {
		lexer := NewLexer(`ip in $nets`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenIn, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenListRef, tok.Type)
		assert.Equal(t, "nets", tok.Literal)
	})
}

func TestLexerCIDRAndIPParsing(t *testing.T) {
	t.Run("valid CIDR starting with letter in identifier path", func(t *testing.T) {
		// fe80::/10 starts with 'f', enters readIdentifierToken
		lexer := NewLexer(`ip in fe80::/10`)
		lexer.NextToken() // ip
		lexer.NextToken() // in
		tok := lexer.NextToken()
		assert.Equal(t, TokenCIDR, tok.Type)
	})

	t.Run("looks like CIDR but invalid in identifier path", func(t *testing.T) {
		// Starts with letter, has '/', looksLikeCIDR true, but ParseCIDR fails
		lexer := NewLexer(`ip in fzzz::/999`)
		lexer.NextToken() // ip
		lexer.NextToken() // in
		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
	})

	t.Run("looks like IP but invalid in identifier path", func(t *testing.T) {
		// Starts with letter, has ':', looksLikeIP true, but ParseIP fails
		lexer := NewLexer(`x == fzzz::gggg`)
		lexer.NextToken() // x
		lexer.NextToken() // ==
		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
	})

	t.Run("valid inline IPv6 CIDR from number path", func(t *testing.T) {
		lexer := NewLexer(`ip in 2001:db8::/32`)
		lexer.NextToken() // ip
		lexer.NextToken() // in
		tok := lexer.NextToken()
		assert.Equal(t, TokenCIDR, tok.Type)
	})

	t.Run("valid IPv6 IP starting with letter", func(t *testing.T) {
		lexer := NewLexer(`ip == fe80::1`)
		lexer.NextToken() // ip
		lexer.NextToken() // ==
		tok := lexer.NextToken()
		assert.Equal(t, TokenIP, tok.Type)
	})
}

func TestLexerTimestampAndDuration(t *testing.T) {
	t.Run("RFC 3339 timestamp", func(t *testing.T) {
		lexer := NewLexer(`2026-03-19T10:00:00Z`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenTime, tok.Type)
		assert.Equal(t, "2026-03-19T10:00:00Z", tok.Literal)
		expected := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
		assert.True(t, expected.Equal(tok.Value.(time.Time)))
	})

	t.Run("RFC 3339 timestamp with fractional seconds", func(t *testing.T) {
		lexer := NewLexer(`2026-03-19T10:00:00.123456789Z`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenTime, tok.Type)
		expected := time.Date(2026, 3, 19, 10, 0, 0, 123456789, time.UTC)
		assert.True(t, expected.Equal(tok.Value.(time.Time)))
	})

	t.Run("RFC 3339 timestamp with timezone offset", func(t *testing.T) {
		lexer := NewLexer(`2026-03-19T10:00:00+05:00`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenTime, tok.Type)
		expected := time.Date(2026, 3, 19, 5, 0, 0, 0, time.UTC)
		assert.True(t, expected.Equal(tok.Value.(time.Time)))
	})

	t.Run("simple duration 30m", func(t *testing.T) {
		lexer := NewLexer(`30m`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenDuration, tok.Type)
		assert.Equal(t, "30m", tok.Literal)
		assert.Equal(t, 30*time.Minute, tok.Value.(time.Duration))
	})

	t.Run("duration 7d", func(t *testing.T) {
		lexer := NewLexer(`7d`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenDuration, tok.Type)
		assert.Equal(t, 7*24*time.Hour, tok.Value.(time.Duration))
	})

	t.Run("compound duration 1h30m", func(t *testing.T) {
		lexer := NewLexer(`1h30m`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenDuration, tok.Type)
		assert.Equal(t, "1h30m", tok.Literal)
		assert.Equal(t, time.Hour+30*time.Minute, tok.Value.(time.Duration))
	})

	t.Run("compound duration 2d4h30m15s", func(t *testing.T) {
		lexer := NewLexer(`2d4h30m15s`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenDuration, tok.Type)
		expected := 2*24*time.Hour + 4*time.Hour + 30*time.Minute + 15*time.Second
		assert.Equal(t, expected, tok.Value.(time.Duration))
	})

	t.Run("duration in expression", func(t *testing.T) {
		lexer := NewLexer(`ttl >= 30m`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)
		assert.Equal(t, "ttl", tok.Literal)

		tok = lexer.NextToken()
		assert.Equal(t, TokenGe, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenDuration, tok.Type)
		assert.Equal(t, 30*time.Minute, tok.Value.(time.Duration))
	})

	t.Run("timestamp in expression", func(t *testing.T) {
		lexer := NewLexer(`created_at >= 2026-03-19T10:00:00Z`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenIdent, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenGe, tok.Type)

		tok = lexer.NextToken()
		assert.Equal(t, TokenTime, tok.Type)
	})

	t.Run("invalid timestamp", func(t *testing.T) {
		lexer := NewLexer(`2026-13-19T99:00:00Z`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenError, tok.Type)
	})

	t.Run("duration 5s", func(t *testing.T) {
		lexer := NewLexer(`5s`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenDuration, tok.Type)
		assert.Equal(t, 5*time.Second, tok.Value.(time.Duration))
	})

	t.Run("integer not confused with duration", func(t *testing.T) {
		lexer := NewLexer(`42`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenInt, tok.Type)
		assert.Equal(t, int64(42), tok.Value)
	})

	t.Run("IP not confused with timestamp", func(t *testing.T) {
		lexer := NewLexer(`192.168.0.1`)
		tok := lexer.NextToken()
		assert.Equal(t, TokenIP, tok.Type)
	})
}
