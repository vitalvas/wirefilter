package wirefilter

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Lexer tokenizes filter expression strings into tokens.
type Lexer struct {
	input string
	pos   int
	ch    byte
}

// NewLexer creates a new lexer for the given input string.
func NewLexer(input string) *Lexer {
	l := &Lexer{input: input}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.pos >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.pos]
	}
	l.pos++
}

func (l *Lexer) peekChar() byte {
	if l.pos >= len(l.input) {
		return 0
	}
	return l.input[l.pos]
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
		l.readChar()
	}
}

// readOperatorToken handles multi-character operators.
func (l *Lexer) readOperatorToken() (Token, bool) {
	switch l.ch {
	case '=':
		if l.peekChar() == '=' {
			l.readChar()
			if l.peekChar() == '=' {
				l.readChar()
				return Token{Type: TokenAllEq, Literal: "==="}, true
			}
			return Token{Type: TokenEq, Literal: "=="}, true
		}
	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			if l.peekChar() == '=' {
				l.readChar()
				return Token{Type: TokenAnyNe, Literal: "!=="}, true
			}
			return Token{Type: TokenNe, Literal: "!="}, true
		}
		return Token{Type: TokenNot, Literal: "!"}, true
	case '<':
		if l.peekChar() == '=' {
			l.readChar()
			return Token{Type: TokenLe, Literal: "<="}, true
		}
		return Token{Type: TokenLt, Literal: "<"}, true
	case '>':
		if l.peekChar() == '=' {
			l.readChar()
			return Token{Type: TokenGe, Literal: ">="}, true
		}
		return Token{Type: TokenGt, Literal: ">"}, true
	case '&':
		if l.peekChar() == '&' {
			l.readChar()
			return Token{Type: TokenAnd, Literal: "&&"}, true
		}
	case '|':
		if l.peekChar() == '|' {
			l.readChar()
			return Token{Type: TokenOr, Literal: "||"}, true
		}
	case '^':
		if l.peekChar() == '^' {
			l.readChar()
			return Token{Type: TokenXor, Literal: "^^"}, true
		}
	case '.':
		if l.peekChar() == '.' {
			l.readChar()
			return Token{Type: TokenRange, Literal: ".."}, true
		}
	}
	return Token{}, false
}

// NextToken returns the next token from the input.
func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	if tok, ok := l.readOperatorToken(); ok {
		l.readChar()
		return tok
	}

	var tok Token

	switch l.ch {
	case 0:
		tok = Token{Type: TokenEOF}
	case '~':
		tok = Token{Type: TokenMatches, Literal: "~"}
	case '(':
		tok = Token{Type: TokenLParen, Literal: "("}
	case ')':
		tok = Token{Type: TokenRParen, Literal: ")"}
	case '{':
		tok = Token{Type: TokenLBrace, Literal: "{"}
	case '}':
		tok = Token{Type: TokenRBrace, Literal: "}"}
	case '[':
		tok = Token{Type: TokenLBracket, Literal: "["}
	case ']':
		tok = Token{Type: TokenRBracket, Literal: "]"}
	case ',':
		tok = Token{Type: TokenComma, Literal: ","}
	case '"':
		tok.Literal, tok.Value = l.readString()
		if tok.Value == nil {
			tok.Type = TokenError
		} else {
			tok.Type = TokenString
		}
	case '*':
		tok = Token{Type: TokenAsterisk, Literal: "*"}
	case '+':
		tok = Token{Type: TokenPlus, Literal: "+"}
	case '%':
		tok = Token{Type: TokenMod, Literal: "%"}
	case '$':
		tok.Type = TokenListRef
		tok.Literal = l.readListName()
		tok.Value = tok.Literal
		return tok
	default:
		switch {
		case isLetter(l.ch):
			return l.readIdentifierToken()
		case isDigit(l.ch) || (l.ch == '-' && isDigit(l.peekChar())):
			return l.readNumberToken()
		case l.ch == '-':
			tok = Token{Type: TokenMinus, Literal: "-"}
		case l.ch == '/':
			tok = Token{Type: TokenDiv, Literal: "/"}
		default:
			tok = Token{Type: TokenError, Literal: string(l.ch), Value: fmt.Sprintf("unexpected character: %c", l.ch)}
		}
	}

	l.readChar()
	return tok
}

