package kevs

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

type ValueKind uint8

const (
	ValueKindUndefined ValueKind = iota
	ValueKindString
	ValueKindInteger
	ValueKindBoolean
	ValueKindList
	ValueKindTable
)

func (self ValueKind) String() string {
	switch self {
	case ValueKindUndefined:
		return "undefined"
	case ValueKindString:
		return "string"
	case ValueKindInteger:
		return "integer"
	case ValueKindBoolean:
		return "boolean"
	case ValueKindList:
		return "list"
	case ValueKindTable:
		return "table"
	default:
		return "unknown"
	}
}

type List []Value

type ValueData struct {
	List    List
	Table   Table
	String  string
	Integer int64
	Boolean bool
}

type Value struct {
	Kind ValueKind
	Data ValueData
}

type KeyValue struct {
	Key   string
	Value Value
}

type Table []KeyValue

func Parse(file, content string, flags Flags) (Table, error) {
	tokens, err := Scan(file, content, flags)
	if err != nil {
		return nil, err
	}
	return ParseTokens(file, content, flags, tokens)
}

type TokenKind uint8

const (
	TokenKindUndefined TokenKind = iota
	TokenKindKey
	TokenKindDelim
	TokenKindValue
)

func (self TokenKind) String() string {
	switch self {
	case TokenKindUndefined:
		return "undefined"
	case TokenKindKey:
		return "key"
	case TokenKindDelim:
		return "delim"
	case TokenKindValue:
		return "value"
	default:
		return "unknown"
	}
}

type Token struct {
	Value string
	Kind  TokenKind
	Line  int
}

type Flags struct {
	AbortOnError bool
}

type params struct {
	file    string
	content string
	flags   Flags
}

type scanner struct {
	params params
	tokens []Token
	line   int
	err    error
}

const (
	kKeyValSep      = '='
	kKeyValEnd      = ';'
	kCommentBegin   = '#'
	kStringBegin    = '"'
	kRawStringBegin = '`'
	kListBegin      = '['
	kListEnd        = ']'
	kTableBegin     = '{'
	kTableEnd       = '}'

	spaces = " \t"
)

func Scan(file, content string, flags Flags) ([]Token, error) {
	s := scanner{
		params: params{
			file:    file,
			content: content,
			flags:   flags,
		},
		line: 1,
	}

	for len(s.params.content) != 0 {
		s.trim_space()
		ok := false
		switch {
		case s.expect('\n'):
			ok = s.scan_newline()
		case s.expect(kCommentBegin):
			ok = s.scan_comment()
		default:
			ok = s.scan_key_value()
		}
		if !ok {
			return nil, s.err
		}
	}

	return s.tokens, nil
}

func (self *scanner) trim_space() {
	self.params.content = strings.TrimLeft(self.params.content, spaces)
}

func (self *scanner) expect(c byte) bool {
	if len(self.params.content) == 0 {
		return false
	}
	return self.params.content[0] == c
}

func (self *scanner) scan_newline() bool {
	self.line++
	self.advance(1)
	return true
}

func (self *scanner) advance(n int) {
	self.params.content = self.params.content[n:]
}

func (self *scanner) scan_comment() bool {
	newline := strings.IndexByte(self.params.content, '\n')
	if newline == -1 {
		self.errorf("comment does not end with newline")
		return false
	}
	self.advance(newline)
	return true
}

func (self *scanner) scan_key_value() bool {
	if !self.scan_key() {
		return false
	}
	// separator check done in scan_key, no need to check again
	self.append_delim()
	return self.scan_value()
}

func (self *scanner) errorf(format string, args ...any) {
	self.err = fmt.Errorf("%s:%d: error: scan: %s", self.params.file, self.line, fmt.Sprintf(format, args...))

	if self.params.flags.AbortOnError {
		panic(self.err)
	}
}

func indexAny(s, chars string) (rune, int) {
	for i, c := range s {
		if strings.ContainsRune(chars, c) {
			return c, i
		}
	}
	return 0, -1
}

func (self *scanner) scan_key() bool {
	c, i := indexAny(self.params.content, "=;\n")
	if c != kKeyValSep {
		self.errorf("key-value pair is missing separator")
		return false
	}
	self.append(TokenKindKey, i)
	if len(self.tokens[len(self.tokens)-1].Value) == 0 {
		self.errorf("empty key")
		return false
	}
	return true
}

