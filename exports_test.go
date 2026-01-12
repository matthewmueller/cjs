package cjs_test

import (
	"sort"
	"testing"

	"github.com/matryer/is"
	"github.com/matthewmueller/cjs"
)

func exportsEqual(t testing.TB, actual, expect []string) {
	is := is.New(t)
	sort.Strings(actual)
	sort.Strings(expect)
	is.Equal(expect, actual)
}

func TestShebang(t *testing.T) {
	is := is.New(t)
	exports, err := cjs.ParseExports("test.js", `#!/bin/bash
		exports.foo = 'bar';
	`)
	is.NoErr(err)
	exportsEqual(t, exports, []string{
		"foo",
	})
}

func TestModuleExports(t *testing.T) {
	is := is.New(t)
	exports, err := cjs.ParseExports("test.js", `
		module.exports = 'asdf';
	`)
	is.NoErr(err)
	exportsEqual(t, exports, []string{
		"default",
	})
}

func TestModuleExportsField(t *testing.T) {
	is := is.New(t)
	exports, err := cjs.ParseExports("test.js", `
		module.exports.asdf = 'asdf';
	`)
	is.NoErr(err)
	exportsEqual(t, exports, []string{
		"asdf",
	})
}

func TestLiteralExports(t *testing.T) {
	is := is.New(t)
	exports, err := cjs.ParseExports("test.js", `
		module.exports = { a, b: c, d, 'e': f };
	`)
	is.NoErr(err)
	exportsEqual(t, exports, []string{
		"a",
		"b",
		"d",
		"e",
		"default",
	})
}

func TestExportsDotAssign(t *testing.T) {
	is := is.New(t)
	exports, err := cjs.ParseExports("test.js", `
		exports.foo = 'bar';
		exports['baz'] = 'qux';
	`)
	is.NoErr(err)
	exportsEqual(t, exports, []string{
		"foo",
		"baz",
	})
}

func TestModuleExportsReexportSpread(t *testing.T) {
	is := is.New(t)
	exports, err := cjs.ParseExports("test.js", `
		module.exports = {
			...a,
			...b,
			...require('dep1'),
			c: d,
			...require('dep2'),
			name
		};
	`)
	is.NoErr(err)
	exportsEqual(t, exports, []string{
		"c",
		"name",
		"default",
	})
}

func TestModuleAssign(t *testing.T) {
	is := is.New(t)
	exports, err := cjs.ParseExports("test.js", `
		module.exports.asdf = 'asdf';
		exports = 'asdf';
		module.exports = require('./asdf');
		if (maybe)
			module.exports = require("./another");
	`)
	is.NoErr(err)
	exportsEqual(t, exports, []string{
		"asdf",
		"default",
	})
}

func TestIgnoreESMSyntax(t *testing.T) {
	is := is.New(t)
	exports, err := cjs.ParseExports("test.js", `
		import 'x';
		export { x };
		exports.a = 1;
		export function x () {}
		exports["b"] = 2;
		import.meta.url
		import {
			y as z
		} from 'y';
		module.exports.c = 3;
		export {
			y as z,
			}
		module.exports.d = 3;
	`)
	is.NoErr(err)
	exportsEqual(t, exports, []string{
		"a",
		"b",
		"c",
		"d",
	})
}

func TestDefinePropertyValue(t *testing.T) {
	is := is.New(t)
	exports, err := cjs.ParseExports("test.js", `
		Object.defineProperty(exports, 'namedExport', { enumerable: false, value: true });
		Object.defineProperty(exports, 'namedExport', { configurable: false, value: true });
		Object.defineProperty(module.exports, 'thing', { value: true });
		Object.defineProperty(exports, "other", { enumerable: true, value: true });
		Object.defineProperty(exports, "__esModule", { value: true });
	`)
	is.NoErr(err)
	exportsEqual(t, exports, []string{
		"namedExport",
		"thing",
		"other",
		"__esModule",
	})
}

