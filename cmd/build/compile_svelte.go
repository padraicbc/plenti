package build

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"rogchap.com/v8go"
)

// Compile once
var (
	// Use var instead of const so it can be redeclared multiple times.
	reConst = regexp.MustCompile(`(?m)^const\s`)
	// Regex match static import statements.
	reStaticImport = regexp.MustCompile(`import\s((.*)\sfrom(.*);|(((.*)\n){0,})\}\sfrom(.*);)`)
	// Regex match static export statements.
	reStaticExport = regexp.MustCompile(`export\s(.*);`)
	// Match: allComponents["layout_components_decrementer_svelte"]
	reAllComponentsBracketStr = regexp.MustCompile(`allComponents\[\"(.*)\"\]`)
	// Replace import references with variable signatures.
	reStaticImportPath = regexp.MustCompile(`(?:'|").*(?:'|")`)
	reStaticImportName = regexp.MustCompile(`import\s(.*)\sfrom`)
	// Match: allComponents[component]
	reAllComponentsBracket = regexp.MustCompile(`allComponents\[(.*)\]`)
	// Match: allComponents.layout_components_grid_svelte
	reAllComponentsDot = regexp.MustCompile(`allComponents\.(layout_.*_svelte)`)
	// Only add named imports to create_ssr_component().
	reCreateFunc = regexp.MustCompile(`(create_ssr_component\(\(.*\)\s=>\s\{)`)
	// Only use comp signatures inside JS template literal placeholders.
	reTemplatePlaceholder = regexp.MustCompile(`(?s)\$\{validate_component\(.*\)\}`)
)

func compileSvelte(ctx *v8go.Context, SSRctx *v8go.Context, layoutPath string,
	destFile string, stylePath string, tempBuildDir string) error {

	component, err := ioutil.ReadFile(layoutPath)
	if err != nil {
		return fmt.Errorf("Can't read component: %w", err)
	}
	componentStr := string(component)
	if err = origBuildComponentStr(ctx, componentStr, destFile, stylePath); err != nil {
		return err
	}
	// example of combining RunScript  calls into one which may be faster.
	//  Won't know until we have decent benchmark set od data.
	// if err = combinedRunExample(ctx, componentStr, destFile, stylePath); err != nil {
	// 	return err
	// }

	// maybe SSR is optional?
	return ssr(ctx, SSRctx, layoutPath, stylePath, tempBuildDir, componentStr)
}