func (self *scanner) scan_value() bool {
	self.trim_space()
	ok := false
	switch {
	case self.expect(kListBegin):
		ok = self.scan_list_value()
	case self.expect(kTableBegin):
		ok = self.scan_table_value()
	case self.expect(kStringBegin):
		ok = self.scan_string_value()
	case self.expect(kRawStringBegin):
		ok = self.scan_raw_string()
	default:
		ok = self.scan_int_or_bool_value()
	}
	if !ok {
		return false
	}
	if !self.scan_delim(kKeyValEnd) {
		self.errorf("value does not end with semicolon")
		return false
	}
	return true
}

func (self *scanner) scan_delim(c byte) bool {
	if !self.expect(c) {
		return false
	}
	self.append_delim()
	return true
}

func (self *scanner) scan_string_value() bool {
	// advance past leading quote
	end := 1
	s := self.params.content

	for {
		// search for trailing quote
		i := strings.IndexByte(s[end:], kStringBegin)
		if i == -1 {
			self.errorf("string value does not end with quote")
			return false
		}

		// advance
		end += i + 1

		// stop if quote is not escaped
		if prev := s[end-2]; prev != '\\' {
			break
		}
	}

	self.append(TokenKindValue, end)

	return true
}

func (self *scanner) scan_raw_string() bool {
	end := strings.IndexByte(self.params.content[1:], kRawStringBegin)
	if end == -1 {
		self.errorf("raw string value does not end with backtick")
		return false
	}

	// +2 for leading and trailing quotes
	self.append(TokenKindValue, end+2)

	// count newlines in raw string to keep line count accurate
	self.line += strings.Count(self.tokens[len(self.tokens)-1].Value, "\n")

	return true
}

func (self *scanner) scan_int_or_bool_value() bool {
	// search for all possible value endings
	// if semicolon(or none of them) is not found => error
	c, end := indexAny(self.params.content, ";]}\n")
	if end == -1 || c != kKeyValEnd {
		self.errorf("integer or boolean value does not end with semicolon")
		return false
	}
	self.append(TokenKindValue, end)
	return true
}

func (self *scanner) scan_list_value() bool {
	self.append_delim()
	for {
		self.trim_space()
		if len(self.params.content) == 0 {
			self.errorf("end of input without list end")
			return false
		}
		if self.expect('\n') {
			if !self.scan_newline() {
				return false
			}
			continue
		}
		if self.expect(kCommentBegin) {
			if !self.scan_comment() {
				return false
			}
			continue
		}
		if self.expect(kListEnd) {
			self.append_delim()
			return true
		}
		if !self.scan_value() {
			return false
		}
		if self.expect(kListEnd) {
			self.append_delim()
			return true
		}
	}
}

func (self *scanner) scan_table_value() bool {
	self.append_delim()
	for {
		self.trim_space()
		if len(self.params.content) == 0 {
			self.errorf("end of input without table end")
			return false
		}
		if self.expect('\n') {
			if !self.scan_newline() {
				return false
			}
			continue
		}
		if self.expect(kCommentBegin) {
			if !self.scan_comment() {
				return false
			}
			continue
		}
		if self.expect(kTableEnd) {
			self.append_delim()
			return true
		}
		if !self.scan_key_value() {
			return false
		}
		if self.expect(kTableEnd) {
			self.append_delim()
			return true
		}
	}
}

func (self *scanner) append_delim() {
	self.tokens = append(self.tokens, Token{
		Kind:  TokenKindDelim,
		Value: self.params.content[0:1],
		Line:  self.line,
	})
	self.advance(1)
}

func (self *scanner) append(kind TokenKind, end int) {
	val := self.params.content[:end]
	val = strings.TrimRight(val, spaces)

	self.tokens = append(self.tokens, Token{
		Kind:  kind,
		Value: val,
		Line:  self.line,
	})

	self.advance(end)
}

type parser struct {
	params params
	tokens []Token
	table  Table
	i      int
	err    error
}

