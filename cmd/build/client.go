package build

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"plenti/readers"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"rogchap.com/v8go"
)

// SSRctx is a v8go context for loaded with components needed to render HTML.
var SSRctx *v8go.Context
var doOnceBuild sync.Once

var doOnceBuildFunc = func(tempBuildDir, ejectedPath string) error {

	// Get svelte compiler code from node_modules.
	compiler, err := ioutil.ReadFile(
		fmt.Sprintf("%snode_modules/svelte/compiler.js", tempBuildDir),
	)
	if err != nil {
		return fmt.Errorf("Can't read %s/node_modules/svelte/compiler.js: %w", tempBuildDir, err)

	}

	// Remove reference to 'self' that breaks v8go on line 19 of node_modules/svelte/compiler.js.
	compilerStr = strings.Replace(string(compiler), "self.performance.now();", "'';", 1)

	var svelteLibs = [6]string{
		tempBuildDir + "node_modules/svelte/animate/index.js",
		tempBuildDir + "node_modules/svelte/easing/index.js",
		tempBuildDir + "node_modules/svelte/internal/index.js",
		tempBuildDir + "node_modules/svelte/motion/index.js",
		tempBuildDir + "node_modules/svelte/store/index.js",
		tempBuildDir + "node_modules/svelte/transition/index.js",
	}

	for _, svelteLib := range svelteLibs {
		// Use v8go and add create_ssr_component() function.
		createSsrComponent, err := ioutil.ReadFile(svelteLib)
		if err != nil {
			return fmt.Errorf("Can't read %s: %w", svelteLib, err)

		}
		// Fix "Cannot access 'on_destroy' before initialization" errors on line 1320 & line 1337 of node_modules/svelte/internal/index.js.
		createSsrStr := strings.ReplaceAll(string(createSsrComponent), "function create_ssr_component(fn) {", "function create_ssr_component(fn) {var on_destroy= {};")
		// Use empty noop() function created above instead of missing method.
		createSsrStr = strings.ReplaceAll(createSsrStr, "internal.noop", "noop")
		createSSRs = append(createSSRs, createSsrStr)

	}
	return nil
}

// Gets created once.
var createSSRs []string
var compilerStr string

func resetSSRAndCompilerContext() (*v8go.Context, error) {
	ctx, err := v8go.NewContext(nil)

	if err != nil {
		return nil, fmt.Errorf("Could not create Isolate: %w", err)

	}
	_, err = ctx.RunScript(compilerStr, "compile_svelte")
	if err != nil {
		return nil, fmt.Errorf("Could not add svelte compiler: %w", err)

	}
	SSRctx, err = v8go.NewContext(nil)
	if err != nil {
		return nil, fmt.Errorf("Could not create Isolate: %w", err)

	}
	// Fix "ReferenceError: exports is not defined" errors on line 1319 (exports.current_component;).
	if _, err := SSRctx.RunScript("var exports = {};", "create_ssr"); err != nil {
		return nil, err
	}
	// Fix "TypeError: Cannot read property 'noop' of undefined" from node_modules/svelte/store/index.js.
	if _, err := SSRctx.RunScript("function noop(){}", "create_ssr"); err != nil {
		return nil, err
	}

	for _, createSsrStr := range createSSRs {

		_, err = SSRctx.RunScript(createSsrStr, "create_ssr")
		SSRctx.RunScript(createSsrStr, "create_ssr")
		/*
			// `ReferenceError: require is not defined` error on build so cannot quit ...
			if err != nil {
				fmt.Printf("Could not add create_ssr_component() func from svelte/internal: %v", err)
				// return err
			}
			// TODO: Fix this ^
		*/
	}
	return ctx, nil
}