func ssr(ctx *v8go.Context, SSRctx *v8go.Context, layoutPath, stylePath string, tempBuildDir, componentStr string) error {

	// maybe do the same as compile and dont pass?
	// component, err := ioutil.ReadFile(layoutPath)
	// if err != nil {
	// 	return fmt.Errorf("Can't read component: %w", err)
	// }
	// componentStr := string(component)

	// Get Server Side Rendered (SSR) JS.
	_, ssrCompileErr := ctx.RunScript("var { js: ssrJs, css: ssrCss } = svelte.compile(`"+componentStr+"`, {generate: 'ssr'});", "compile_svelte")
	if ssrCompileErr != nil {
		return fmt.Errorf("V8go could not compile ssrJs.code: %w", ssrCompileErr)
	}
	ssrJsCode, err := ctx.RunScript("ssrJs.code;", "compile_svelte")
	if err != nil {
		return fmt.Errorf("V8go could not get ssrJs.code value: %w", err)
	}

	// Remove static import statements.
	ssrStr := reStaticImport.ReplaceAllString(ssrJsCode.String(), `/*$0*/`)
	// Remove static export statements.
	ssrStr = reStaticExport.ReplaceAllString(ssrStr, `/*$0*/`)

	ssrStr = reConst.ReplaceAllString(ssrStr, "var ")
	// Remove temporary theme directory info from path before making a comp signature.
	layoutPath = strings.TrimPrefix(layoutPath, tempBuildDir)
	// Create custom variable name for component based on the file path for the layout.
	componentSignature := strings.ReplaceAll(strings.ReplaceAll(layoutPath, "/", "_"), ".", "_")
	// Use signature instead of generic "Component". Add space to avoid also replacing part of "loadComponent".
	ssrStr = strings.ReplaceAll(ssrStr, " Component ", " "+componentSignature+" ")

	namedExports := reStaticExport.FindAllStringSubmatch(ssrStr, -1)
	// Loop through all export statements.
	for _, namedExport := range namedExports {
		// Get exported functions that aren't default.
		// Ignore names that contain semicolons to try to avoid pulling in CSS map code: https://github.com/sveltejs/svelte/issues/3604
		if !strings.HasPrefix(namedExport[1], "default ") && !strings.Contains(namedExport[1], ";") {
			// Get just the name(s) inside the curly brackets
			exportNames := makeNameList(namedExport)
			for _, exportName := range exportNames {
				if exportName != "" && componentSignature != "" {
					// Create new component signature with variable name appended to the end.
					ssrStr = strings.ReplaceAll(ssrStr, exportName, componentSignature+"_"+exportName)
				}
			}
		}
	}

	// Replace import references with variable signatures.

	namedImports := reStaticImport.FindAllString(ssrStr, -1)
	for _, namedImport := range namedImports {
		// Get path only from static import statement.
		importPath := reStaticImportPath.FindString(namedImport)
		importNameSlice := reStaticImportName.FindStringSubmatch(namedImport)
		importNameStr := ""
		var namedImportNameStrs []string
		if len(importNameSlice) > 0 {
			importNameStr = importNameSlice[1]
			// Check if it's a named import (starts w curly bracket)
			// and import path should not have spaces (ignores CSS mapping: https://github.com/sveltejs/svelte/issues/3604).
			if strings.Contains(importNameSlice[1], "{") && !strings.Contains(importPath, " ") {
				namedImportNameStrs = makeNameList(importNameSlice)
			}
		}
		// Remove quotes around path.
		importPath = strings.Trim(importPath, `'"`)
		// Get individual path arguments.
		layoutParts := strings.Split(layoutPath, "/")
		layoutFileName := layoutParts[len(layoutParts)-1]
		layoutRootPath := strings.TrimSuffix(layoutPath, layoutFileName)

		importParts := strings.Split(importPath, "/")
		// Initialize the import signature that will be used for unique variable names.
		importSignature := ""
		// Check if the path ends with a file extension, e.g. ".svelte".
		if len(filepath.Ext(importParts[len(importParts)-1])) > 1 {
			for _, importPart := range importParts {
				// Check if path starts relative to current folder.
				if importPart == "." {
					// Remove the proceeding dot so the file can be combined with the root.
					importPath = strings.TrimPrefix(importPath, "./")
				}
				// Check if path goes up a folder.
				if importPart == ".." {
					// Remove the proceeding double dots so it can be combined with root.
					importPath = strings.TrimPrefix(importPath, importPart+"/")
					// Split the layout root path so we can remove the last segment since the double dots indicates going back a folder.
					layoutParts = strings.Split(layoutRootPath, "/")
					layoutRootPath = strings.TrimSuffix(layoutRootPath, layoutParts[len(layoutParts)-2]+"/")
				}
			}
			// Create the variable name from the full path.
			importSignature = strings.ReplaceAll(strings.ReplaceAll((layoutRootPath+importPath), "/", "_"), ".", "_")
		}
		// TODO: Add an else ^ to account for NPM dependencies?
		// Check that there is a valid import to replace.
		if importNameStr != "" && importSignature != "" {

			// Only replace this specific variable, so not anything that has letters, underscores, or numbers attached to it.
			reImportNameUse := regexp.MustCompile(`([^a-zA-Z_0-9])` + importNameStr + `([^a-zA-Z_0-9])`)
			// Find the template placeholders.
			ssrStr = reTemplatePlaceholder.ReplaceAllStringFunc(ssrStr,
				func(placeholder string) string {
					// Use the signature instead of variable name.
					return reImportNameUse.ReplaceAllString(placeholder, "${1}"+importSignature+"${2}")
				},
			)
		}

		// Handle each named import, e.g. import { first, second } from "./whatever.svelte".
		for _, currentNamedImport := range namedImportNameStrs {
			// Remove whitespace on sides that might occur when splitting into array by comma.
			currentNamedImport = strings.TrimSpace(currentNamedImport)
			// Check that there is a valid named import.
			if currentNamedImport != "" && importSignature != "" {

				// Entry should be block scoped, like: let count = layout_scripts_stores_svelte_count;
				blockScopedVar := "\n let " + currentNamedImport + " = " + importSignature + "_" + currentNamedImport + ";"
				// Add block scoped var inside create_ssr_component.
				ssrStr = reCreateFunc.ReplaceAllString(ssrStr, "${1}"+blockScopedVar)
			}
		}
	}

	// Remove allComponents object (leaving just componentSignature) for SSR.
	// Match: allComponents.layout_components_grid_svelte
	ssrStr = reAllComponentsDot.ReplaceAllString(ssrStr, "${1}")
	// Match: allComponents[component]
	ssrStr = reAllComponentsBracket.ReplaceAllString(ssrStr, "globalThis[${1}]")
	// Match: allComponents["layout_components_decrementer_svelte"]

	ssrStr = reAllComponentsBracketStr.ReplaceAllString(ssrStr, "${1}")

	paginatedContent, _ := getPagination()
	for _, pager := range paginatedContent {
		if "layout_content_"+pager.contentType+"_svelte" == componentSignature {
			for _, paginationVar := range pager.paginationVars {
				// Prefix var so it doesn't conflict with other variables.
				globalVar := "plenti_global_pager_" + paginationVar
				// Initialize var outside of function to set it as global.
				ssrStr = "var " + globalVar + ";\n" + ssrStr
				// Match where the pager var is set, like: let totalPages = Math.ceil(totalPosts / postsPerPage);
				reLocalVar := regexp.MustCompile(`((let\s|const\s|var\s)` + paginationVar + `.*;)`)
				// Create statement to assign local var to global var.
				makeGlobalVar := globalVar + " = " + paginationVar + ";"
				// Assign value to global var inside create_ssr_component() func, like: plenti_global_pager_totalPages = totalPages;
				ssrStr = reLocalVar.ReplaceAllString(ssrStr, "${1}\n"+makeGlobalVar)
				// Clear out styles for SSR since they are already pulled from client components.
				ssrStr = removeCSS(ssrStr)
			}
		}
	}

	// Add component to context so it can be used to render HTML in data_source.go.
	_, err = SSRctx.RunScript(ssrStr, "create_ssr")
	if err != nil {
		return fmt.Errorf("Could not add SSR Component: %w", err)
	}
	return nil
}

