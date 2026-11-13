// C interface of the canvas view's text input (canvas_text.m) for the Go
// side (canvas_text.go): UITextInput, the soft keyboard and its inset.
// See docs/specs/mobile-canvas.md §2.3 and §2.4. Main thread only.

#ifndef TANGO_IOS_CANVAS_TEXT_H
#define TANGO_IOS_CANVAS_TEXT_H

#include <stdint.h>

// UITextInputTraits of the focused field (canvas_plan.go textTraits).
typedef struct {
	int keyboard;   // UIKeyboardType
	int ret;        // UIReturnKeyType
	int secure;     // secureTextEntry
	int correct;    // UITextAutocorrectionType
	int spell;      // UITextSpellCheckingType
	int capitalize; // UITextAutocapitalizationType
} tgc_traits;

// Edits UIKit makes through UITextInput, passed to tangoIOSCanvasText
// with their text and UTF-16 range (a, b = location, length).
enum {
	TGC_TEXT_MARK = 0,    // setMarkedText:selectedRange: (a, b: selection in the text)
	TGC_TEXT_INSERT = 1,  // insertText:
	TGC_TEXT_UNMARK = 2,  // unmarkText
	TGC_TEXT_DELETE = 3,  // deleteBackward
	TGC_TEXT_REPLACE = 4, // replaceRange:withText: (a, b: the range)
	TGC_TEXT_SELECT = 5,  // setSelectedTextRange: (a, b: the range)
};

// Turns text input on (shows the soft keyboard) or off (hides it), with
// the traits to use.
void tgc_text_on(uintptr_t h, int on, tgc_traits t);
// The caret rectangle in view coordinates (points): where the input
// method places its candidate window.
void tgc_text_caret(uintptr_t h, double x, double y, double w, double hh);
// New traits for the focused field; the keyboard reloads if they differ.
void tgc_text_traits(uintptr_t h, tgc_traits t);
// Replaces the mirror: text (UTF-8, n bytes), selection and marked range
// in UTF-16 units (markStart < 0: none). notifyText and notifySel tell
// UIKit's input delegate about a change it did not make itself.
void tgc_text_mirror(uintptr_t h, const char *text, int n, int selStart, int selLen,
	int markStart, int markLen, int notifyText, int notifySel);

// Self-test (canvas_selftest.go): performs op on the view as UIKit would.
void tgc_text_selftest(uintptr_t h, int op, const char *text, int a, int b);
// 1: the view is first responder and its soft keyboard is in use.
int tgc_text_active(uintptr_t h);
// Calls tangoIOSCanvasSelftest(h) on the main thread after seconds.
void tgc_after(double seconds, uintptr_t h);

#endif
