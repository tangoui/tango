// C interface of the canvas accessibility bridge (canvas_a11y.m) for the
// Go side (canvas_a11y.go). Go keeps the semantics tree (ui/a11y) and
// pushes, for every added or changed node, a tga_node with everything
// UIKit may ask; canvas_a11y.m keeps one TGAXElement per node id and
// answers UIKit's queries from them without calling Go. Actions, queries
// and status changes call back into Go (tangoIOSCanvasA11y*). See
// docs/specs/mobile-canvas.md §5.1. Main thread only.

#ifndef TANGO_IOS_CANVAS_A11Y_H
#define TANGO_IOS_CANVAS_A11Y_H

#include <stdint.h>

// Trait bits: a11ybridge.IOSTraitSet (canvas_a11y.go checks they match).
// canvas_a11y.m maps each to its UIAccessibilityTrait* constant.
enum {
	TGA_TRAIT_BUTTON = 1 << 0,
	TGA_TRAIT_LINK = 1 << 1,
	TGA_TRAIT_HEADER = 1 << 2,
	TGA_TRAIT_SEARCH_FIELD = 1 << 3,
	TGA_TRAIT_IMAGE = 1 << 4,
	TGA_TRAIT_SELECTED = 1 << 5,
	TGA_TRAIT_STATIC_TEXT = 1 << 6,
	TGA_TRAIT_NOT_ENABLED = 1 << 7,
	TGA_TRAIT_ADJUSTABLE = 1 << 8,
	TGA_TRAIT_UPDATES_FREQUENTLY = 1 << 9,
	TGA_TRAIT_TAB_BAR = 1 << 10,
	TGA_TRAIT_TOGGLE_BUTTON = 1 << 11,
};

// Shapes (a11ybridge.IOSShape) and container types
// (a11ybridge.IOSContainerType).
enum {
	TGA_SHAPE_ELEMENT = 1,
	TGA_SHAPE_CONTAINER = 2,
};
enum {
	TGA_CONTAINER_NONE = 0,
	TGA_CONTAINER_DATA_TABLE = 1,
	TGA_CONTAINER_LIST = 2,
	TGA_CONTAINER_LANDMARK = 3,
	TGA_CONTAINER_SEMANTIC_GROUP = 4,
};

// Action bits of tga_node.actions: which UIKit action methods the
// element answers (the a11y.Action they perform is chosen in Go).
enum {
	TGA_ACT_ACTIVATE = 1 << 0,  // accessibilityActivate
	TGA_ACT_INCREMENT = 1 << 1, // accessibilityIncrement
	TGA_ACT_DECREMENT = 1 << 2, // accessibilityDecrement
};

// Requests passed to tangoIOSCanvasA11yAction.
enum {
	TGA_OP_ACTIVATE = 0,
	TGA_OP_INCREMENT = 1,
	TGA_OP_DECREMENT = 2,
	TGA_OP_ESCAPE = 3,         // accessibilityPerformEscape
	TGA_OP_SCROLL_FORWARD = 4, // accessibilityScroll: down, right or next
	TGA_OP_SCROLL_BACK = 5,    // accessibilityScroll: up, left or previous
	TGA_OP_CUSTOM = 6,         // custom action number arg
	TGA_OP_FOCUSED = 7,        // accessibilityElementDidBecomeFocused
};

// One node of the tree. Strings are UTF-8 and only read during the call.
typedef struct {
	uint64_t id, parent;
	int shape;     // TGA_SHAPE_*
	int self;      // a container that also exposes an element for itself
	uint32_t traits;
	int container; // TGA_CONTAINER_*
	int modal;     // accessibilityViewIsModal
	double x, y, w, h; // Node.Bounds: points in the canvas view
	const char *label, *value, *hint, *ident;
	int actions;        // TGA_ACT_*
	const char *custom; // custom action names separated by '\n' ("" none)
	int row, col;       // 1-based table cell position (0: not a cell)
	int rows, cols;     // table size (data table containers)
	const uint64_t *kids;
	int nkids;
} tga_node;

// Installs the VoiceOver / Switch Control observers once.
void tga_install(void);
// 1: VoiceOver or Switch Control is running.
int tga_assistive(void);

// An update: tga_begin, then tga_set / tga_remove for each changed node,
// then tga_end. reset drops every element first (a new tree). tga_end
// posts the notifications after the frame: ScreenChanged moving into node
// screen when it is not 0, else LayoutChanged when layout is set (or
// reread is not 0: VoiceOver then reads that element again).
void tga_begin(uintptr_t h, uint64_t root, int reset);
void tga_set(uintptr_t h, const tga_node *n);
void tga_remove(uintptr_t h, uint64_t id);
void tga_end(uintptr_t h, int layout, uint64_t screen, uint64_t reread);
// Drops every element (the tree went away) and posts LayoutChanged.
void tga_clear(uintptr_t h);
// The node with keyboard focus (0: none); VoiceOver is moved to it.
void tga_focus(uintptr_t h, uint64_t id);
// Speaks text once; assertive interrupts current speech.
void tga_announce(uintptr_t h, const char *text, int assertive);
// Starts (1) or stops (0) the deactivation check, which calls
// tangoIOSCanvasA11yTick every few seconds.
void tga_watch(uintptr_t h, int on);

// Self-test (canvas_a11y.go): the element tree as UIKit sees it, one line
// per element (malloc'd, the caller frees it); and an action performed on
// the first element whose label is label, as an assistive technology
// would (op: TGA_OP_ACTIVATE, INCREMENT, DECREMENT, ESCAPE, or -1 to
// increment the first adjustable element). Returns -1 when no element
// matches, else the action's result.
char *tga_dump(uintptr_t h);
int tga_selftest_op(uintptr_t h, const char *label, int op);
// Calls tangoIOSCanvasA11ySelftest(h) on the main thread after seconds.
void tga_after(double seconds, uintptr_t h);

#endif
