package cjs_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/matryer/is"
	"github.com/matthewmueller/cjs"
	"github.com/matthewmueller/diff"
)

//go:generate go tool esbuild --bundle --platform=browser --format=esm --outfile=testdata/react.js react
//go:generate go tool esbuild --bundle --platform=browser --format=esm --outfile=testdata/react-dom-client.js react-dom/client --alias:react=/node_modules/react --external:/node_modules/react --alias:scheduler=/node_modules/scheduler --external:/node_modules/scheduler
//go:generate go tool esbuild --bundle --platform=browser --format=esm --outfile=testdata/react-dom-server.js react-dom/server --alias:react=/node_modules/react --external:/node_modules/react --alias:scheduler=/node_modules/scheduler --external:/node_modules/scheduler
//go:generate go tool esbuild --bundle --platform=browser --format=esm --outfile=testdata/d3.js d3

var update = flag.Bool("update", false, "update testdata files")

func replaceExt(path, newExt string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return path + newExt
	}
	return path[:len(path)-len(ext)] + newExt
}

func TestData(t *testing.T) {
	is := is.New(t)
	des, err := os.ReadDir("testdata")
	is.NoErr(err)
	for _, de := range des {
		if de.IsDir() {
			continue
		} else if filepath.Ext(de.Name()) != ".js" {
			continue
		}
		inputPath := filepath.Join("testdata", de.Name())

		// Parse exports
		inputBytes, err := os.ReadFile(inputPath)
		is.NoErr(err)
		actualExports, err := cjs.ParseExports(inputPath, string(inputBytes))
		is.NoErr(err)

		if *update {
			expectPath := filepath.Join("testdata", replaceExt(de.Name(), ".json"))
			expectBytes, err := json.MarshalIndent(actualExports, "", "  ")
			is.NoErr(err)
			err = os.WriteFile(expectPath, expectBytes, 0644)
			is.NoErr(err)
		}

		expectPath := filepath.Join("testdata", replaceExt(de.Name(), ".json"))
		expectBytes, err := os.ReadFile(expectPath)
		is.NoErr(err)
		var expectExports []string
		is.NoErr(json.Unmarshal(expectBytes, &expectExports))
		is.Equal(actualExports, expectExports)

		// Rewrite requires
		expectPath = filepath.Join("testdata", replaceExt(de.Name(), ".mjs"))
		actualRewrite, err := cjs.RewriteRequires(inputPath, "/node_modules/", string(inputBytes))
		is.NoErr(err)

		if *update {
			err = os.WriteFile(expectPath, []byte(actualRewrite), 0644)
			is.NoErr(err)
		}

		expectBytes, err = os.ReadFile(expectPath)
		is.NoErr(err)
		diff.TestString(t, actualRewrite, string(expectBytes))
	}
}