func ParseTokens(file, content string, flags Flags, tokens []Token) (Table, error) {
	p := parser{
		params: params{
			file:    file,
			content: content,
			flags:   flags,
		},
		tokens: tokens,
	}

	for p.i < len(tokens) {
		kv, ok := p.parse_key_value(p.table)
		if !ok {
			return nil, p.err
		}
		p.table = append(p.table, *kv)
	}

	return p.table, nil
}

func (self *parser) parse_key_value(parent Table) (*KeyValue, bool) {
	key, ok := self.parse_key(parent)
	if !ok {
		return nil, false
	}

	if !self.parse_delim(kKeyValSep) {
		self.errorf("missing key value separator")
		return nil, false
	}

	val, ok := self.parse_value()
	if !ok {
		return nil, false
	}

	out := &KeyValue{
		Key:   key,
		Value: *val,
	}

	return out, true
}

func (self *parser) parse_key(parent Table) (string, bool) {
	if !self.expect(TokenKindKey) {
		self.errorf("expected key token")
		return "", false
	}

	tok := self.get()

	if !is_identifier(tok.Value) {
		self.errorf("key is not a valid identifier: '%s'", tok.Value)
		return "", false
	}

	// check if key is unique
	for _, kv := range parent {
		if kv.Key == tok.Value {
			self.errorf("key '%s' is not unique for current table", tok.Value)
			return "", false
		}
	}

	key := tok.Value

	self.pop()

	return key, true
}

func (self *parser) parse_value() (*Value, bool) {
	var ok bool
	var out *Value

	switch {
	case self.expect_delim(kListBegin):
		out, ok = self.parse_list_value()
	case self.expect_delim(kTableBegin):
		out, ok = self.parse_table_value()
	default:
		out, ok = self.parse_simple_value()
	}
	if !ok {
		return nil, false
	}

	if !self.parse_delim(kKeyValEnd) {
		self.errorf("missing key value end")
		return nil, false
	}

	return out, true
}

func (self *parser) parse_list_value() (*Value, bool) {
	out := &Value{
		Kind: ValueKindList,
	}

	self.pop()

	for {
		if self.parse_delim(kListEnd) {
			return out, true
		}

		v, ok := self.parse_value()
		if !ok {
			return nil, false
		}
		out.Data.List = append(out.Data.List, *v)

		if self.parse_delim(kListEnd) {
			return out, true
		}
	}
}

func (self *parser) parse_table_value() (*Value, bool) {
	out := &Value{
		Kind: ValueKindTable,
	}

	self.pop()

	for {
		if self.parse_delim(kTableEnd) {
			return out, true
		}

		kv, ok := self.parse_key_value(out.Data.Table)
		if !ok {
			return nil, false
		}
		out.Data.Table = append(out.Data.Table, *kv)

		if self.parse_delim(kTableEnd) {
			return out, true
		}
	}
}

func (self *parser) parse_simple_value() (*Value, bool) {
	if !self.expect(TokenKindValue) {
		self.errorf("expected value token")
		return nil, false
	}

	val := self.get().Value

	ok := true
	out := &Value{}

	switch {
	case val[0] == kStringBegin:
		data, err := normString(val[1 : len(val)-1])
		if err != nil {
			self.errorf("could not normalize string: %s", err)
			return nil, false
		}
		out.Kind = ValueKindString
		out.Data.String = data

	case val[0] == kRawStringBegin:
		out.Kind = ValueKindString
		out.Data.String = val[1 : len(val)-1]

	case val == "true":
		out.Kind = ValueKindBoolean
		out.Data.Boolean = true

	case val == "false":
		out.Kind = ValueKindBoolean
		out.Data.Boolean = false

	default:
		i, err := str_to_int(val, 0)
		if err != nil {
			self.errorf("value '%s' is not an integer: %s", val, err)
			ok = false
		} else {
			out.Kind = ValueKindInteger
			out.Data.Integer = i
		}
	}

	self.pop()

	return out, ok
}

