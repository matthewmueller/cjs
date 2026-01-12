package cjs_test

import (
	"testing"

	"github.com/matryer/is"
	"github.com/matthewmueller/cjs"
	"github.com/matthewmueller/diff"
	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/js"
)

func requiresEqual(t *testing.T, actual, expected string) {
	t.Helper()
	actualAst, err := js.Parse(parse.NewInputString(string(actual)), js.Options{})
	if err != nil {
		t.Fatalf("failed to parse actual js: %v", err)
	}
	expectedAst, err := js.Parse(parse.NewInputString(string(expected)), js.Options{})
	if err != nil {
		t.Fatalf("failed to parse expected js: %v", err)
	}
	diff.TestString(t, actualAst.JSString(), expectedAst.JSString())
}

func TestUseStrict(t *testing.T) {
	is := is.New(t)
	actual, err := cjs.RewriteRequires("test.js", "/node_modules/", `
		"use strict";
		var React = __require("/node_modules/react");
		var ReactDOM = __require("/node_modules/react-dom");
	`)
	is.NoErr(err)
	requiresEqual(t, actual, `
		"use strict";
		import __cjs_import_react__ from "/node_modules/react"
		import __cjs_import_react_dom__ from "/node_modules/react-dom"
		const __cjs_imports__ = {
			"/node_modules/react": __cjs_import_react__,
			"/node_modules/react-dom": __cjs_import_react_dom__,
		}
		function __cjs_require__(path) {
			const req = __cjs_imports__[path]
			if (!req) {
				throw new Error("Module not found: " + path)
			}
			return req
		}
		var React = __cjs_require__("/node_modules/react");
		var ReactDOM = __cjs_require__("/node_modules/react-dom");
	`)
}

func TestRequireShebang(t *testing.T) {
	is := is.New(t)
	actual, err := cjs.RewriteRequires("test.js", "/node_modules/", `#!/usr/bin/env node
var fs = __require("/node_modules/fs-extra");
console.log(fs);
	`)
	is.NoErr(err)
	requiresEqual(t, actual, `#!/usr/bin/env node
import __cjs_import_fs_extra__ from "/node_modules/fs-extra"
const __cjs_imports__ = {
	"/node_modules/fs-extra": __cjs_import_fs_extra__,
}
function __cjs_require__(path) {
	const req = __cjs_imports__[path]
	if (!req) {
		throw new Error("Module not found: " + path)
	}
	return req
}
var fs = __cjs_require__("/node_modules/fs-extra");
console.log(fs);
	`)
}

func TestMultipleSameRequire(t *testing.T) {
	is := is.New(t)
	actual, err := cjs.RewriteRequires("test.js", "/node_modules/", `
		var React1 = __require("/node_modules/react");
		var React2 = __require("/node_modules/react");
		var React3 = require2("/node_modules/react");
	`)
	is.NoErr(err)
	requiresEqual(t, actual, `
		import __cjs_import_react__ from "/node_modules/react"
		const __cjs_imports__ = {
			"/node_modules/react": __cjs_import_react__,
		}
		function __cjs_require__(path) {
			const req = __cjs_imports__[path]
			if (!req) {
				throw new Error("Module not found: " + path)
			}
			return req
		}
		var React1 = __cjs_require__("/node_modules/react");
		var React2 = __cjs_require__("/node_modules/react");
		var React3 = __cjs_require__("/node_modules/react");
	`)
}

func TestNoRequires(t *testing.T) {
	is := is.New(t)
	actual, err := cjs.RewriteRequires("test.js", "/node_modules/", `
		var x = 1;
		console.log(x);
	`)
	is.NoErr(err)
	requiresEqual(t, actual, `
		var x = 1;
		console.log(x);
	`)
}

func TestNonMatchingPrefix(t *testing.T) {
	is := is.New(t)
	actual, err := cjs.RewriteRequires("test.js", "/node_modules/", `
		var local = __require("./local");
		var remote = __require("/node_modules/react");
	`)
	is.NoErr(err)
	requiresEqual(t, actual, `
		import __cjs_import_react__ from "/node_modules/react"
		const __cjs_imports__ = {
			"/node_modules/react": __cjs_import_react__,
		}
		function __cjs_require__(path) {
			const req = __cjs_imports__[path]
			if (!req) {
				throw new Error("Module not found: " + path)
			}
			return req
		}
		var local = __require("./local");
		var remote = __cjs_require__("/node_modules/react");
	`)
}

func TestDifferentFunctionNames(t *testing.T) {
	is := is.New(t)
	actual, err := cjs.RewriteRequires("test.js", "/lib/", `
		var a = require1("/lib/a");
		var b = require2("/lib/b");
		var c = myRequire("/lib/c");
	`)
	is.NoErr(err)
	requiresEqual(t, actual, `
		import __cjs_import_a__ from "/lib/a"
		import __cjs_import_b__ from "/lib/b"
		import __cjs_import_c__ from "/lib/c"
		const __cjs_imports__ = {
			"/lib/a": __cjs_import_a__,
			"/lib/b": __cjs_import_b__,
			"/lib/c": __cjs_import_c__,
		}
		function __cjs_require__(path) {
			const req = __cjs_imports__[path]
			if (!req) {
				throw new Error("Module not found: " + path)
			}
			return req
		}
		var a = __cjs_require__("/lib/a");
		var b = __cjs_require__("/lib/b");
		var c = __cjs_require__("/lib/c");
	`)
}

