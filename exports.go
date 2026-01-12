package cjs

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/js"
)

func ParseExports(path, code string) ([]string, error) {
	_, code = extractShebang(code)
	ast, err := js.Parse(parse.NewInputString(string(code)), js.Options{})
	if err != nil {
		return nil, fmt.Errorf("cjs: failed to parse %s: %w", path, err)
	}

	visitor := &exportVisitor{
		exports:          make(map[string]bool),
		hasDefaultExport: false,
		unsafeGetters:    make(map[string]bool),
	}

	js.Walk(visitor, ast)

	// Check for errors during traversal
	if visitor.err != nil {
		return nil, visitor.err
	}

	// Remove any exports that were marked as unsafe getters
	for name := range visitor.unsafeGetters {
		delete(visitor.exports, name)
	}

	// Convert map to slice
	exports := make([]string, 0, len(visitor.exports))
	for name := range visitor.exports {
		exports = append(exports, name)
	}

	// Add default export if present
	if visitor.hasDefaultExport {
		exports = append(exports, "default")
	}

	sort.Strings(exports)
	return exports, nil
}

type exportVisitor struct {
	err              error
	exports          map[string]bool
	unsafeGetters    map[string]bool
	hasDefaultExport bool
}

func (r *exportVisitor) Exit(n js.INode) {}

func (v *exportVisitor) Enter(n js.INode) js.IVisitor {
	// Handle BinaryExpr (assignments)
	if bin, ok := n.(*js.BinaryExpr); ok {
		if bin.Op == js.EqToken {
			v.handleAssignment(bin.X, bin.Y)
		}
	}

	// Handle CallExpr (Object.defineProperty, etc.)
	if call, ok := n.(*js.CallExpr); ok {
		v.handleCallExpr(call)
	}

	return v
}

func (v *exportVisitor) handleAssignment(left, right js.IExpr) {
	// Check for exports.foo = ... or module.exports.foo = ...
	if dot, ok := left.(*js.DotExpr); ok {
		if v.isExportsIdent(dot.X) {
			// exports.foo = ...
			// Property name can be either *js.Var or js.LiteralExpr (no pointer)
			if ident, ok := dot.Y.(*js.Var); ok {
				v.exports[string(ident.Data)] = true
			} else if lit, ok := dot.Y.(js.LiteralExpr); ok {
				v.exports[string(lit.Data)] = true
			}
		} else if v.isModuleExports(dot.X) {
			// module.exports.foo = ...
			if ident, ok := dot.Y.(*js.Var); ok {
				v.exports[string(ident.Data)] = true
			} else if lit, ok := dot.Y.(js.LiteralExpr); ok {
				v.exports[string(lit.Data)] = true
			}
		} else if v.isModuleIdent(dot.X) && v.isExportsField(dot.Y) {
			// module.exports = ...
			v.hasDefaultExport = true
			// Check if it's an object literal
			if obj, ok := right.(*js.ObjectExpr); ok {
				v.extractObjectKeys(obj)
			}
		}
	} else if index, ok := left.(*js.IndexExpr); ok {
		// exports['foo'] = ... or module.exports['foo'] = ...
		if v.isExportsIdent(index.X) || v.isModuleExports(index.X) {
			if name := v.extractStringLiteral(index.Y); name != "" {
				v.exports[name] = true
			}
		}
	} else if v.isModuleExports(left) {
		// module.exports = ...
		v.hasDefaultExport = true
		// Check if it's an object literal
		if obj, ok := right.(*js.ObjectExpr); ok {
			v.extractObjectKeys(obj)
		}
	}
}

func (v *exportVisitor) handleCallExpr(call *js.CallExpr) {
	// Check for Object.defineProperty(exports, 'name', { ... })
	if dot, ok := call.X.(*js.DotExpr); ok {
		if v.isObjectIdent(dot.X) && v.isDefinePropertyField(dot.Y) {
			if len(call.Args.List) >= 3 {
				// First arg should be exports or module.exports
				if v.isExportsIdent(call.Args.List[0].Value) || v.isModuleExports(call.Args.List[0].Value) {
					// Second arg is the property name
					if name := v.extractStringLiteral(call.Args.List[1].Value); name != "" {
						// Third arg is the descriptor
						if obj, ok := call.Args.List[2].Value.(*js.ObjectExpr); ok {
							if v.shouldExportDefineProperty(obj, name) {
								v.exports[name] = true
							}
						}
					}
				}
			}
		}
	}
}