func normString(s string) (string, error) {
	dst := strings.Builder{}

	for i := 0; i < len(s); {
		if s[i] == '\\' {
			i++
			switch s[i] {
			case 'a':
				dst.WriteByte('\a')
				i++
			case 'b':
				dst.WriteByte('\b')
				i++
			case 'f':
				dst.WriteByte('\f')
				i++
			case 'n':
				dst.WriteByte('\n')
				i++
			case 'r':
				dst.WriteByte('\r')
				i++
			case 't':
				dst.WriteByte('\t')
				i++
			case 'v':
				dst.WriteByte('\v')
				i++
			case '"':
				dst.WriteByte('"')
				i++
			case '\\':
				dst.WriteByte('\\')
				i++

			case 'u':
				i++

				if (i + 4) > len(s) {
					return "", fmt.Errorf("\\u must be followed by 4 hex digits: \\uXXXX")
				}

				code, err := str_to_uint(s[i:i+4], 16)
				if err != nil {
					return "", err
				}
				i += 4

				utf8 := ucs_to_utf8(code)
				if utf8 == nil {
					return "", fmt.Errorf("could not encode Unicode code point to UTF-8")
				}
				dst.Write(utf8)

			case 'U':
				i++

				if (i + 8) > len(s) {
					return "", fmt.Errorf("\\U must be followed by 8 hex digits: \\UXXXXXXXX")
				}

				code, err := str_to_uint(s[i:i+8], 16)
				if err != nil {
					return "", err
				}
				i += 8

				utf8 := ucs_to_utf8(code)
				if utf8 == nil {
					return "", fmt.Errorf("could not encode Unicode code point to UTF-8")
				}
				dst.Write(utf8)

			default:
				return "", fmt.Errorf("unknown escape sequence")

			}
		} else {
			dst.WriteByte(s[i])
			i++
		}
	}

	return dst.String(), nil
}

// Convert UCS code point to UTF-8
func ucs_to_utf8(code uint64) []byte {
	// Code points in the surrogate range are not valid for UTF-8.
	if 0xd800 <= code && code <= 0xdfff {
		return nil
	}

	var out []byte

	// 0x00000000 - 0x0000007F:
	// 0xxxxxxx
	if code <= 0x0000007F {
		out = append(out, byte(code))
		return out
	}

	// 0x00000080 - 0x000007FF:
	// 110xxxxx 10xxxxxx
	if code <= 0x000007FF {
		out = append(out, byte(0xc0|(code>>6)))
		out = append(out, byte(0x80|(code&0x3f)))
		return out
	}

	// 0x00000800 - 0x0000FFFF:
	// 1110xxxx 10xxxxxx 10xxxxxx
	if code <= 0x0000FFFF {
		out = append(out, byte(0xe0|(code>>12)))
		out = append(out, byte(0x80|((code>>6)&0x3f)))
		out = append(out, byte(0x80|(code&0x3f)))
		return out
	}

	// 0x00010000 - 0x0010FFFF:
	// 11110xxx 10xxxxxx 10xxxxxx 10xxxxxx
	if code <= 0x0010FFFF {
		out = append(out, byte(0xf0|(code>>18)))
		out = append(out, byte(0x80|((code>>12)&0x3f)))
		out = append(out, byte(0x80|((code>>6)&0x3f)))
		out = append(out, byte(0x80|(code&0x3f)))
		return out
	}

	return nil
}

func (self *parser) parse_delim(c byte) bool {
	if !self.expect_delim(c) {
		return false
	}
	self.pop()
	return true
}

func (self parser) expect_delim(delim byte) bool {
	if !self.expect(TokenKindDelim) {
		return false
	}
	return self.get().Value == string(delim)
}

func (self *parser) errorf(format string, args ...any) {
	self.err = fmt.Errorf("%s:%d: error: parse: %s", self.params.file, self.get().Line, fmt.Sprintf(format, args...))

	if self.params.flags.AbortOnError {
		panic(self.err)
	}
}

func is_digit(c byte) bool { return c >= '0' && c <= '9' }

func lower(c byte) byte { return (c | ('x' - 'X')) }

func is_letter(c byte) bool {
	return lower(c) >= 'a' && lower(c) <= 'z'
}

func is_identifier(s string) bool {
	c := s[0]
	if c != '_' && !is_letter(c) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !is_digit(s[i]) && !is_letter(s[i]) && s[i] != '_' {
			return false
		}
	}
	return true
}

func (self parser) get() Token {
	return self.tokens[self.i]
}

func (self *parser) pop() { self.i++ }

func (self parser) expect(kind TokenKind) bool {
	if self.i >= len(self.tokens) {
		self.errorf("expected token '%s', have nothing", kind)
		return false
	}
	return self.get().Kind == kind
}

