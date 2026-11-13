//go:build ios

// UIKit half of the canvas accessibility bridge (docs/specs/mobile-canvas.md
// §5.1). The TGCanvasView is not an accessibility element itself; its
// accessibilityElements are TGAXElement objects (UIAccessibilityElement
// subclasses), one per node of the semantics tree, in the tree's reading
// order. Lists, tables, landmarks, groups and dialogs are elements that
// are containers (isAccessibilityElement NO, accessibilityElements set);
// a container whose node is also a control (a list item holding a
// switch) exposes a second element for itself, first among its children.
//
// Elements are created and updated only from what Go pushes (tga_set), so
// every UIKit query is answered here without calling Go; they are kept by
// node id across updates, since VoiceOver follows its cursor by object
// identity. Frames are the node bounds in the view and are converted to
// screen coordinates when asked, so scrolling the page or rotating needs
// no new tree. Actions go to Go (tangoIOSCanvasA11yAction), which hands
// them to the sink.

#import <UIKit/UIKit.h>
#include <stdlib.h>
#include <string.h>

#include "canvas_a11y.h"
#include "canvas_view.h"
#include "_cgo_export.h"

static NSString *str(const char *s) {
	if (!s || !*s) return nil;
	return [NSString stringWithUTF8String:s];
}

static UIAccessibilityTraits uiTraits(uint32_t t) {
	static const struct {
		uint32_t bit;
		const UIAccessibilityTraits *trait;
	} table[] = {
		{TGA_TRAIT_BUTTON, &UIAccessibilityTraitButton},
		{TGA_TRAIT_LINK, &UIAccessibilityTraitLink},
		{TGA_TRAIT_HEADER, &UIAccessibilityTraitHeader},
		{TGA_TRAIT_SEARCH_FIELD, &UIAccessibilityTraitSearchField},
		{TGA_TRAIT_IMAGE, &UIAccessibilityTraitImage},
		{TGA_TRAIT_SELECTED, &UIAccessibilityTraitSelected},
		{TGA_TRAIT_STATIC_TEXT, &UIAccessibilityTraitStaticText},
		{TGA_TRAIT_NOT_ENABLED, &UIAccessibilityTraitNotEnabled},
		{TGA_TRAIT_ADJUSTABLE, &UIAccessibilityTraitAdjustable},
		{TGA_TRAIT_UPDATES_FREQUENTLY, &UIAccessibilityTraitUpdatesFrequently},
		{TGA_TRAIT_TAB_BAR, &UIAccessibilityTraitTabBar},
		{TGA_TRAIT_TOGGLE_BUTTON, &UIAccessibilityTraitToggleButton},
	};
	UIAccessibilityTraits r = UIAccessibilityTraitNone;
	for (size_t i = 0; i < sizeof table / sizeof table[0]; i++) {
		if (t & table[i].bit) r |= *table[i].trait;
	}
	return r;
}

static UIAccessibilityContainerType uiContainer(int c) {
	switch (c) {
	case TGA_CONTAINER_DATA_TABLE: return UIAccessibilityContainerTypeDataTable;
	case TGA_CONTAINER_LIST: return UIAccessibilityContainerTypeList;
	case TGA_CONTAINER_LANDMARK: return UIAccessibilityContainerTypeLandmark;
	case TGA_CONTAINER_SEMANTIC_GROUP: return UIAccessibilityContainerTypeSemanticGroup;
	}
	return UIAccessibilityContainerTypeNone;
}

@class TGAXState;

@interface TGAXElement : UIAccessibilityElement <UIAccessibilityContainerDataTable, UIAccessibilityContainerDataTableCell>
@property(nonatomic) uint64_t nid;
@property(nonatomic) uint64_t parentID;
@property(nonatomic, weak) TGCanvasView *canvas;
@property(nonatomic, weak) TGAXElement *owner; // the container a self element stands for
@property(nonatomic) BOOL isContainer;
@property(nonatomic) int containerKind;
@property(nonatomic) CGRect box;
@property(nonatomic) uint32_t tgTraits;
@property(nonatomic) int acts;
@property(nonatomic) NSInteger row, col, rows, cols; // 1-based; 0 unknown
@property(nonatomic, copy) NSArray<NSNumber *> *kids;
@property(nonatomic, strong) TGAXElement *selfElement;
@property(nonatomic, copy) NSArray *cachedChildren;
@property(nonatomic) uint64_t cacheGen;
@end

