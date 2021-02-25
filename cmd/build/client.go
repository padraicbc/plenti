package build

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"plenti/common"
	"plenti/readers"
	"regexp"
	"strconv"
	"strings"
	"time"

	"rogchap.com/v8go"
)

var (
	// Match var css = { ... }
	reCSS = regexp.MustCompile(`var(\s)css(\s)=(\s)\{(.*\n){0,}\};`)
	// Setup regex to find pagination.
	rePaginateCli = regexp.MustCompile(`:paginate\((.*?)\)`)
)

// SSRctx is a v8go context for loaded with components needed to render HTML.
var SSRctx *v8go.Context

// Client builds the SPA.
func Client(buildPath string, tempBuildDir string, ejectedPath string) error {

	defer Benchmark(time.Now(), "Compiling client SPA with Svelte")

	Log("\nCompiling client SPA with svelte")

	stylePath := buildPath + "/spa/bundle.css"
	// clear styles as we append bytes.
	common.Set(stylePath, &common.FData{})

	// Initialize string for layout.js component list.
	var allComponentsStr string

	// Set up counter for logging output.
	compiledComponentCounter := 0

	// Get svelte compiler code from node_modules.
	compiler, err := ioutil.ReadFile(
		fmt.Sprintf("%snode_modules/svelte/compiler.js", tempBuildDir),
	)
	if err != nil {
		return fmt.Errorf("Can't read %s/node_modules/svelte/compiler.js: %w%s", tempBuildDir, err, common.Caller())

	}
	// Remove reference to 'self' that breaks v8go on line 19 of node_modules/svelte/compiler.js.
	compilerStr := strings.Replace(string(compiler), "self.performance.now();", "'';", 1)
	ctx, err := v8go.NewContext(nil)
	if err != nil {
		return fmt.Errorf("Could not create Isolate: %w%s", err, common.Caller())

	}
	_, err = ctx.RunScript(compilerStr, "compile_svelte")
	if err != nil {
		return fmt.Errorf("Could not add svelte compiler: %w%s", err, common.Caller())

	}

	SSRctx, err = v8go.NewContext(nil)
	if err != nil {
		return fmt.Errorf("Could not create Isolate: %w%s", err, common.Caller())

	}
	// Fix "ReferenceError: exports is not defined" errors on line 1319 (exports.current_component;).
	if _, err := SSRctx.RunScript("var exports = {};", "create_ssr"); err != nil {
		return err
	}
	// Fix "TypeError: Cannot read property 'noop' of undefined" from node_modules/svelte/store/index.js.
	if _, err := SSRctx.RunScript("function noop(){}", "create_ssr"); err != nil {
		return err
	}

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
			return fmt.Errorf("Can't read %s: %w%s", svelteLib, err, common.Caller())

		}
		// Fix "Cannot access 'on_destroy' before initialization" errors on line 1320 & line 1337 of node_modules/svelte/internal/index.js.
		createSsrStr := strings.ReplaceAll(string(createSsrComponent), "function create_ssr_component(fn) {", "function create_ssr_component(fn) {var on_destroy= {};")
		// Use empty noop() function created above instead of missing method.
		createSsrStr = strings.ReplaceAll(createSsrStr, "internal.noop", "noop")
		_, err = SSRctx.RunScript(createSsrStr, "create_ssr")

		// TODO: `ReferenceError: require is not defined` error on build so cannot quit ...
		// 	node_modules/svelte/animate/index.js: ReferenceError: require is not defined
		//  node_modules/svelte/easing/index.js: ReferenceError: require is not defined
		//  node_modules/svelte/motion/index.js: ReferenceError: require is not defined
		//  node_modules/svelte/store/index.js: ReferenceError: require is not defined
		//  node_modules/svelte/transition/index.js: ReferenceError: require is not defined
		// if err != nil {
		// 	fmt.Println(fmt.Errorf("Could not add create_ssr_component() func from svelte/internal for file %s: %w%s", svelteLib, err, common.Caller()))

		// }

	}

	// Compile router separately since it's ejected from core.
	if err = (compileSvelte(ctx, SSRctx, ejectedPath+"/router.svelte", buildPath+"/spa/ejected/router.js", stylePath, tempBuildDir)); err != nil {
		return err
	}

	// Go through all file paths in the "/layout" folder.
	err = filepath.Walk(tempBuildDir+"layout", func(layoutPath string, layoutFileInfo os.FileInfo, err error) error {

		if err != nil {
			return fmt.Errorf("can't stat %s: %w", layoutPath, err)
		}
		// Create destination path.
		destFile := buildPath + "/spa" + strings.TrimPrefix(layoutPath, tempBuildDir+"layout")

		// Make sure path is a directory
		if layoutFileInfo.IsDir() {
			if common.UseMemFS {
				return nil
			}
			if err = os.MkdirAll(destFile, os.ModePerm); err != nil {
				return fmt.Errorf("can't make path: %s %w%s", layoutPath, err, common.Caller())
			}
		} else {
			// If the file is in .svelte format, compile it to .js
			if filepath.Ext(layoutPath) == ".svelte" {

				// Replace .svelte file extension with .js.
				destFile = strings.TrimSuffix(destFile, filepath.Ext(destFile)) + ".js"

				if err = compileSvelte(ctx, SSRctx, layoutPath, destFile, stylePath, tempBuildDir); err != nil {
					return fmt.Errorf("%w%s", err, common.Caller())
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
	// TODO: return file names here amd anywhere possible
	if err != nil {
		return err

	}
	if common.UseMemFS {
		b := []byte(allComponentsStr)
		common.Set(buildPath+"/spa/ejected/layout.js", &common.FData{Hash: common.CRC32Hasher(b), B: b})

	} else {

		// Write layout.js to filesystem.
		err = ioutil.WriteFile(buildPath+"/spa/ejected/layout.js", []byte(allComponentsStr), os.ModePerm)
		if err != nil {
			return fmt.Errorf("Unable to write layout.js file: %w%s", err, common.Caller())

		}
	}

	Log("Number of components compiled: " + strconv.Itoa(compiledComponentCounter))
	return nil
}

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
		paginateReplacements := rePaginateCli.FindAllStringSubmatch(slug, -1)
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