func (self Table) Dump() {
	for _, kv := range self {
		switch kv.Value.Kind {
		case ValueKindTable:
			fmt.Printf("%s %s\n", kv.Key, kv.Value.Kind)
			kv.Value.Data.Table.Dump()

		case ValueKindList:
			fmt.Printf("%s %s\n", kv.Key, kv.Value.Kind)
			kv.Value.Data.List.Dump()

		case ValueKindString:
			fmt.Printf("%s %s '%s'\n", kv.Key, kv.Value.Kind, kv.Value.Data.String)

		case ValueKindBoolean:
			fmt.Printf("%s %s %v\n", kv.Key, kv.Value.Kind, kv.Value.Data.Boolean)

		case ValueKindInteger:
			fmt.Printf("%s %s %d\n", kv.Key, kv.Value.Kind, kv.Value.Data.Integer)

		default:
			fmt.Printf("%s %s\n", kv.Key, kv.Value.Kind)

		}
	}
}

func (self List) Dump() {
	for _, v := range self {
		switch v.Kind {
		case ValueKindTable:
			fmt.Printf("%s\n", v.Kind)
			v.Data.Table.Dump()

		case ValueKindList:
			fmt.Printf("%s\n", v.Kind)
			v.Data.List.Dump()

		case ValueKindString:
			fmt.Printf("%s '%s'\n", v.Kind, v.Data.String)

		case ValueKindBoolean:
			fmt.Printf("%s %v\n", v.Kind, v.Data.Boolean)

		case ValueKindInteger:
			fmt.Printf("%s %d\n", v.Kind, v.Data.Integer)

		default:
			fmt.Printf("%s\n", v.Kind)

		}
	}
}

func str_to_uint(s string, base uint64) (uint64, error) {
	if len(s) == 0 {
		return 0, fmt.Errorf("empty input")
	}

	if base == 0 {
		if s[0] == '0' {
			// stop if 0
			if len(s) == 1 {
				return 0, nil
			}
			if len(s) < 3 {
				return 0, fmt.Errorf("leading 0 requires at least 2 more chars")
			}
			switch s[1] {
			case 'x':
				base = 16
				s = s[2:]
			case 'o':
				base = 8
				s = s[2:]
			case 'b':
				base = 2
				s = s[2:]
			default:
				return 0, fmt.Errorf("invalid base char, must be 'x', 'o' or 'b'")
			}
		} else {
			base = 10
		}
	} else {
		if base != 2 && base != 8 && base != 16 {
			return 0, fmt.Errorf("invalid base")
		}
	}

	const max = 1<<64 - 1

	// cutoff is the smallest number such that cutoff*base > max.
	cutoff := max/base + 1

	n := uint64(0)
	for i := 0; i < len(s); i++ {
		c := s[i]

		d := uint64(0)
		switch {
		case is_digit(c):
			d = uint64(c - '0')
		case is_letter(c):
			d = uint64(lower(c) - 'a' + 10)
		default:
			return 0, fmt.Errorf("invalid char, must be a letter or a digit")
		}

		if d >= base {
			return 0, fmt.Errorf("invalid digit, bigger than base")
		}

		if n >= cutoff {
			return 0, fmt.Errorf("invalid input, mul overflows")
		}
		n *= base

		n1 := n + d
		if n1 < n || n1 > max {
			return 0, fmt.Errorf("invalid input, add overflows")
		}
		n = n1
	}

	return n, nil
}

func str_to_int(s string, base uint64) (int64, error) {
	if len(s) == 0 {
		return 0, fmt.Errorf("empty input")
	}

	neg := false
	if s[0] == '+' {
		s = s[1:]
	} else if s[0] == '-' {
		neg = true
		s = s[1:]
	}

	un, err := str_to_uint(s, base)
	if err != nil {
		return 0, err
	}

	max := uint64(1) << 63

	if !neg && un >= max {
		return 0, fmt.Errorf("invalid input, overflows max value")
	}
	if neg && un > max {
		return 0, fmt.Errorf("invalid input, underflows min value")
	}

	//nolint overflow checked above
	n := int64(un)
	if neg && n >= 0 {
		n = -n
	}

	return n, nil
}

func (self Table) get(key string) (*Value, error) {
	for _, kv := range self {
		if kv.Key == key {
			return &kv.Value, nil
		}
	}
	return nil, errors.New("key not found")
}