@interface TGAXState : NSObject
@property(nonatomic, strong) NSMutableDictionary<NSNumber *, TGAXElement *> *elems;
@property(nonatomic) uint64_t root;
@property(nonatomic, copy) NSArray<NSNumber *> *rootKids;
@property(nonatomic, copy) NSArray *cachedRoot;
@property(nonatomic) uint64_t gen; // bumped by every update: child caches compare it
@property(nonatomic) uint64_t focus;
@property(nonatomic, strong) NSTimer *timer;
// Notifications waiting for the end of the frame.
@property(nonatomic) BOOL pendLayout;
@property(nonatomic) uint64_t pendScreen, pendReread, pendFocus;
@property(nonatomic, strong) NSMutableArray<NSAttributedString *> *pendSpeech;
@property(nonatomic) BOOL flushQueued;
@end

@implementation TGAXState
@end

static TGAXState *stateOf(TGCanvasView *v) {
	if (!v) return nil;
	if (!v.tgAX) {
		TGAXState *s = [TGAXState new];
		s.elems = [NSMutableDictionary dictionary];
		s.pendSpeech = [NSMutableArray array];
		v.tgAX = s;
	}
	return v.tgAX;
}

static TGAXState *stateFor(uintptr_t h) {
	return stateOf(tgc_view(h));
}

// children maps node ids to their elements, skipping nodes without one.
static NSArray *childrenOf(TGAXState *s, NSArray<NSNumber *> *kids, TGAXElement *selfElement) {
	NSMutableArray *out = [NSMutableArray arrayWithCapacity:kids.count + 1];
	if (selfElement) [out addObject:selfElement];
	for (NSNumber *k in kids) {
		TGAXElement *e = s.elems[k];
		if (e) [out addObject:e];
	}
	return out;
}

@implementation TGAXElement

- (TGAXState *)state {
	return self.canvas.tgAX;
}

- (BOOL)isAccessibilityElement {
	return !self.isContainer;
}

- (id)accessibilityContainer {
	if (self.owner) return self.owner;
	TGAXState *s = self.state;
	if (!s || self.parentID == s.root) return self.canvas;
	return s.elems[@(self.parentID)] ?: self.canvas;
}

- (NSArray *)accessibilityElements {
	if (!self.isContainer) return nil;
	TGAXState *s = self.state;
	if (!s) return @[];
	if (!self.cachedChildren || self.cacheGen != s.gen) {
		self.cachedChildren = childrenOf(s, self.kids, self.selfElement);
		self.cacheGen = s.gen;
	}
	return self.cachedChildren;
}

- (NSInteger)accessibilityElementCount {
	return self.isContainer ? (NSInteger)self.accessibilityElements.count : 0;
}

- (id)accessibilityElementAtIndex:(NSInteger)i {
	NSArray *a = self.accessibilityElements;
	return i >= 0 && i < (NSInteger)a.count ? a[i] : nil;
}

- (NSInteger)indexOfAccessibilityElement:(id)e {
	NSUInteger i = [self.accessibilityElements indexOfObjectIdenticalTo:e];
	return i == NSNotFound ? NSNotFound : (NSInteger)i;
}

- (UIAccessibilityContainerType)accessibilityContainerType {
	return self.isContainer ? uiContainer(self.containerKind) : UIAccessibilityContainerTypeNone;
}

