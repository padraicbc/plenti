import svelte from 'svelte/compiler.js';
//const svelte = require('svelte/compiler.js');

// The "component" variable gets injected by client.go.
//let component;

// Create component JS that can run in the browser.
/*
export let { js, css } = svelte.compile('/home/jimafisk/Desktop/aaa/layout/content/pages.svelte', {
	css: false
});
*/


function getComp(component) {
    return {js} = svelte.compile(component, {
	    css: false
    });
}

/*
(() => {
    return {js} = svelte.compile(component, {
	    css: false
    });
})();
*/
// Return the JS and CSS object.
//(() => { js, css })();
//(() => js)();
//(() => component)();
//(() => js)();
//return "test";