func (l *Lexer) readString() (string, any) {
	l.readChar()
	start := l.pos - 1

	// Fast path: check if string has no escape sequences
	hasEscape := false
	for l.ch != '"' && l.ch != 0 {
		if l.ch == '\\' {
			hasEscape = true
			break
		}
		l.readChar()
	}

	// Check for unterminated string
	if l.ch == 0 && !hasEscape {
		return l.input[start : l.pos-1], nil // nil value indicates error
	}

	// If no escapes, return substring directly (zero allocation)
	if !hasEscape {
		result := l.input[start : l.pos-1]
		return result, result
	}

	// Slow path: handle escape sequences
	var result strings.Builder
	result.Grow(l.pos - start + 16) // Pre-allocate with estimate

	// Copy what we've already scanned
	result.WriteString(l.input[start : l.pos-1])

	// Continue scanning with escape handling
	for l.ch != '"' && l.ch != 0 {
		if l.ch == '\\' {
			l.readChar()
			switch l.ch {
			case 'n':
				result.WriteByte('\n')
			case 't':
				result.WriteByte('\t')
			case 'r':
				result.WriteByte('\r')
			case '\\':
				result.WriteByte('\\')
			case '"':
				result.WriteByte('"')
			default:
				result.WriteByte(l.ch)
			}
		} else {
			result.WriteByte(l.ch)
		}
		l.readChar()
	}

	// Check for unterminated string
	if l.ch == 0 {
		return result.String(), nil // nil value indicates error
	}

	str := result.String()
	return str, str
}

func (l *Lexer) readIdentifier() string {
	start := l.pos - 1
	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '.' || l.ch == '_' || l.ch == '-' || l.ch == ':' || l.ch == '/' {
		l.readChar()
	}
	return l.input[start : l.pos-1]
}

func (l *Lexer) readRawString() (string, bool) {
	l.readChar() // consume opening "
	start := l.pos - 1

	for l.ch != '"' && l.ch != 0 {
		l.readChar()
	}

	if l.ch == 0 {
		return l.input[start : l.pos-1], false // unterminated
	}

	return l.input[start : l.pos-1], true
}

func (l *Lexer) readListName() string {
	l.readChar() // consume $
	start := l.pos - 1

	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' || l.ch == '-' {
		l.readChar()
	}

	return l.input[start : l.pos-1]
}

// isLetter checks if the byte is an ASCII letter (fast path for common case).
func isLetter(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

// isDigit checks if the byte is an ASCII digit (fast path for common case).
func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

// isHexChar checks if the byte is a hex character (a-f, A-F).
func isHexChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

func (l *Lexer) readIdentifierToken() Token {
	// Check for raw string r"..."
	if l.ch == 'r' && l.peekChar() == '"' {
		l.readChar() // consume 'r'
		literal, ok := l.readRawString()
		if !ok {
			return Token{
				Type:    TokenError,
				Literal: literal,
				Value:   "unterminated raw string",
			}
		}
		l.readChar() // consume closing "
		return Token{
			Type:    TokenRawString,
			Literal: literal,
			Value:   literal,
		}
	}

	literal := l.readIdentifier()
	tok := Token{Literal: literal}

	// Fast case-insensitive keyword matching
	lower := strings.ToLower(literal)
	switch lower {
	case "and":
		tok.Type = TokenAnd
	case "or":
		tok.Type = TokenOr
	case "not":
		tok.Type = TokenNot
	case "contains":
		tok.Type = TokenContains
	case "matches":
		tok.Type = TokenMatches
	case "in":
		tok.Type = TokenIn
	case "xor":
		tok.Type = TokenXor
	case "wildcard":
		tok.Type = TokenWildcard
	case "strict":
		// Look ahead for "wildcard" to form "strict wildcard"
		// Save position for potential rollback
		savedPos := l.pos
		savedCh := l.ch
		l.skipWhitespace()
		if l.ch != 0 && isLetter(l.ch) {
			startPos := l.pos - 1
			for isLetter(l.ch) || isDigit(l.ch) || l.ch == '.' || l.ch == '_' || l.ch == '-' || l.ch == ':' || l.ch == '/' {
				l.readChar()
			}
			nextLiteral := l.input[startPos : l.pos-1]
			if strings.ToLower(nextLiteral) == "wildcard" {
				tok.Type = TokenStrictWildcard
				tok.Literal = "strict wildcard"
				return tok
			}
			// Not "wildcard", restore position and treat "strict" as identifier
			l.pos = savedPos
			l.ch = savedCh
		} else {
			// No following identifier, restore position
			l.pos = savedPos
			l.ch = savedCh
		}
		tok.Type = TokenIdent
		tok.Value = literal
	case "true":
		tok.Type = TokenBool
		tok.Value = true
	case "false":
		tok.Type = TokenBool
		tok.Value = false
	default:
		// Try to parse as CIDR first (e.g., 192.168.0.0/24, 2001:db8::/32)
		if looksLikeCIDR(literal) {
			if _, ipNet, err := net.ParseCIDR(literal); err == nil {
				tok.Type = TokenCIDR
				tok.Value = ipNet
				return tok
			}
		}
		// Try to parse as IP if it looks like one (starts with digit or contains colon for IPv6)
		if looksLikeIP(literal) {
			if ip := NormalizeIP(net.ParseIP(literal)); ip != nil {
				tok.Type = TokenIP
				tok.Value = ip
				return tok
			}
		}
		tok.Type = TokenIdent
		tok.Value = literal
	}
	return tok
}

// looksLikeCIDR returns true if the literal might be a CIDR notation (contains /).
func looksLikeCIDR(s string) bool {
	// CIDR must contain a / and start with a digit or hex char (for IPv6)
	hasSlash := false
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			hasSlash = true
			break
		}
	}
	if !hasSlash {
		return false
	}
	// Must start with a digit (IPv4) or contain colon (IPv6)
	return looksLikeIP(s)
}