func (v *exportVisitor) shouldExportDefineProperty(obj *js.ObjectExpr, name string) bool {
	hasGetter := false
	hasValue := false
	enumerableFalse := false

	for _, prop := range obj.List {
		// Handle shorthand method syntax like `get() {}`
		if method, ok := prop.Value.(*js.MethodDecl); ok {
			// Check if the method name is "get"
			methodName := string(method.Name.Literal.Data)
			if methodName == "get" || method.Get {
				hasGetter = true
				// Check if it's a safe getter
				if !v.isSafeGetterMethod(method) {
					v.unsafeGetters[name] = true
					return false
				}
			}
			continue
		}

		if prop.Name == nil || !prop.Name.IsSet() {
			continue
		}

		keyName := v.extractPropertyName(prop.Name)

		switch keyName {
		case "get":
			hasGetter = true
			// Check if it's a safe getter (returns a static member access)
			if !v.isSafeGetter(prop.Value) {
				v.unsafeGetters[name] = true
				return false
			}
		case "value":
			hasValue = true
		case "enumerable":
			if lit, ok := prop.Value.(*js.LiteralExpr); ok {
				if string(lit.Data) == "false" {
					enumerableFalse = true
				}
			}
		}
	}

	// Check if this property was previously marked as unsafe
	if v.unsafeGetters[name] {
		delete(v.exports, name)
		return false
	}

	// If it has a getter and enumerable is false, don't export
	if hasGetter && enumerableFalse {
		return false
	}

	// If it has either a value or a getter, export it
	return hasValue || hasGetter
}

func (v *exportVisitor) isSafeGetter(expr js.IExpr) bool {
	// A safe getter is a function that returns a static member access
	// like: function() { return obj.prop; }
	fn, ok := expr.(*js.FuncDecl)
	if !ok {
		return false
	}

	if len(fn.Body.List) == 0 {
		return false
	}

	// Look for a return statement
	for _, stmt := range fn.Body.List {
		if ret, ok := stmt.(*js.ReturnStmt); ok {
			if ret.Value != nil {
				// Check if it's a dot or index expression (static member access)
				switch ret.Value.(type) {
				case *js.DotExpr, *js.IndexExpr, *js.Var:
					return true
				}
			}
		}
	}

	return false
}

func (v *exportVisitor) isSafeGetterMethod(method *js.MethodDecl) bool {
	// A safe getter is a method that returns a static member access
	if len(method.Body.List) == 0 {
		return false
	}

	// Look for a return statement
	for _, stmt := range method.Body.List {
		if ret, ok := stmt.(*js.ReturnStmt); ok {
			if ret.Value != nil {
				// Check if it's a dot or index expression (static member access)
				switch ret.Value.(type) {
				case *js.DotExpr, *js.IndexExpr, *js.Var:
					return true
				}
			}
		}
	}

	return false
}

func (v *exportVisitor) extractObjectKeys(obj *js.ObjectExpr) {
	for _, prop := range obj.List {
		// Skip spread properties
		if prop.Spread {
			continue
		}

		if prop.Name == nil || !prop.Name.IsSet() {
			continue
		}

		// Extract the key name
		if keyName := v.extractPropertyName(prop.Name); keyName != "" {
			v.exports[keyName] = true
		}
	}
}

func (v *exportVisitor) isExportsIdent(expr js.IExpr) bool {
	if ident, ok := expr.(*js.Var); ok {
		return string(ident.Data) == "exports"
	}
	return false
}

func (v *exportVisitor) isModuleIdent(expr js.IExpr) bool {
	if ident, ok := expr.(*js.Var); ok {
		return string(ident.Data) == "module"
	}
	return false
}

func (v *exportVisitor) isObjectIdent(expr js.IExpr) bool {
	if ident, ok := expr.(*js.Var); ok {
		return string(ident.Data) == "Object"
	}
	return false
}

func (v *exportVisitor) isExportsField(expr js.IExpr) bool {
	if ident, ok := expr.(*js.Var); ok {
		return string(ident.Data) == "exports"
	}
	if lit, ok := expr.(js.LiteralExpr); ok {
		return string(lit.Data) == "exports"
	}
	return false
}

func (v *exportVisitor) isDefinePropertyField(expr js.IExpr) bool {
	if ident, ok := expr.(*js.Var); ok {
		return string(ident.Data) == "defineProperty"
	}
	if lit, ok := expr.(js.LiteralExpr); ok {
		return string(lit.Data) == "defineProperty"
	}
	return false
}

func (v *exportVisitor) isModuleExports(expr js.IExpr) bool {
	if dot, ok := expr.(*js.DotExpr); ok {
		return v.isModuleIdent(dot.X) && v.isExportsField(dot.Y)
	}
	return false
}