func combinedRunScriptExample(ctx *v8go.Context, componentStr, destFile, stylePath string) error {
	c, err := ctx.RunScript("var { js, css } = svelte.compile(`"+componentStr+"`, {css: false, hydratable: true}); js.code +';;;;'+css.code", "compile_svelte")
	if err != nil {
		return err
	}
	bytes := strings.SplitN(c.String(), ";;;;", 2)

	jsBytes := []byte(bytes[0])
	err = ioutil.WriteFile(destFile, jsBytes, 0755)
	if err != nil {
		return fmt.Errorf("Unable to write compiled client file: %w", err)
	}
	cssStr := strings.TrimSpace(bytes[1])

	if cssStr != "null" {
		cssFile, err := os.OpenFile(stylePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("Could not open bundle.css for writing: %w", err)
		}
		defer cssFile.Close()
		if _, err := cssFile.WriteString(cssStr); err != nil {
			return fmt.Errorf("could not write to cssStr: %w", err)
		}
	}
	return nil

}

func origBuildComponentStr(ctx *v8go.Context, componentStr, destFile, stylePath string) error {

	// // Compile component with Svelte.
	_, err := ctx.RunScript("var { js, css } = svelte.compile(`"+componentStr+"`, {css: false, hydratable: true});", "compile_svelte")
	if err != nil {
		return err
	}
	// Get the JS code from the compiled result.
	jsCode, err := ctx.RunScript("js.code;", "compile_svelte")
	if err != nil {
		return fmt.Errorf("V8go could not execute js.code: %w", err)
	}
	jsBytes := []byte(jsCode.String())
	err = ioutil.WriteFile(destFile, jsBytes, 0755)
	if err != nil {
		return fmt.Errorf("Unable to write compiled client file: %w", err)
	}

	// Get the CSS code from the compiled result.
	cssCode, err := ctx.RunScript("css.code;", "compile_svelte")
	if err != nil {
		return fmt.Errorf("V8go could not execute css.code: %w", err)
	}
	cssStr := strings.TrimSpace(cssCode.String())
	// If there is CSS, write it into the bundle.css file.
	if cssStr != "null" {
		cssFile, err := os.OpenFile(stylePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("Could not open bundle.css for writing: %w", err)
		}
		defer cssFile.Close()
		if _, err := cssFile.WriteString(cssStr); err != nil {
			return fmt.Errorf("could not write to cssStr: %w", err)
		}
	}
	return nil

}