// looksLikeIP returns true if the literal might be an IP address.
// This is a fast heuristic to avoid calling net.ParseIP on every identifier.
func looksLikeIP(s string) bool {
	if len(s) == 0 {
		return false
	}
	// IPv4 starts with a digit, IPv6 can start with a digit or letter followed by colons
	firstChar := s[0]
	if firstChar >= '0' && firstChar <= '9' {
		// Could be IPv4 or IPv6 starting with digit
		return true
	}
	// Check for IPv6 with :: or hex prefix
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return true
		}
	}
	return false
}

// isDurationSuffix checks if a byte is a valid duration unit suffix.
func isDurationSuffix(ch byte) bool {
	return ch == 'd' || ch == 'h' || ch == 'm' || ch == 's'
}

// isTimestampChar checks if a byte is valid within an RFC 3339 timestamp literal.
func isTimestampChar(ch byte) bool {
	return isDigit(ch) || ch == '-' || ch == 'T' || ch == 't' ||
		ch == ':' || ch == 'Z' || ch == 'z' || ch == '+' || ch == '.'
}

// readTimestampToken reads an RFC 3339 timestamp literal starting from start position.
// Called when we've read exactly 4 digits followed by '-'.
func (l *Lexer) readTimestampToken(start int) Token {
	// Continue reading characters valid in RFC 3339: digits, -, T, t, :, Z, z, +, .
	// Stop at '..' (range operator)
	for isTimestampChar(l.ch) && (l.ch != '.' || l.peekChar() != '.') {
		l.readChar()
	}
	literal := l.input[start : l.pos-1]

	t, err := time.Parse(time.RFC3339Nano, literal)
	if err != nil {
		return Token{
			Type:    TokenError,
			Literal: literal,
			Value:   fmt.Sprintf("invalid timestamp: %s", literal),
		}
	}
	return Token{
		Type:    TokenTime,
		Literal: literal,
		Value:   t.UTC(),
	}
}

// readDurationToken reads a compound duration literal (e.g., 2d4h30m15s).
// Called when digits followed by a duration suffix are detected.
// digitsPart is the already-read digits string.
func (l *Lexer) readDurationToken(start int, digitsPart string) Token {
	var total time.Duration

	// Process the first group: digitsPart + current suffix
	num, _ := strconv.ParseInt(digitsPart, 10, 64)
	suffix := l.ch
	total += durationFromUnit(num, suffix)
	l.readChar()

	// Greedily read more [0-9]+[dhms] groups
	for isDigit(l.ch) {
		numStart := l.pos - 1
		for isDigit(l.ch) {
			l.readChar()
		}
		numStr := l.input[numStart : l.pos-1]
		if !isDurationSuffix(l.ch) {
			// Not a valid duration continuation - error
			literal := l.input[start : l.pos-1]
			return Token{
				Type:    TokenError,
				Literal: literal,
				Value:   fmt.Sprintf("invalid duration: %s", literal),
			}
		}
		n, _ := strconv.ParseInt(numStr, 10, 64)
		total += durationFromUnit(n, l.ch)
		l.readChar()
	}

	literal := l.input[start : l.pos-1]
	return Token{
		Type:    TokenDuration,
		Literal: literal,
		Value:   total,
	}
}