// Bounds are points in the canvas view, already mirrored for RTL by the
// layout: converted, never flipped.
- (CGRect)accessibilityFrame {
	TGCanvasView *v = self.canvas;
	if (!v || !v.window) return CGRectZero;
	return UIAccessibilityConvertFrameToScreenCoordinates(self.box, v);
}

- (UIAccessibilityTraits)accessibilityTraits {
	return uiTraits(self.tgTraits);
}

- (int)perform:(int)op arg:(int)arg {
	TGCanvasView *v = self.canvas;
	if (!v) return 0;
	return tangoIOSCanvasA11yAction(v.handle, self.nid, op, arg);
}

- (BOOL)accessibilityActivate {
	if (!(self.acts & TGA_ACT_ACTIVATE)) return NO; // UIKit taps the activation point
	return [self perform:TGA_OP_ACTIVATE arg:0] != 0;
}

- (void)accessibilityIncrement {
	if (self.acts & TGA_ACT_INCREMENT) [self perform:TGA_OP_INCREMENT arg:0];
}

- (void)accessibilityDecrement {
	if (self.acts & TGA_ACT_DECREMENT) [self perform:TGA_OP_DECREMENT arg:0];
}

- (BOOL)accessibilityPerformEscape {
	return [self perform:TGA_OP_ESCAPE arg:0] != 0;
}

- (BOOL)accessibilityScroll:(UIAccessibilityScrollDirection)dir {
	BOOL fwd = dir == UIAccessibilityScrollDirectionDown || dir == UIAccessibilityScrollDirectionNext ||
		dir == UIAccessibilityScrollDirectionRight;
	return [self perform:fwd ? TGA_OP_SCROLL_FORWARD : TGA_OP_SCROLL_BACK arg:0] != 0;
}

- (void)accessibilityElementDidBecomeFocused {
	[self perform:TGA_OP_FOCUSED arg:0];
}

// ---- data tables ----

- (BOOL)conformsToProtocol:(Protocol *)p {
	if (p == @protocol(UIAccessibilityContainerDataTable)) {
		return self.isContainer && self.containerKind == TGA_CONTAINER_DATA_TABLE;
	}
	if (p == @protocol(UIAccessibilityContainerDataTableCell)) {
		return self.row > 0 || self.col > 0;
	}
	return [super conformsToProtocol:p];
}

- (NSUInteger)accessibilityRowCount {
	return (NSUInteger)MAX(self.rows, 0);
}

- (NSUInteger)accessibilityColumnCount {
	return (NSUInteger)MAX(self.cols, 0);
}

static TGAXElement *findCell(TGAXElement *e, NSUInteger row, NSUInteger col, int depth) {
	if (depth > 8) return nil;
	for (TGAXElement *k in e.accessibilityElements) {
		if (![k isKindOfClass:TGAXElement.class]) continue;
		if (!k.isContainer && k.row == (NSInteger)row + 1 && k.col == (NSInteger)col + 1) return k;
		if (k.isContainer) {
			TGAXElement *f = findCell(k, row, col, depth + 1);
			if (f) return f;
		}
	}
	return nil;
}

- (id<UIAccessibilityContainerDataTableCell>)accessibilityDataTableCellElementForRow:(NSUInteger)row column:(NSUInteger)column {
	return findCell(self, row, column, 0);
}

- (NSRange)accessibilityRowRange {
	return self.row > 0 ? NSMakeRange((NSUInteger)self.row - 1, 1) : NSMakeRange(NSNotFound, 0);
}

- (NSRange)accessibilityColumnRange {
	return self.col > 0 ? NSMakeRange((NSUInteger)self.col - 1, 1) : NSMakeRange(NSNotFound, 0);
}

@end

// ---------------------------------------------------------------------------
// The canvas view: the container of the root's children.

@implementation TGCanvasView (Accessibility)

- (BOOL)isAccessibilityElement {
	return NO;
}

