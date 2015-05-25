package parser

// Note that you should include a lot of calls to panic() where something's happening that shouldn't be.
// This will help to find bugs. Once the compiler is in a better state, a lot of these calls can be removed.

import (
	"fmt"
	"os"
	"strings"
	"strconv"

	"github.com/ark-lang/ark-go/lexer"
	"github.com/ark-lang/ark-go/util"
)

type parser struct {
	file         *File
	input        []*lexer.Token
	currentToken int
	verbose      bool

	scope *Scope
}

func (v *parser) err(err string, stuff ...interface{}) {
	fmt.Printf(util.TEXT_RED+util.TEXT_BOLD+"Parser error:"+util.TEXT_RESET+" [%s:%d:%d] %s\n",
		v.peek(0).Filename, v.peek(0).LineNumber, v.peek(0).CharNumber, fmt.Sprintf(err, stuff...))
	os.Exit(2)
}

func (v *parser) peek(ahead int) *lexer.Token {
	if ahead < 0 {
		panic(fmt.Sprintf("Tried to peek a negative number: %d", ahead))
	}

	if v.currentToken+ahead >= len(v.input) {
		return nil
	}

	return v.input[v.currentToken+ahead]
}

func (v *parser) consumeToken() *lexer.Token {
	ret := v.peek(0)
	v.currentToken++
	return ret
}

func (v *parser) pushNode(node Node) {
	v.file.nodes = append(v.file.nodes, node)
}

func (v *parser) pushScope() {
	v.scope = newScope(v.scope)
}

func (v *parser) popScope() {
	v.scope = v.scope.Outer
	if v.scope == nil {
		panic("pushed too many scopes")
	}
}

func (v *parser) tokenMatches(ahead int, t lexer.TokenType, contents string) bool {
	tok := v.peek(ahead)
	return tok.Type == t && (contents == "" || (tok.Contents == contents))
}

func (v *parser) tokensMatch(args ...interface{}) bool {
	if len(args)%2 != 0 {
		panic("passed uneven args to tokensMatch")
	}

	for i := 0; i < len(args)/2; i++ {
		if !(v.tokenMatches(i, args[i*2].(lexer.TokenType), args[i*2+1].(string))) {
			return false
		}
	}
	return true
}

func Parse(tokens []*lexer.Token, verbose bool) *File {
	p := &parser{
		file: &File{
			nodes: make([]Node, 0),
		},
		input:   tokens,
		verbose: verbose,
		scope:   newGlobalScope(),
	}

	if verbose {
		fmt.Println(util.TEXT_BOLD+util.TEXT_GREEN+"Started parsing"+util.TEXT_RESET, tokens[0].Filename)
	}
	p.parse()
	if verbose {
		fmt.Println(util.TEXT_BOLD+util.TEXT_GREEN+"Finished parsing"+util.TEXT_RESET, tokens[0].Filename)
	}

	return p.file
}

func (v *parser) parse() {
	for v.peek(0) != nil {
		if n := v.parseStatement(); n != nil {
			v.pushNode(n)
		} else {
			v.consumeToken() // TODO
		}
	}
}

func (v *parser) parseStatement() Node {
	if decl := v.parseDecl(); decl != nil {
		return decl
	}
	return nil
}

func (v *parser) parseDecl() Decl {
	if variableDecl := v.parseVariableDecl(); variableDecl != nil {
		return variableDecl
	}
	return nil
}

func (v *parser) parseVariableDecl() *VariableDecl {
	variable := &Variable{}
	varDecl := &VariableDecl{
		Variable: variable,
	}

	if v.tokenMatches(0, lexer.TOKEN_IDENTIFIER, KEYWORD_MUT) {
		variable.Mutable = true
		v.consumeToken()
	}

	if v.tokensMatch(lexer.TOKEN_IDENTIFIER, "", lexer.TOKEN_OPERATOR, ":") {
		variable.Name = v.consumeToken().Contents // consume name

		v.consumeToken() // consume :

		if v.tokenMatches(0, lexer.TOKEN_IDENTIFIER, "") {
			typeName := v.consumeToken().Contents // consume type

			variable.Type = v.scope.GetType(typeName)
			if variable.Type == nil {
				v.err("Unrecognized type `%s`", typeName)
			}
		} else if v.tokenMatches(0, lexer.TOKEN_OPERATOR, "=") {
			panic("type inference unimplemented")
		}

		if v.tokenMatches(0, lexer.TOKEN_OPERATOR, "=") {
			v.consumeToken() // consume =
			varDecl.Assignment = v.parseExpr()
			if varDecl.Assignment == nil {
				v.err("Expected expression in assignment to variable `%s`", variable.Name)
			}

		} else if v.tokenMatches(0, lexer.TOKEN_SEPARATOR, ";") {
			v.consumeToken()
		} else {
			v.err("Missing semicolon at end of variable declaration")
		}
	} else {
		return nil
	}

	if sname := v.scope.InsertVariable(variable); sname != nil {
		v.err("Illegal redeclaration of variable `%s`", variable.Name)
	}
	return varDecl
}

func (v *parser) parseExpr() Expr {
	if litExpr := v.parseLiteral(); litExpr != nil {
		return litExpr
	}
	return nil
}

func (v *parser) parseLiteral() Expr {
	if numLit := v.parseNumericLiteral(); numLit != nil {
		return numLit
	} else if stringLit := v.parseStringLiteral(); stringLit != nil {
		return stringLit
	} else if runeLit := v.parseRuneLiteral(); runeLit != nil {
		return runeLit
	}
	return nil
}

func (v *parser) parseNumericLiteral() Expr {
	if !v.tokenMatches(0, lexer.TOKEN_NUMBER, "") {
		return nil
	}
	
	num := v.consumeToken().Contents
	var err error
	
	if strings.HasPrefix(num, "0x") || strings.HasPrefix(num, "0X") {
		// Hexadecimal integer
		hex := &IntegerLiteral {}
		hex.value, err = strconv.ParseUint(num[2:], 16, 64)
		if err != nil {
			panic("bad hex got through lexer")
		}
		return hex
	} else if strings.HasPrefix(num, "0b") {
		// Binary integer
		bin := &IntegerLiteral {}
		bin.value, err = strconv.ParseUint(num[2:], 2, 64)
		if err != nil {
			panic("bad binary got through lexer")
		}
		return bin
	} else if strings.HasPrefix(num, "0o") {
		// Octal integer
		oct := &IntegerLiteral {}
		oct.value, err = strconv.ParseUint(num[2:], 8, 64)
		if err != nil {
			panic("bad octal got through lexer")
		}
		return oct
	} else if strings.ContainsRune(num, '.') || strings.HasSuffix(num, "f") || strings.HasSuffix(num, "d") {
		if strings.Count(num, ".") > 1 {
			v.err("Floating-point cannot have multiple periods: `%s`", num)
			return nil
		}
		// TODO parse float
		return nil
	} else {
		// Decimal integer
		i := &IntegerLiteral {}
		i.value, err = strconv.ParseUint(num, 10, 64)
		if err != nil {
			panic("bad decimal got through lexer")
		}
		return i
	}
}

func (v *parser) parseStringLiteral() *StringLiteral {
	return nil
}

func (v *parser) parseRuneLiteral() *RuneLiteral {
	return nil
}