func TestScopedPackage(t *testing.T) {
	is := is.New(t)
	actual, err := cjs.RewriteRequires("test.js", "/node_modules/", `
		var babel = __require("/node_modules/@babel/core");
		var react = __require("/node_modules/@react/hooks");
	`)
	is.NoErr(err)
	requiresEqual(t, actual, `
		import __cjs_import_core__ from "/node_modules/@babel/core"
		import __cjs_import_hooks__ from "/node_modules/@react/hooks"
		const __cjs_imports__ = {
			"/node_modules/@babel/core": __cjs_import_core__,
			"/node_modules/@react/hooks": __cjs_import_hooks__,
		}
		function __cjs_require__(path) {
			const req = __cjs_imports__[path]
			if (!req) {
				throw new Error("Module not found: " + path)
			}
			return req
		}
		var babel = __cjs_require__("/node_modules/@babel/core");
		var react = __cjs_require__("/node_modules/@react/hooks");
	`)
}

func TestReactDom(t *testing.T) {
	is := is.New(t)
	actual, err := cjs.RewriteRequires("test.js", "/node_modules/", `
		var __getOwnPropNames = Object.getOwnPropertyNames;
		var __require = /* @__PURE__ */ ((x) => typeof require !== "undefined" ? require : typeof Proxy !== "undefined" ? new Proxy(x, {
			get: (a, b) => (typeof require !== "undefined" ? require : a)[b]
		}) : x)(function(x) {
			if (typeof require !== "undefined") return require.apply(this, arguments);
			throw Error('Dynamic require of "' + x + '" is not supported');
		});
		var __commonJS = (cb, mod) => function __require2() {
			return mod || (0, cb[__getOwnPropNames(cb)[0]])((mod = { exports: {} }).exports, mod), mod.exports;
		};
		// node_modules/react-dom/cjs/react-dom-client.development.js
		var require_react_dom_client_development = __commonJS({
			"node_modules/react-dom/cjs/react-dom-client.development.js"(exports) {
				"use strict";
				(function() {
					function findHook(fiber, id) {
						for (fiber = fiber.memoizedState; null !== fiber && 0 < id; )
							fiber = fiber.next, id--;
						return fiber;
					}
					"undefined" !== typeof __REACT_DEVTOOLS_GLOBAL_HOOK__ && "function" === typeof __REACT_DEVTOOLS_GLOBAL_HOOK__.registerInternalModuleStart && __REACT_DEVTOOLS_GLOBAL_HOOK__.registerInternalModuleStart(Error());
					var Scheduler = __require("/node_modules/scheduler"), React = __require("/node_modules/react"), ReactDOM = __require("/node_modules/react-dom");
					/* @__PURE__ */ Symbol.for("react.scope");
				})();
			}
		});
	`)
	is.NoErr(err)
	requiresEqual(t, actual, `
		import __cjs_import_scheduler__ from "/node_modules/scheduler"
		import __cjs_import_react__ from "/node_modules/react"
		import __cjs_import_react_dom__ from "/node_modules/react-dom"
		const __cjs_imports__ = {
			"/node_modules/scheduler": __cjs_import_scheduler__,
			"/node_modules/react": __cjs_import_react__,
			"/node_modules/react-dom": __cjs_import_react_dom__,
		}
		function __cjs_require__(path) {
			const req = __cjs_imports__[path]
			if (!req) {
				throw new Error("Module not found: " + path)
			}
			return req
		}
		var __getOwnPropNames = Object.getOwnPropertyNames;
		var __require = /* @__PURE__ */ ((x) => typeof require !== "undefined" ? require : typeof Proxy !== "undefined" ? new Proxy(x, {
			get: (a, b) => (typeof require !== "undefined" ? require : a)[b]
		}) : x)(function(x) {
			if (typeof require !== "undefined") return require.apply(this, arguments);
			throw Error('Dynamic require of "' + x + '" is not supported');
		});
		var __commonJS = (cb, mod) => function __require2() {
			return mod || (0, cb[__getOwnPropNames(cb)[0]])((mod = { exports: {} }).exports, mod), mod.exports;
		};
		// node_modules/react-dom/cjs/react-dom-client.development.js
		var require_react_dom_client_development = __commonJS({
			"node_modules/react-dom/cjs/react-dom-client.development.js"(exports) {
				"use strict";
				(function() {
					function findHook(fiber, id) {
						for (fiber = fiber.memoizedState; null !== fiber && 0 < id; )
							fiber = fiber.next, id--;
						return fiber;
					}
					"undefined" !== typeof __REACT_DEVTOOLS_GLOBAL_HOOK__ && "function" === typeof __REACT_DEVTOOLS_GLOBAL_HOOK__.registerInternalModuleStart && __REACT_DEVTOOLS_GLOBAL_HOOK__.registerInternalModuleStart(Error());
					var Scheduler = __cjs_require__("/node_modules/scheduler"), React = __cjs_require__("/node_modules/react"), ReactDOM = __cjs_require__("/node_modules/react-dom");
					/* @__PURE__ */ Symbol.for("react.scope");
				})();
			}
		});
	`)
}
