package cjs

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/js"
)

func RewriteRequires(path, prefix, source string) (string, error) {
	// Extract shebang if present
	shebang, codeWithoutShebang := extractShebang(source)

	// Parse the JavaScript (without shebang)
	ast, err := js.Parse(parse.NewInputString(codeWithoutShebang), js.Options{})
	if err != nil {
		return "", fmt.Errorf("cjs: failed to parse %s: %w", path, err)
	}

	// Extract directive prologues (like "use strict") and get code without them
	directives, codeWithoutDirectives := extractDirectivesString(ast, codeWithoutShebang)

	// Find all require-like calls and collect paths
	visitor := &requireVisitor{
		prefix:       prefix,
		requires:     make(map[string]bool),
		requireCalls: []requireCall{},
		pathOrder:    []string{},
	}
	js.Walk(visitor, ast)

	// If no requires found, return original source
	if len(visitor.requires) == 0 {
		return source, nil
	}

	// Use the paths in the order they were discovered
	paths := visitor.pathOrder

	// Generate import statements and object mapping
	var imports strings.Builder
	var objMapping strings.Builder

	for i, reqPath := range paths {
		importName := pathToImportName(reqPath)

		// Import statement
		fmt.Fprintf(&imports, "import %s from %q\n", importName, reqPath)

		// Object mapping
		if i > 0 {
			objMapping.WriteString(",\n\t")
		}
		fmt.Fprintf(&objMapping, "%q: %s", reqPath, importName)
	}

	// Generate the require infrastructure
	infrastructure := fmt.Sprintf(`%sconst __cjs_imports__ = {
	%s,
}
function __cjs_require__(path) {
	const req = __cjs_imports__[path]
	if (!req) {
		throw new Error("Module not found: " + path)
	}
	return req
}
`, imports.String(), objMapping.String())

	// Replace the require function calls with __cjs_require__ (use code without directives to avoid duplication)
	replaced := replaceRequireCalls(codeWithoutDirectives, visitor.requireCalls, prefix)

	// Combine: shebang + directives + infrastructure + modified code
	return shebang + directives + infrastructure + replaced, nil
}

type requireCall struct {
	funcName string
	path     string
}

type requireVisitor struct {
	prefix       string
	requires     map[string]bool
	requireCalls []requireCall
	pathOrder    []string // Preserve order of first occurrence
}

func (v *requireVisitor) Enter(n js.INode) js.IVisitor {
	// Look for any CallExpr with 1 string argument starting with prefix
	if call, ok := n.(*js.CallExpr); ok {
		// Must have exactly 1 argument
		if len(call.Args.List) == 1 {
			// Argument must be a string literal
			if lit, ok := call.Args.List[0].Value.(*js.LiteralExpr); ok {
				pathStr := extractStringLiteral(lit)
				// Only collect paths that start with prefix
				if strings.HasPrefix(pathStr, v.prefix) {
					// Track first occurrence order
					if !v.requires[pathStr] {
						v.pathOrder = append(v.pathOrder, pathStr)
					}
					v.requires[pathStr] = true

					// Track the function name for replacement
					if funcName := v.getFunctionName(call); funcName != "" {
						v.requireCalls = append(v.requireCalls, requireCall{
							funcName: funcName,
							path:     pathStr,
						})
					}
				}
			}
		}
	}
	return v
}

func (v *requireVisitor) Exit(n js.INode) {}

func (v *requireVisitor) getFunctionName(call *js.CallExpr) string {
	if ident, ok := call.X.(*js.Var); ok {
		return string(ident.Data)
	}
	return ""
}

// pathToImportName converts a path like "/node_modules/react" to "__cjs_import_react__"
func pathToImportName(path string) string {
	// Get the last segment of the path
	segments := strings.Split(path, "/")
	var lastName string
	for i := len(segments) - 1; i >= 0; i-- {
		if segments[i] != "" {
			lastName = segments[i]
			break
		}
	}

	if lastName == "" {
		lastName = "module"
	}

	// Replace special characters with underscores
	reg := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	lastName = reg.ReplaceAllString(lastName, "_")

	// Ensure it doesn't start with a number
	if len(lastName) > 0 && lastName[0] >= '0' && lastName[0] <= '9' {
		lastName = "_" + lastName
	}

	return "__cjs_import_" + lastName + "__"
}