func (v *exportVisitor) extractStringLiteral(expr js.IExpr) string {
	if lit, ok := expr.(*js.LiteralExpr); ok {
		data := string(lit.Data)
		// Remove quotes and unescape
		if len(data) >= 2 {
			if (data[0] == '"' && data[len(data)-1] == '"') ||
				(data[0] == '\'' && data[len(data)-1] == '\'') {
				unquoted := data[1 : len(data)-1]
				return unescapeJSString(unquoted)
			}
		}
	}
	return ""
}

// unescapeJSString unescapes JavaScript string escape sequences
func unescapeJSString(s string) string {
	var result []rune
	i := 0
	for i < len(s) {
		if s[i] != '\\' {
			result = append(result, rune(s[i]))
			i++
			continue
		}

		// Handle escape sequence
		if i+1 >= len(s) {
			result = append(result, '\\')
			break
		}

		switch s[i+1] {
		case 'n':
			result = append(result, '\n')
			i += 2
		case 't':
			result = append(result, '\t')
			i += 2
		case 'r':
			result = append(result, '\r')
			i += 2
		case 'b':
			result = append(result, '\b')
			i += 2
		case 'f':
			result = append(result, '\f')
			i += 2
		case 'v':
			result = append(result, '\v')
			i += 2
		case '0':
			// Octal or null
			if i+2 < len(s) && s[i+2] >= '0' && s[i+2] <= '7' {
				// Octal escape \OOO
				end := i + 2
				for end < len(s) && end < i+4 && s[end] >= '0' && s[end] <= '7' {
					end++
				}
				octal := s[i+1 : end]
				var val int
				fmt.Sscanf(octal, "%o", &val)
				result = append(result, rune(val))
				i = end
			} else {
				result = append(result, '\x00')
				i += 2
			}
		case '1', '2', '3', '4', '5', '6', '7':
			// Octal escape
			end := i + 1
			for end < len(s) && end < i+4 && s[end] >= '0' && s[end] <= '7' {
				end++
			}
			octal := s[i+1 : end]
			var val int
			fmt.Sscanf(octal, "%o", &val)
			result = append(result, rune(val))
			i = end
		case 'x':
			// Hex escape \xHH
			if i+3 < len(s) {
				hex := s[i+2 : i+4]
				var val int
				fmt.Sscanf(hex, "%x", &val)
				result = append(result, rune(val))
				i += 4
			} else {
				result = append(result, 'x')
				i += 2
			}
		case 'u':
			// Unicode escape \uHHHH or \u{HHHHHH}
			if i+2 < len(s) && s[i+2] == '{' {
				// \u{HHHHHH}
				end := i + 3
				for end < len(s) && s[end] != '}' {
					end++
				}
				if end < len(s) {
					hex := s[i+3 : end]
					var val int
					fmt.Sscanf(hex, "%x", &val)
					result = append(result, rune(val))
					i = end + 1
				} else {
					result = append(result, 'u')
					i += 2
				}
			} else if i+5 < len(s) {
				// \uHHHH
				hex := s[i+2 : i+6]
				var val int
				fmt.Sscanf(hex, "%x", &val)
				result = append(result, rune(val))
				i += 6
			} else {
				result = append(result, 'u')
				i += 2
			}
		case '\\':
			result = append(result, '\\')
			i += 2
		case '\'':
			result = append(result, '\'')
			i += 2
		case '"':
			result = append(result, '"')
			i += 2
		default:
			// Unknown escape, keep the character
			result = append(result, rune(s[i+1]))
			i += 2
		}
	}
	return string(result)
}

func (v *exportVisitor) extractPropertyName(name *js.PropertyName) string {
	if name == nil || !name.IsSet() {
		return ""
	}

	// Check if it's a computed property
	if name.Computed != nil {
		return v.extractStringLiteral(name.Computed)
	}

	// Otherwise use the literal
	data := string(name.Literal.Data)
	// Check if it's already unquoted (identifier)
	if len(data) >= 2 &&
		((data[0] == '"' && data[len(data)-1] == '"') ||
			(data[0] == '\'' && data[len(data)-1] == '\'')) {
		return unescapeJSString(data[1 : len(data)-1])
	}
	return data
}

// extractShebang returns the shebang line (if present) and the code without it.
func extractShebang(code string) (string, string) {
	lines := bytes.Split([]byte(code), []byte("\n"))
	for i, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		if len(trimmed) >= 2 && trimmed[0] == '#' && trimmed[1] == '!' {
			shebang := string(line) + "\n"
			rest := string(bytes.Join(lines[i+1:], []byte("\n")))
			return shebang, rest
		}
		break
	}
	return "", code
}