// Client builds the SPA.
func Client(buildPath string, tempBuildDir string, ejectedPath string) error {
	// create the content once
	doOnceBuild.Do(func() { doOnceBuildFunc(tempBuildDir, ejectedPath) })

	ctx, err := resetSSRAndCompilerContext()
	if err != nil {
		return err
	}

	defer Benchmark(time.Now(), "Compiling client SPA with Svelte")

	Log("\nCompiling client SPA with svelte")

	// Initialize string for layout.js component list.
	var allComponentsStr string

	// Set up counter for logging output.
	compiledComponentCounter := 0
	stylePath := buildPath + "/spa/bundle.css"

	// Compile router separately since it's ejected from core.
	if err := (compileSvelte(ctx, SSRctx, ejectedPath+"/router.svelte", buildPath+"/spa/ejected/router.js", stylePath, tempBuildDir)); err != nil {
		return err
	}

	// Go through all file paths in the "/layout" folder.
	err = filepath.Walk(tempBuildDir+"layout", func(layoutPath string, layoutFileInfo os.FileInfo, err error) error {
		// Create destination path.
		destFile := buildPath + "/spa" + strings.TrimPrefix(layoutPath, tempBuildDir+"layout")
		// Make sure path is a directory
		if layoutFileInfo.IsDir() {
			// Create any sub directories need for filepath.
			if err = os.MkdirAll(destFile, os.ModePerm); err != nil {
				return err
			}
		} else {
			// If the file is in .svelte format, compile it to .js
			if filepath.Ext(layoutPath) == ".svelte" {

				// Replace .svelte file extension with .js.
				destFile = strings.TrimSuffix(destFile, filepath.Ext(destFile)) + ".js"

				if err = compileSvelte(ctx, SSRctx, layoutPath, destFile, stylePath, tempBuildDir); err != nil {
					return err
				}

				// Remove temporary theme build directory.
				destLayoutPath := strings.TrimPrefix(layoutPath, tempBuildDir)
				// Create entry for layout.js.
				layoutSignature := strings.ReplaceAll(strings.ReplaceAll((destLayoutPath), "/", "_"), ".", "_")
				// Remove layout directory.
				destLayoutPath = strings.TrimPrefix(destLayoutPath, "layout/")
				// Compose entry for layout.js file.
				allComponentsStr = allComponentsStr + "export {default as " + layoutSignature + "} from '../" + destLayoutPath + "';\n"

				compiledComponentCounter++

			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("Could not get layout file: %w", err)

	}

	// Write layout.js to filesystem.
	err = ioutil.WriteFile(buildPath+"/spa/ejected/layout.js", []byte(allComponentsStr), os.ModePerm)
	if err != nil {
		return fmt.Errorf("Unable to write layout.js file: %w", err)

	}

	Log("Number of components compiled: " + strconv.Itoa(compiledComponentCounter))
	return nil
}

var reCSS = regexp.MustCompile(`var(\s)css(\s)=(\s)\{(.*\n){0,}\};`)

// Match var css = { ... }
func removeCSS(str string) string {

	// Delete these styles because they often break pagination SSR.
	return reCSS.ReplaceAllString(str, "")
}

func makeNameList(importNameSlice []string) []string {
	var namedImportNameStrs []string
	// Get just the name(s) of the variable(s).
	namedImportNameStr := strings.Trim(importNameSlice[1], "{ }")
	// Chech if there are multiple names separated by a comma.
	if strings.Contains(namedImportNameStr, ",") {
		// Break apart by comma and add to individual items to array.
		namedImportNameStrs = append(namedImportNameStrs, strings.Split(namedImportNameStr, ",")...)
		for i := range namedImportNameStrs {
			// Remove surrounding whitespace (this will be present if there was space after the comma).
			namedImportNameStrs[i] = strings.TrimSpace(namedImportNameStrs[i])
		}
	} else {
		// Only one name was used, so add it directly to the array.
		namedImportNameStrs = append(namedImportNameStrs, namedImportNameStr)
	}
	return namedImportNameStrs
}

type pager struct {
	contentType    string
	contentPath    string
	paginationVars []string
}

// Setup regex to find pagination.
var rePaginate = regexp.MustCompile(`:paginate\((.*?)\)`)

func getPagination() ([]pager, *regexp.Regexp) {
	// Get settings from config file.
	siteConfig, _ := readers.GetSiteConfig(".")

	// Initialize new pager struct
	var pagers []pager
	// Check for pagination in plenti.json config file.
	for configContentType, slug := range siteConfig.Types {
		// Initialize list of all :paginate() vars in a given slug.
		replacements := []string{}
		// Find every instance of :paginate() in the slug.
		paginateReplacements := rePaginate.FindAllStringSubmatch(slug, -1)
		// Loop through all :paginate() replacements found in config file.
		for _, replacement := range paginateReplacements {
			// Add the variable name defined within the brackets to the list.
			replacements = append(replacements, replacement[1])
		}
		var pager pager
		pager.contentType = configContentType
		pager.contentPath = slug
		pager.paginationVars = replacements
		pagers = append(pagers, pager)
	}
	return pagers, rePaginate
}