func TestRollupBabelReexportGetter(t *testing.T) {
	is := is.New(t)
	exports, err := cjs.ParseExports("test.js", `
		Object.defineProperty(exports, 'a', {
			enumerable: true,
			get: function () {
				return q.p;
			}
		});

		Object.defineProperty(exports, 'b', {
			enumerable: false,
			get: function () {
				return q.p;
			}
		});

		Object.defineProperty(exports, "c", {
			get: function get () {
				return q['p' ];
			}
		});

		Object.defineProperty(exports, 'd', {
			get: function () {
				return __ns.val;
			}
		});

		Object.defineProperty(exports, 'e', {
			get () {
				return external;
			}
		});
	`)
	is.NoErr(err)
	exportsEqual(t, exports, []string{
		"a",
		"c",
		"d",
		"e",
	})
}

func TestTypescriptReexports(t *testing.T) {
	is := is.New(t)
	exports, err := cjs.ParseExports("test.js", `
		"use strict";
		function __export(m) {
			for (var p in m) if (!exports.hasOwnProperty(p)) exports[p] = m[p];
		}
		Object.defineProperty(exports, "__esModule", { value: true });
		__export(require("external1"));
		tslib.__export(require("external2"));
		__exportStar(require("external3"));
		tslib1.__exportStar(require("external4"));

		"use strict";
		Object.defineProperty(exports, "__esModule", { value: true });
		var color_factory_1 = require("./color-factory");
		Object.defineProperty(exports, "colorFactory", { enumerable: true, get: function () { return color_factory_1.colorFactory; }, });
	`)
	is.NoErr(err)
	exportsEqual(t, exports, []string{
		"__esModule",
		"colorFactory",
	})
}

func TestEsbuildHintStyle(t *testing.T) {
	is := is.New(t)
	exports, err := cjs.ParseExports("test.js", `
		0 && (module.exports = {a, b, c}) && __exportStar(require('fs'));
	`)
	is.NoErr(err)
	exportsEqual(t, exports, []string{
		"a",
		"b",
		"c",
		"default",
	})
}

func TestTemplateStringExpressionAmbiguity(t *testing.T) {
	is := is.New(t)
	exports, err := cjs.ParseExports("test.js", "`$`\nimport('a');\n``\nexports.a = 'a';\n`a$b`\nexports['b'] = 'b';\n`{$}`\nexports['b'].b;")
	is.NoErr(err)
	exportsEqual(t, exports, []string{
		"a",
		"b",
	})
}

func TestNonIdentifiers(t *testing.T) {
	is := is.New(t)
	exports, err := cjs.ParseExports("test.js", `
		module.exports = { "ab cd": foo };
		exports["not identifier"] = "asdf";
		exports["\u{1F310}"] = 1;
		exports["\uD83C"] = 1;
		exports["\u58B8"] = 1;
		exports["\n"] = 1;
		exports["\xFF"] = 1;
		exports["       "] = 1;
		exports["z"] = 1;
		exports["'"] = 1;
		exports["@notidentifier"] = "asdf";
		Object.defineProperty(exports, "%notidentifier", { value: x });
		Object.defineProperty(exports, "hm\u{1F914}", { value: x });
		exports["\u2A09"] = 45;
		exports["\u03B1"] = 54;
		exports.package = "STRICT RESERVED!";
		exports.var = "RESERVED";
	`)
	is.NoErr(err)
	exportsEqual(t, exports, []string{
		"\n",
		"       ",
		"%notidentifier",
		"'",
		"@notidentifier",
		"ab cd",
		"default",
		"hm🤔",
		"not identifier",
		"package",
		"var",
		"z",
		"ÿ",
		"α",
		"⨉",
		"墸",
		"�",
		"🌐",
	})
}

func TestGetterOptOuts(t *testing.T) {
	is := is.New(t)
	exports, err := cjs.ParseExports("test.js", `
		Object.defineProperty(exports, 'a', {
			enumerable: true,
			get: function () {
				return q.p;
			}
		});

		if (false) {
			Object.defineProperty(exports, 'a', {
				enumerable: false,
				get: function () {
					return dynamic();
				}
			});
		}
	`)
	is.NoErr(err)
	// The second defineProperty should mark 'a' as an unsafe getter, preventing export
	exportsEqual(t, exports, []string{})
}