- (NSArray *)accessibilityElements {
	// Any query means an assistive technology or automation reads the
	// view (Voice Control, XCUITest, Accessibility Inspector).
	tangoIOSCanvasA11yQuery(self.handle);
	TGAXState *s = self.tgAX;
	if (!s) return @[];
	if (!s.cachedRoot) s.cachedRoot = childrenOf(s, s.rootKids, nil);
	return s.cachedRoot;
}

- (NSInteger)accessibilityElementCount {
	return (NSInteger)self.accessibilityElements.count;
}

- (id)accessibilityElementAtIndex:(NSInteger)i {
	NSArray *a = self.accessibilityElements;
	return i >= 0 && i < (NSInteger)a.count ? a[i] : nil;
}

- (NSInteger)indexOfAccessibilityElement:(id)e {
	NSUInteger i = [self.accessibilityElements indexOfObjectIdenticalTo:e];
	return i == NSNotFound ? NSNotFound : (NSInteger)i;
}

@end

// ---------------------------------------------------------------------------
// Activation

static void statusChanged(NSNotification *n) {
	tangoIOSCanvasA11yStatus(tga_assistive());
}

void tga_install(void) {
	static BOOL done;
	if (done) return;
	done = YES;
	NSNotificationCenter *nc = NSNotificationCenter.defaultCenter;
	for (NSNotificationName name in @[UIAccessibilityVoiceOverStatusDidChangeNotification,
		     UIAccessibilitySwitchControlStatusDidChangeNotification]) {
		[nc addObserverForName:name object:nil queue:nil usingBlock:^(NSNotification *n) {
			statusChanged(n);
		}];
	}
}

int tga_assistive(void) {
	return UIAccessibilityIsVoiceOverRunning() || UIAccessibilityIsSwitchControlRunning();
}

void tga_watch(uintptr_t h, int on) {
	TGAXState *s = stateFor(h);
	if (!s) return;
	[s.timer invalidate];
	s.timer = nil;
	if (!on) return;
	s.timer = [NSTimer timerWithTimeInterval:5 repeats:YES block:^(NSTimer *t) {
		tangoIOSCanvasA11yTick(h);
	}];
	[NSRunLoop.mainRunLoop addTimer:s.timer forMode:NSRunLoopCommonModes];
}

void tga_teardown(TGCanvasView *v) {
	TGAXState *s = v.tgAX;
	if (!s) return;
	[s.timer invalidate];
	s.timer = nil;
	[s.elems removeAllObjects];
	s.rootKids = nil;
	s.cachedRoot = nil;
	[s.pendSpeech removeAllObjects];
	v.tgAX = nil;
}

// ---------------------------------------------------------------------------
// Updates

static void touch(TGAXState *s) {
	s.gen++;
	s.cachedRoot = nil;
}

void tga_begin(uintptr_t h, uint64_t root, int reset) {
	TGAXState *s = stateFor(h);
	if (!s) return;
	if (reset) {
		[s.elems removeAllObjects];
		s.rootKids = nil;
	}
	s.root = root;
	touch(s);
}

static void fill(TGAXElement *e, const tga_node *n) {
	NSString *label = str(n->label), *value = str(n->value), *hint = str(n->hint);
	if (![e.accessibilityLabel isEqualToString:label] && (label || e.accessibilityLabel)) e.accessibilityLabel = label;
	if (![e.accessibilityValue isEqualToString:value] && (value || e.accessibilityValue)) e.accessibilityValue = value;
	if (![e.accessibilityHint isEqualToString:hint] && (hint || e.accessibilityHint)) e.accessibilityHint = hint;
	e.accessibilityIdentifier = str(n->ident);
	e.box = CGRectMake(n->x, n->y, n->w, n->h);
	e.accessibilityFrameInContainerSpace = e.box;
	e.tgTraits = n->traits;
	e.acts = n->actions;
	e.row = n->row;
	e.col = n->col;
	e.rows = n->rows;
	e.cols = n->cols;
	NSString *custom = str(n->custom);
	if (!custom) {
		e.accessibilityCustomActions = nil;
		return;
	}
	NSMutableArray *acts = [NSMutableArray array];
	__weak TGAXElement *weak = e;
	NSArray<NSString *> *names = [custom componentsSeparatedByString:@"\n"];
	for (NSUInteger i = 0; i < names.count; i++) {
		int idx = (int)i;
		[acts addObject:[[UIAccessibilityCustomAction alloc] initWithName:names[i]
			actionHandler:^BOOL(UIAccessibilityCustomAction *a) {
				TGAXElement *el = weak;
				return el && [el perform:TGA_OP_CUSTOM arg:idx] != 0;
			}]];
	}
	e.accessibilityCustomActions = acts;
}