// replaceRequireCalls replaces require function calls with __cjs_require__
func replaceRequireCalls(source string, calls []requireCall, prefix string) string {
	// Build patterns for each require call we found
	// Replace funcName("path") with __cjs_require__("path")
	result := source

	// Group calls by function name to build regex patterns
	funcToPaths := make(map[string][]string)
	for _, call := range calls {
		funcToPaths[call.funcName] = append(funcToPaths[call.funcName], call.path)
	}

	// For each function name, replace its calls
	for funcName := range funcToPaths {
		// Use regex to match function calls: funcName("...")
		// We need to escape special regex characters in the function name
		escapedFunc := regexp.QuoteMeta(funcName)
		pattern := escapedFunc + `\s*\(\s*(["\'])` + regexp.QuoteMeta(prefix)
		re := regexp.MustCompile(pattern)

		// Replace with __cjs_require__(
		result = re.ReplaceAllStringFunc(result, func(match string) string {
			// Extract the quote character
			re2 := regexp.MustCompile(escapedFunc + `\s*\(\s*(["\'])`)
			quoteMatch := re2.FindStringSubmatch(match)
			if len(quoteMatch) > 1 {
				return "__cjs_require__(" + quoteMatch[1] + prefix
			}
			return "__cjs_require__(\"" + prefix
		})
	}

	return result
}

// extractStringLiteral extracts the string value from a literal expression
func extractStringLiteral(lit *js.LiteralExpr) string {
	data := string(lit.Data)
	// Remove quotes
	if len(data) >= 2 {
		if (data[0] == '"' && data[len(data)-1] == '"') ||
			(data[0] == '\'' && data[len(data)-1] == '\'') {
			return data[1 : len(data)-1]
		}
	}
	return data
}

// extractDirectivesString extracts directive prologues from the source
// Returns the directive strings and the source without directives
func extractDirectivesString(ast *js.AST, source string) (string, string) {
	var directives strings.Builder
	directiveCount := 0

	// Count directive prologue statements in AST
	for i := 0; i < len(ast.BlockStmt.List); i++ {
		stmt := ast.BlockStmt.List[i]

		// Check if this is a directive prologue statement
		if _, ok := stmt.(*js.DirectivePrologueStmt); ok {
			directiveCount++
		} else if _, ok := stmt.(*js.Comment); ok {
			// Skip comments - continue looking for directives
			continue
		} else {
			break // Stop at first non-comment, non-directive statement
		}
	}

	// If no directives, return as-is
	if directiveCount == 0 {
		return "", source
	}

	// Extract directive strings from source
	pos := 0
	foundDirectives := 0

	for foundDirectives < directiveCount && pos < len(source) {
		// Skip whitespace
		for pos < len(source) && (source[pos] == ' ' || source[pos] == '\t' || source[pos] == '\n' || source[pos] == '\r') {
			pos++
		}

		if pos >= len(source) {
			break
		}

		// Look for string literal (directive)
		if source[pos] == '"' || source[pos] == '\'' {
			quote := source[pos]
			start := pos
			pos++

			// Find closing quote
			for pos < len(source) && source[pos] != quote {
				if source[pos] == '\\' {
					pos++ // Skip escaped character
				}
				pos++
			}
			pos++ // Skip closing quote

			// Skip whitespace
			for pos < len(source) && (source[pos] == ' ' || source[pos] == '\t') {
				pos++
			}

			// Expect semicolon
			if pos < len(source) && source[pos] == ';' {
				pos++
				directives.WriteString(source[start:pos])
				directives.WriteString("\n")
				foundDirectives++
			}
		} else {
			break // Not a directive
		}
	}

	// Skip any trailing whitespace/newlines after directives
	for pos < len(source) && (source[pos] == ' ' || source[pos] == '\t' || source[pos] == '\n' || source[pos] == '\r') {
		pos++
	}

	return directives.String(), source[pos:]
}