func (self Table) GetString(key string) (string, error) {
	val, err := self.get(key)
	if err != nil {
		return "", err
	}
	if val.Kind != ValueKindString {
		return "", errors.New("value is not string")
	}
	return val.Data.String, nil
}

func (self Table) GetInteger(key string) (int64, error) {
	val, err := self.get(key)
	if err != nil {
		return 0, err
	}
	if val.Kind != ValueKindInteger {
		return 0, errors.New("value is not integer")
	}
	return val.Data.Integer, nil
}

func (self Table) GetBoolean(key string) (bool, error) {
	val, err := self.get(key)
	if err != nil {
		return false, err
	}
	if val.Kind != ValueKindBoolean {
		return false, errors.New("value is not boolean")
	}
	return val.Data.Boolean, nil
}

func (self Table) GetTable(key string) (Table, error) {
	val, err := self.get(key)
	if err != nil {
		return nil, err
	}
	if val.Kind != ValueKindTable {
		return nil, errors.New("value is not table")
	}
	return val.Data.Table, nil
}

func (self Table) GetList(key string) (List, error) {
	val, err := self.get(key)
	if err != nil {
		return nil, err
	}
	if val.Kind != ValueKindList {
		return nil, errors.New("value is not list")
	}
	return val.Data.List, nil
}

const (
	reflectTag = "kevs"
)

func (self Table) Unmarshal(dst any) error {
	val := reflect.ValueOf(dst)
	if val.Type().Kind() != reflect.Pointer || val.Elem().Kind() != reflect.Struct {
		return errors.New("destination must be a pointer to a struct")
	}
	v := reflect.Indirect(val)
	if !v.CanAddr() {
		return errors.New("destination cannot be addressed")
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, found := f.Tag.Lookup(reflectTag)
		if !found {
			continue
		}
		switch f.Type.Kind() {
		case reflect.String:
			vv, err := self.GetString(name)
			if err != nil {
				return fmt.Errorf("struct '%s': field '%s': %w", t.Name(), f.Name, err)
			}
			v.Field(i).SetString(vv)
		case reflect.Int:
			vv, err := self.GetInteger(name)
			if err != nil {
				return fmt.Errorf("struct '%s': field '%s': %w", t.Name(), f.Name, err)
			}
			v.Field(i).SetInt(vv)
		case reflect.Bool:
			vv, err := self.GetBoolean(name)
			if err != nil {
				return fmt.Errorf("struct '%s': field '%s': %w", t.Name(), f.Name, err)
			}
			v.Field(i).SetBool(vv)
		case reflect.Slice, reflect.Array:
			vv, err := self.GetList(name)
			if err != nil {
				return fmt.Errorf("struct '%s': field '%s': %w", t.Name(), f.Name, err)
			}
			if err := vv.unmarshal(v.Field(i)); err != nil {
				return err
			}
		case reflect.Struct:
			vv, err := self.GetTable(name)
			if err != nil {
				return fmt.Errorf("struct '%s': field '%s': %w", t.Name(), f.Name, err)
			}
			if err := vv.Unmarshal(v.Field(i).Addr().Interface()); err != nil {
				return err
			}
		default:
			return fmt.Errorf("struct '%s': field '%s': type must be one of: %s, %s, %s", t.Name(), f.Name, reflect.String, reflect.Int, reflect.Bool)
		}
	}
	return nil
}

func (self List) unmarshal(v reflect.Value) error {
	slice := reflect.MakeSlice(v.Type(), len(self), len(self))
	for i, item := range self {
		elem := slice.Index(i)
		switch {
		case item.Kind == ValueKindString && elem.Kind() == reflect.String:
			elem.SetString(item.Data.String)
		case item.Kind == ValueKindInteger && elem.Kind() == reflect.Int:
			elem.SetInt(item.Data.Integer)
		case item.Kind == ValueKindBoolean && elem.Kind() == reflect.Bool:
			elem.SetBool(item.Data.Boolean)
		case item.Kind == ValueKindTable && elem.Kind() == reflect.Struct:
			err := item.Data.Table.Unmarshal(elem.Addr().Interface())
			if err != nil {
				return err
			}
		}
	}
	v.Set(slice)
	return nil
}