void tga_set(uintptr_t h, const tga_node *n) {
	TGCanvasView *v = tgc_view(h);
	TGAXState *s = stateOf(v);
	if (!s) return;
	NSMutableArray<NSNumber *> *kids = [NSMutableArray arrayWithCapacity:(NSUInteger)n->nkids];
	for (int i = 0; i < n->nkids; i++) [kids addObject:@(n->kids[i])];
	if (n->id == s.root) {
		s.rootKids = kids;
		return;
	}
	NSNumber *key = @(n->id);
	TGAXElement *e = s.elems[key];
	if (!e) {
		e = [[TGAXElement alloc] initWithAccessibilityContainer:v];
		e.nid = n->id;
		e.canvas = v;
		s.elems[key] = e;
	}
	e.parentID = n->parent;
	e.isContainer = n->shape == TGA_SHAPE_CONTAINER;
	e.containerKind = n->container;
	e.accessibilityViewIsModal = n->modal != 0;
	e.kids = kids;
	fill(e, n);
	if (e.isContainer) {
		// A container reads its label when VoiceOver enters it; its value,
		// hint and actions belong to its self element.
		e.accessibilityValue = nil;
		e.accessibilityHint = nil;
		e.accessibilityCustomActions = nil;
	}
	if (e.isContainer && n->self) {
		TGAXElement *se = e.selfElement;
		if (!se) {
			se = [[TGAXElement alloc] initWithAccessibilityContainer:e];
			se.nid = n->id;
			se.canvas = v;
			se.owner = e;
			e.selfElement = se;
		}
		se.parentID = n->id;
		fill(se, n);
		se.accessibilityIdentifier = n->ident ? [NSString stringWithFormat:@"%s-self", n->ident] : nil;
	} else {
		e.selfElement = nil;
	}
	if (e.isContainer) e.acts = 0;
}

void tga_remove(uintptr_t h, uint64_t id) {
	TGAXState *s = stateFor(h);
	[s.elems removeObjectForKey:@(id)];
}

// The first element VoiceOver can land on inside e (e itself for an
// element), for ScreenChanged.
static id firstLeaf(id e, int depth) {
	if (!e || depth > 32) return nil;
	if (![e isKindOfClass:TGAXElement.class] || !((TGAXElement *)e).isContainer) return e;
	for (id k in [e accessibilityElements]) {
		id f = firstLeaf(k, depth + 1);
		if (f) return f;
	}
	return nil;
}

