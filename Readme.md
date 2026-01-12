# cjs [![Go Reference](https://pkg.go.dev/badge/github.com/matthewmueller/cjs.svg)](https://pkg.go.dev/github.com/matthewmueller/cjs)

Functions for turning CommonJS code into ES6 modules.

You can think of this library as just the transpiler magic behind https://esm.sh,
where it makes packages like React just work in the browser:

```js
import React, { Children } from "https://esm.sh/react"
```

## How it Works

React only ships CommonJS code ([issue](https://github.com/facebook/react/issues/10021)).

Simplifying a bit, it looks like this:

```js
if (process.env.NODE_ENV === "production") {
  module.exports = require("./cjs/react.production.js")
} else {
  module.exports = require("./cjs/react.development.js")
}

// ./cjs/react.development.js
"use strict";
(function () {
  // ...
  exports.Children = // ...
  exports.version = // ...
  return exports
})()
```

This isn't compatible with browser imports, so we perform the following steps:

1. Bundle code together using ESBuild

2. Use `cjs.ParseExports` to parse out all the CommonJS exports (e.g. `Children`, `version`, etc.)

   This is heavily influenced by the work done in [cjs-module-lexer](https://github.com/nodejs/cjs-module-lexer) by [@guybedford](https://github.com/guybedford).

3. Create a virtual ES6 module entrypoint and rebuild with ESBuild, something like:

   ```ts
   export { Children, version, default } from "react"
   ```

   This lets ESBuild take care of all the nuance between importing CJS from ESM.

4. Hoist up external node_modules using `cjs.RewriteRequires`

   React throws errors if it's imported twice on the page and if you bundle `react` and `react-dom/client` on the same page, you'll end up with two Reacts.

   So to avoid this, we externalize node modules. So for example, in `react-dom/client`, we'll merge all the local source files, but when we see `require('react')`, we transform that into `require('/node_modules/react')` so it reuses the previously imported React.

   The challenge is that we're back to CommonJS. So to fix this we'll detect all these `/node_modules/react`, add ES imports to the top of the source, and rewrite the requires to use our own require:

   ```js
   import __cjs_import_scheduler__ from "/node_modules/scheduler"
   import __cjs_import_react__ from "/node_modules/react"
   import __cjs_import_react_dom__ from "/node_modules/react-dom"
   const __cjs_imports__ = {
     "/node_modules/scheduler": __cjs_import_scheduler__,
     "/node_modules/react": __cjs_import_react__,
     "/node_modules/react-dom": __cjs_import_react_dom__,
   }

   // ...
   var Scheduler = __cjs_require__("/node_modules/scheduler")
   var React = __cjs_require__("/node_modules/react")
   var ReactDOM = __cjs_require__("/node_modules/react-dom")
   ```

This was pretty long-winded, so have a look at the tests for more details.

## Installation

```bash
go get github.com/matthewmueller/cjs
```

## Thanks

Big thanks to

- [@ije](https://github.com/ije) of [esm.sh](https://esm.sh) fame for figuring this all out
- [@guybedford](https://github.com/guybedford) for creating [cjs-module-lexer](https://github.com/nodejs/cjs-module-lexer)
- [@evanw](https://github.com/evanw) for creating and maintaining [ESBuild](https://github.com/evanw/esbuild)

## License

MIT
