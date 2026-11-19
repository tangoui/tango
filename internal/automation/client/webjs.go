package client

import (
	"encoding/json"
	"fmt"
	"time"
)

// The page-side scripts of WebView selector operations. Each is an
// expression for OpEval whose Promise settles within to (the eval's own
// timeout should be a little longer) or rejects with a message naming
// what it waited for.

// WaitSelectorJS resolves with true once an element matches the CSS
// selector sel.
func WaitSelectorJS(sel string, to time.Duration) string {
	return waitJS(sel, to) + `.then(function(){return true})`
}

// WaitExprJS resolves with true once the JavaScript expression expr is
// truthy.
func WaitExprJS(expr string, to time.Duration) string {
	return `new Promise(function(res,rej){var t0=Date.now();(function p(){var v;try{v=(` + expr + `)}catch(e){}` +
		`if(v)return res(true);if(Date.now()-t0>` + fmt.Sprint(to.Milliseconds()) + `)return rej(new Error(` +
		jsQuote("condition not met after "+to.String()+": "+expr) + `));setTimeout(p,30)})()})`
}

// ClickJS waits for the element matching sel, scrolls it into view and
// clicks it (HTMLElement.click).
func ClickJS(sel string, to time.Duration) string {
	return waitJS(sel, to) + `.then(function(e){e.scrollIntoView({block:"center",inline:"center"});` +
		`if(e.focus)e.focus();e.click();return true})`
}

// FillJS waits for the input, textarea or contenteditable element
// matching sel, replaces its value with text and fires input and change
// events (so frameworks such as React and Vue see the change).
func FillJS(sel, text string, to time.Duration) string {
	return waitJS(sel, to) + `.then(function(e){var v=` + jsQuote(text) + `;if(e.focus)e.focus();` +
		`if(e.isContentEditable){e.textContent=v}else{var d=Object.getOwnPropertyDescriptor(Object.getPrototypeOf(e),"value");` +
		`if(d&&d.set){d.set.call(e,v)}else{e.value=v}}` +
		`e.dispatchEvent(new Event("input",{bubbles:true}));e.dispatchEvent(new Event("change",{bubbles:true}));return true})`
}

// TextJS waits for the element matching sel and resolves with its
// innerText (textContent where there is none).
func TextJS(sel string, to time.Duration) string {
	return waitJS(sel, to) + `.then(function(e){return e.innerText!==undefined?e.innerText:e.textContent})`
}

// waitJS resolves with the first element matching sel.
func waitJS(sel string, to time.Duration) string {
	return `new Promise(function(res,rej){var s=` + jsQuote(sel) + `,t0=Date.now();` +
		`(function p(){var e=document.querySelector(s);if(e)return res(e);` +
		`if(Date.now()-t0>` + fmt.Sprint(to.Milliseconds()) + `)return rej(new Error("no element matches "+s+" after ` + to.String() + `"));` +
		`setTimeout(p,30)})()})`
}

func jsQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