static void flush(TGCanvasView *v) {
	TGAXState *s = v.tgAX;
	if (!s) return;
	s.flushQueued = NO;
	BOOL posted = NO;
	if (s.pendScreen) {
		id target = firstLeaf(s.elems[@(s.pendScreen)], 0);
		UIAccessibilityPostNotification(UIAccessibilityScreenChangedNotification, target);
		posted = YES;
	} else if (s.pendLayout || s.pendReread || s.pendFocus) {
		id arg = nil;
		uint64_t want = s.pendReread ?: s.pendFocus;
		if (want) arg = s.elems[@(want)];
		if (!arg && s.pendLayout) {
			// Keep VoiceOver where it is when its element survived;
			// otherwise move it to the keyboard focus (or let it choose).
			id cur = UIAccessibilityFocusedElement(UIAccessibilityNotificationVoiceOverIdentifier);
			BOOL alive = [cur isKindOfClass:TGAXElement.class] &&
				(s.elems[@(((TGAXElement *)cur).nid)] == cur || ((TGAXElement *)cur).owner);
			if (!alive && s.focus) arg = s.elems[@(s.focus)];
		}
		UIAccessibilityPostNotification(UIAccessibilityLayoutChangedNotification, arg);
		posted = YES;
	}
	s.pendLayout = NO;
	s.pendScreen = s.pendReread = s.pendFocus = 0;
	NSArray *speech = [s.pendSpeech copy];
	[s.pendSpeech removeAllObjects];
	if (!speech.count) return;
	void (^speak)(void) = ^{
		for (NSAttributedString *a in speech) {
			UIAccessibilityPostNotification(UIAccessibilityAnnouncementNotification, a);
		}
	};
	if (posted) {
		// A layout or screen change makes VoiceOver read the new element:
		// let it start before the announcement queues or interrupts.
		dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.15 * NSEC_PER_SEC)), dispatch_get_main_queue(), speak);
	} else {
		speak();
	}
}

// queueFlush posts the pending notifications after the current frame.
static void queueFlush(TGCanvasView *v, TGAXState *s) {
	if (s.flushQueued) return;
	s.flushQueued = YES;
	__weak TGCanvasView *weak = v;
	dispatch_async(dispatch_get_main_queue(), ^{
		TGCanvasView *cv = weak;
		if (cv) flush(cv);
	});
}

void tga_end(uintptr_t h, int layout, uint64_t screen, uint64_t reread) {
	TGCanvasView *v = tgc_view(h);
	TGAXState *s = stateOf(v);
	if (!s) return;
	touch(s);
	if (layout) s.pendLayout = YES;
	if (screen) s.pendScreen = screen;
	if (reread) s.pendReread = reread;
	if (layout || screen || reread) queueFlush(v, s);
}

void tga_clear(uintptr_t h) {
	TGCanvasView *v = tgc_view(h);
	TGAXState *s = v.tgAX;
	if (!s) return;
	[s.elems removeAllObjects];
	s.rootKids = nil;
	s.focus = 0;
	touch(s);
	s.pendLayout = YES;
	queueFlush(v, s);
}

void tga_focus(uintptr_t h, uint64_t id) {
	TGCanvasView *v = tgc_view(h);
	TGAXState *s = stateOf(v);
	if (!s) return;
	s.focus = id;
	if (id && UIAccessibilityIsVoiceOverRunning()) {
		s.pendFocus = id;
		queueFlush(v, s);
	}
}

void tga_announce(uintptr_t h, const char *text, int assertive) {
	TGCanvasView *v = tgc_view(h);
	TGAXState *s = stateOf(v);
	NSString *t = str(text);
	if (!s || !t) return;
	// Polite messages wait for current speech; assertive ones interrupt.
	NSAttributedString *a = [[NSAttributedString alloc] initWithString:t
		attributes:@{UIAccessibilitySpeechAttributeQueueAnnouncement: @(assertive ? NO : YES)}];
	[s.pendSpeech addObject:a];
	queueFlush(v, s);
}

// ---------------------------------------------------------------------------
// Self-test

static NSString *traitNames(UIAccessibilityTraits t) {
	static const struct {
		const UIAccessibilityTraits *trait;
		const char *name;
	} names[] = {
		{&UIAccessibilityTraitButton, "button"}, {&UIAccessibilityTraitLink, "link"},
		{&UIAccessibilityTraitHeader, "header"}, {&UIAccessibilityTraitSearchField, "searchField"},
		{&UIAccessibilityTraitImage, "image"}, {&UIAccessibilityTraitSelected, "selected"},
		{&UIAccessibilityTraitStaticText, "staticText"}, {&UIAccessibilityTraitNotEnabled, "notEnabled"},
		{&UIAccessibilityTraitAdjustable, "adjustable"}, {&UIAccessibilityTraitUpdatesFrequently, "updatesFrequently"},
		{&UIAccessibilityTraitTabBar, "tabBar"}, {&UIAccessibilityTraitToggleButton, "toggleButton"},
	};
	NSMutableArray *out = [NSMutableArray array];
	for (size_t i = 0; i < sizeof names / sizeof names[0]; i++) {
		if (t & *names[i].trait) [out addObject:@(names[i].name)];
	}
	return [out componentsJoinedByString:@"|"];
}