// durationFromUnit converts a numeric value and unit suffix to a time.Duration.
func durationFromUnit(n int64, unit byte) time.Duration {
	switch unit {
	case 'd':
		return time.Duration(n) * 24 * time.Hour
	case 'h':
		return time.Duration(n) * time.Hour
	case 'm':
		return time.Duration(n) * time.Minute
	case 's':
		return time.Duration(n) * time.Second
	}
	return 0
}

func (l *Lexer) readNumberToken() Token {
	start := l.pos - 1
	isNegative := false
	if l.ch == '-' {
		isNegative = true
		l.readChar()
	}

	// Read initial digit sequence to check for timestamp or duration
	digitsStart := l.pos - 1
	for isDigit(l.ch) {
		l.readChar()
	}
	digitsPart := l.input[digitsStart : l.pos-1]

	// Check for RFC 3339 timestamp: exactly 4 digits followed by '-'
	if !isNegative && len(digitsPart) == 4 && l.ch == '-' {
		l.readChar()
		return l.readTimestampToken(start)
	}

	// Check for duration literal: digits followed by duration suffix
	if len(digitsPart) > 0 && isDurationSuffix(l.ch) {
		tok := l.readDurationToken(start, digitsPart)
		if isNegative && tok.Type == TokenDuration {
			tok.Value = -tok.Value.(time.Duration)
		}
		return tok
	}

	// Continue reading IP/CIDR/Number characters
	l.readIPCIDRChars()
	literal := l.input[start : l.pos-1]

	return l.classifyNumericLiteral(literal)
}

// readIPCIDRChars consumes characters that could form IP, CIDR, or numeric literals.
func (l *Lexer) readIPCIDRChars() {
	hasColon := false
	for {
		if isDigit(l.ch) {
			l.readChar()
			continue
		}
		if l.ch == '.' && l.peekChar() != '.' {
			l.readChar()
			continue
		}
		if l.ch == ':' {
			hasColon = true
			l.readChar()
			continue
		}
		if l.ch == '/' {
			l.readChar()
			continue
		}
		if hasColon && isHexChar(l.ch) {
			l.readChar()
			continue
		}
		break
	}
}

// classifyNumericLiteral attempts to parse a literal as CIDR, IP, float, or integer.
func (l *Lexer) classifyNumericLiteral(literal string) Token {
	if looksLikeCIDR(literal) {
		if _, ipNet, err := net.ParseCIDR(literal); err == nil {
			return Token{Type: TokenCIDR, Literal: literal, Value: ipNet}
		}
	}

	if looksLikeIP(literal) {
		if ip := NormalizeIP(net.ParseIP(literal)); ip != nil {
			return Token{Type: TokenIP, Literal: literal, Value: ip}
		}
	}

	if strings.Contains(literal, ".") && !strings.Contains(literal, ":") && !strings.Contains(literal, "/") {
		if fval, err := strconv.ParseFloat(literal, 64); err == nil {
			return Token{Type: TokenFloat, Literal: literal, Value: fval}
		}
	}

	val, err := strconv.ParseInt(literal, 10, 64)
	if err != nil {
		return l.numericErrorToken(literal)
	}
	return Token{Type: TokenInt, Literal: literal, Value: val}
}

// numericErrorToken creates an error token for an invalid numeric literal.
func (l *Lexer) numericErrorToken(literal string) Token {
	isDigitsOnly := true
	startIdx := 0
	if len(literal) > 0 && literal[0] == '-' {
		startIdx = 1
	}
	for i := startIdx; i < len(literal); i++ {
		if !isDigit(literal[i]) {
			isDigitsOnly = false
			break
		}
	}
	errMsg := fmt.Sprintf("invalid number or IP: %s", literal)
	if isDigitsOnly {
		errMsg = fmt.Sprintf("integer overflow: %s", literal)
	}
	return Token{Type: TokenError, Literal: literal, Value: errMsg}
}

// Error creates a formatted error with the current lexer position.
func (l *Lexer) Error(format string, args ...any) error {
	return fmt.Errorf("lexer error at position %d: %s", l.pos, fmt.Sprintf(format, args...))
}