static void dumpInto(NSMutableString *out, id e, int depth) {
	if (depth > 32) return;
	NSString *pad = [@"" stringByPaddingToLength:(NSUInteger)depth * 2 withString:@" " startingAtIndex:0];
	BOOL container = ![e isAccessibilityElement] && [e accessibilityElements] != nil;
	CGRect f = [e accessibilityFrame];
	[out appendFormat:@"%@%@ label=%@ value=%@ hint=%@ traits=[%@] frame={%.0f,%.0f,%.0f,%.0f}",
		pad, container ? @"container" : @"element",
		[e accessibilityLabel] ? [NSString stringWithFormat:@"\"%@\"", [e accessibilityLabel]] : @"-",
		[e accessibilityValue] ? [NSString stringWithFormat:@"\"%@\"", [e accessibilityValue]] : @"-",
		[e accessibilityHint] ? [NSString stringWithFormat:@"\"%@\"", [e accessibilityHint]] : @"-",
		traitNames([e accessibilityTraits]), f.origin.x, f.origin.y, f.size.width, f.size.height];
	if (container) [out appendFormat:@" type=%ld", (long)[e accessibilityContainerType]];
	if ([e accessibilityViewIsModal]) [out appendString:@" modal"];
	if ([e accessibilityCustomActions].count) {
		NSMutableArray *n = [NSMutableArray array];
		for (UIAccessibilityCustomAction *a in [e accessibilityCustomActions]) [n addObject:a.name];
		[out appendFormat:@" actions=[%@]", [n componentsJoinedByString:@","]];
	}
	[out appendString:@"\n"];
	if (container) {
		for (id k in [e accessibilityElements]) dumpInto(out, k, depth + 1);
	}
}

char *tga_dump(uintptr_t h) {
	TGCanvasView *v = tgc_view(h);
	if (!v) return strdup("");
	NSMutableString *out = [NSMutableString string];
	[out appendFormat:@"view isAccessibilityElement=%d\n", v.isAccessibilityElement];
	for (id e in v.accessibilityElements) dumpInto(out, e, 1);
	return strdup(out.UTF8String);
}

static id findLabel(id e, NSString *label, BOOL adjustable, int depth) {
	if (depth > 32) return nil;
	if ([e isAccessibilityElement]) {
		if (adjustable ? ([e accessibilityTraits] & UIAccessibilityTraitAdjustable) != 0
		               : [[e accessibilityLabel] isEqualToString:label]) return e;
		return nil;
	}
	for (id k in [e accessibilityElements]) {
		id f = findLabel(k, label, adjustable, depth + 1);
		if (f) return f;
	}
	return nil;
}

int tga_selftest_op(uintptr_t h, const char *label, int op) {
	TGCanvasView *v = tgc_view(h);
	if (!v) return -1;
	NSString *l = str(label) ?: @"";
	id e = nil;
	for (id k in v.accessibilityElements) {
		e = findLabel(k, l, op < 0, 1);
		if (e) break;
	}
	if (!e) return -1;
	switch (op) {
	case TGA_OP_ACTIVATE: return [e accessibilityActivate] ? 1 : 0;
	case TGA_OP_ESCAPE: return [e accessibilityPerformEscape] ? 1 : 0;
	case TGA_OP_DECREMENT: [e accessibilityDecrement]; return 1;
	default: [e accessibilityIncrement]; return 1;
	}
}

void tga_after(double seconds, uintptr_t h) {
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(seconds * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
		@autoreleasepool {
			tangoIOSCanvasA11ySelftest(h);
		}
	});
}
