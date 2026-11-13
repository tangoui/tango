//go:build ios

// Text input of the canvas view (docs/specs/mobile-canvas.md §2.3, §2.4):
// TGCanvasView adopts UITextInput, so the soft keyboard, input methods,
// autocorrection, dictation and hardware-keyboard typing all reach the
// canvas. UIKit's many queries (text in a range, selection, marked text)
// are answered from a mirror of the document; each edit is passed to Go
// (tangoIOSCanvasText), which turns it into input events and sends the
// new mirror back (tgc_text_mirror). Positions are UTF-16 offsets, the
// unit of NSString and of the Go mirror.
//
// The view stays first responder while it is in a window so hardware
// keys always reach it (canvas.m). Whether that shows the soft keyboard
// depends on text input: while it is off, inputView is an empty view, so
// no keyboard appears; turning it on reloads the input views. The
// keyboard's frame becomes the canvas' keyboard inset.

#import <UIKit/UIKit.h>
#include <string.h>

#include "canvas_view.h"
#include "_cgo_export.h"

// ---------------------------------------------------------------------------
// Positions and ranges: UTF-16 offsets into the mirror.

@interface TGTextPosition : UITextPosition
@property(nonatomic, readonly) NSInteger offset;
+ (instancetype)at:(NSInteger)offset;
@end

@implementation TGTextPosition
+ (instancetype)at:(NSInteger)offset {
	TGTextPosition *p = [self new];
	p->_offset = offset;
	return p;
}
@end

@interface TGTextRange : UITextRange
@property(nonatomic, readonly) NSRange range;
+ (instancetype)with:(NSRange)range;
@end

@implementation TGTextRange
+ (instancetype)with:(NSRange)range {
	TGTextRange *r = [self new];
	r->_range = range;
	return r;
}
- (UITextPosition *)start {
	return [TGTextPosition at:(NSInteger)self.range.location];
}
- (UITextPosition *)end {
	return [TGTextPosition at:(NSInteger)NSMaxRange(self.range)];
}
- (BOOL)isEmpty {
	return self.range.length == 0;
}
@end

static NSInteger offsetOf(UITextPosition *p) {
	return [p isKindOfClass:TGTextPosition.class] ? ((TGTextPosition *)p).offset : 0;
}

static NSRange rangeOf(UITextRange *r) {
	return [r isKindOfClass:TGTextRange.class] ? ((TGTextRange *)r).range : NSMakeRange(0, 0);
}

// utf8 returns s as UTF-8; lone surrogates (possible in NSString) become
// U+FFFD instead of failing.
static NSData *utf8(NSString *s) {
	if (!s) return [NSData data];
	NSData *d = [s dataUsingEncoding:NSUTF8StringEncoding allowLossyConversion:YES];
	return d ?: [NSData data];
}

// ---------------------------------------------------------------------------
// UITextInput

@implementation TGCanvasView (TextInput)

- (void)setupText {
	self.mText = @"";
	self.mSel = NSMakeRange(0, 0);
	self.mMark = NSMakeRange(NSNotFound, 0);
	self.caret = CGRectZero;
	self.kbScreen = CGRectNull;
	self.noKeyboard = [[UIView alloc] initWithFrame:CGRectZero];
	// The iPad shortcut bar would offer undo, redo and paste, which go
	// through UIResponder actions the canvas does not implement.
	self.inputAssistantItem.leadingBarButtonGroups = @[];
	self.inputAssistantItem.trailingBarButtonGroups = @[];
	NSNotificationCenter *nc = NSNotificationCenter.defaultCenter;
	[nc addObserver:self selector:@selector(tgKeyboardWillChange:) name:UIKeyboardWillChangeFrameNotification object:nil];
	[nc addObserver:self selector:@selector(tgKeyboardWillHide:) name:UIKeyboardWillHideNotification object:nil];
}

// send passes one edit to Go, which updates the mirror before returning.
- (void)send:(int)op text:(NSString *)text a:(NSUInteger)a b:(NSUInteger)b {
	if (!self.textOn) return;
	NSMutableData *d = [utf8(text) mutableCopy];
	[d appendBytes:"" length:1];
	tangoIOSCanvasText(self.handle, op, (char *)d.bytes, (int)MIN(a, (NSUInteger)INT32_MAX), (int)MIN(b, (NSUInteger)INT32_MAX));
}

- (UIView *)inputView {
	return self.textOn ? nil : self.noKeyboard;
}

- (UIView *)textInputView {
	return self;
}

// ---- UIKeyInput ----

- (BOOL)hasText {
	return self.mText.length > 0;
}

- (void)insertText:(NSString *)text {
	[self send:TGC_TEXT_INSERT text:text a:0 b:0];
}

- (void)deleteBackward {
	[self send:TGC_TEXT_DELETE text:nil a:0 b:0];
}

// ---- edits ----

- (void)setMarkedText:(NSString *)text selectedRange:(NSRange)sel {
	text = text ?: @"";
	if (sel.location == NSNotFound || sel.location > text.length) sel = NSMakeRange(text.length, 0);
	[self send:TGC_TEXT_MARK text:text a:sel.location b:MIN(sel.length, text.length - sel.location)];
}

- (void)setAttributedMarkedText:(NSAttributedString *)text selectedRange:(NSRange)sel {
	[self setMarkedText:text.string selectedRange:sel];
}

- (void)unmarkText {
	[self send:TGC_TEXT_UNMARK text:nil a:0 b:0];
}

- (void)replaceRange:(UITextRange *)range withText:(NSString *)text {
	NSRange r = [self clamp:rangeOf(range)];
	[self send:TGC_TEXT_REPLACE text:text a:r.location b:r.length];
}

- (UITextRange *)selectedTextRange {
	return [TGTextRange with:self.mSel];
}

- (void)setSelectedTextRange:(UITextRange *)range {
	if (!range) return;
	NSRange r = [self clamp:rangeOf(range)];
	if (NSEqualRanges(r, self.mSel)) return;
	[self send:TGC_TEXT_SELECT text:nil a:r.location b:r.length];
}

- (UITextRange *)markedTextRange {
	return self.mMark.location == NSNotFound ? nil : [TGTextRange with:self.mMark];
}

- (NSDictionary<NSAttributedStringKey, id> *)markedTextStyle {
	return self.tgMarkedStyle;
}

- (void)setMarkedTextStyle:(NSDictionary<NSAttributedStringKey, id> *)style {
	self.tgMarkedStyle = style; // package ui draws the composition itself
}

- (id<UITextInputDelegate>)inputDelegate {
	return self.tgInputDelegate;
}

- (void)setInputDelegate:(id<UITextInputDelegate>)d {
	self.tgInputDelegate = d;
}

- (id<UITextInputTokenizer>)tokenizer {
	if (!self.tgTokenizer) self.tgTokenizer = [[UITextInputStringTokenizer alloc] initWithTextInput:self];
	return self.tgTokenizer;
}

// ---- document queries, from the mirror ----

- (NSRange)clamp:(NSRange)r {
	NSUInteger n = self.mText.length;
	NSUInteger s = MIN(r.location, n);
	return NSMakeRange(s, MIN(r.length, n - s));
}

- (NSString *)textInRange:(UITextRange *)range {
	if (!range) return nil;
	return [self.mText substringWithRange:[self clamp:rangeOf(range)]];
}

- (UITextPosition *)beginningOfDocument {
	return [TGTextPosition at:0];
}

- (UITextPosition *)endOfDocument {
	return [TGTextPosition at:(NSInteger)self.mText.length];
}

- (UITextRange *)textRangeFromPosition:(UITextPosition *)from toPosition:(UITextPosition *)to {
	if (!from || !to) return nil;
	NSInteger a = offsetOf(from), b = offsetOf(to);
	if (b < a) {
		NSInteger t = a;
		a = b;
		b = t;
	}
	return [TGTextRange with:NSMakeRange((NSUInteger)a, (NSUInteger)(b - a))];
}

- (UITextPosition *)positionFromPosition:(UITextPosition *)p offset:(NSInteger)off {
	if (!p) return nil;
	NSInteger o = offsetOf(p) + off;
	if (o < 0 || o > (NSInteger)self.mText.length) return nil;
	return [TGTextPosition at:o];
}

// The canvas knows no line layout: up and down move like left and right.
- (UITextPosition *)positionFromPosition:(UITextPosition *)p inDirection:(UITextLayoutDirection)dir offset:(NSInteger)off {
	BOOL back = dir == UITextLayoutDirectionLeft || dir == UITextLayoutDirectionUp;
	return [self positionFromPosition:p offset:back ? -off : off];
}

- (NSComparisonResult)comparePosition:(UITextPosition *)a toPosition:(UITextPosition *)b {
	NSInteger x = offsetOf(a), y = offsetOf(b);
	return x < y ? NSOrderedAscending : x > y ? NSOrderedDescending : NSOrderedSame;
}

- (NSInteger)offsetFromPosition:(UITextPosition *)from toPosition:(UITextPosition *)to {
	return offsetOf(to) - offsetOf(from);
}

- (UITextPosition *)positionWithinRange:(UITextRange *)range farthestInDirection:(UITextLayoutDirection)dir {
	BOOL back = dir == UITextLayoutDirectionLeft || dir == UITextLayoutDirectionUp;
	return back ? range.start : range.end;
}

- (UITextRange *)characterRangeByExtendingPosition:(UITextPosition *)p inDirection:(UITextLayoutDirection)dir {
	NSInteger o = offsetOf(p), n = (NSInteger)self.mText.length;
	o = MAX(0, MIN(o, n));
	BOOL back = dir == UITextLayoutDirectionLeft || dir == UITextLayoutDirectionUp;
	return back ? [TGTextRange with:NSMakeRange(0, (NSUInteger)o)] : [TGTextRange with:NSMakeRange((NSUInteger)o, (NSUInteger)(n - o))];
}

- (NSWritingDirection)baseWritingDirectionForPosition:(UITextPosition *)p inDirection:(UITextStorageDirection)dir {
	return NSWritingDirectionNatural;
}

- (void)setBaseWritingDirection:(NSWritingDirection)dir forRange:(UITextRange *)range {
}

// ---- geometry: only the caret is known (SetTextInput) ----

- (CGRect)caretBox {
	CGRect r = self.caret;
	if (r.size.width < 1) r.size.width = 2;
	if (r.size.height < 1) r.size.height = 20;
	return r;
}

- (CGRect)firstRectForRange:(UITextRange *)range {
	return self.caretBox;
}

- (CGRect)caretRectForPosition:(UITextPosition *)p {
	return self.caretBox;
}

- (NSArray<UITextSelectionRect *> *)selectionRectsForRange:(UITextRange *)range {
	return @[];
}

- (UITextPosition *)closestPositionToPoint:(CGPoint)pt {
	return [TGTextPosition at:(NSInteger)NSMaxRange(self.mSel)];
}

- (UITextPosition *)closestPositionToPoint:(CGPoint)pt withinRange:(UITextRange *)range {
	NSRange r = rangeOf(range);
	NSInteger o = MAX((NSInteger)r.location, MIN((NSInteger)NSMaxRange(self.mSel), (NSInteger)NSMaxRange(r)));
	return [TGTextPosition at:o];
}

- (UITextRange *)characterRangeAtPoint:(CGPoint)pt {
	return [TGTextRange with:NSMakeRange(NSMaxRange(self.mSel), 0)];
}

// ---- UITextInputTraits, from the focused field's hints ----

- (UIKeyboardType)keyboardType {
	return (UIKeyboardType)self.traits.keyboard;
}

- (void)setKeyboardType:(UIKeyboardType)v {
	tgc_traits t = self.traits;
	t.keyboard = (int)v;
	self.traits = t;
}

- (UIReturnKeyType)returnKeyType {
	return (UIReturnKeyType)self.traits.ret;
}

- (void)setReturnKeyType:(UIReturnKeyType)v {
	tgc_traits t = self.traits;
	t.ret = (int)v;
	self.traits = t;
}

- (BOOL)isSecureTextEntry {
	return self.traits.secure != 0;
}

- (void)setSecureTextEntry:(BOOL)v {
	tgc_traits t = self.traits;
	t.secure = v;
	self.traits = t;
}

- (UITextAutocorrectionType)autocorrectionType {
	return (UITextAutocorrectionType)self.traits.correct;
}

- (void)setAutocorrectionType:(UITextAutocorrectionType)v {
	tgc_traits t = self.traits;
	t.correct = (int)v;
	self.traits = t;
}

- (UITextSpellCheckingType)spellCheckingType {
	return (UITextSpellCheckingType)self.traits.spell;
}

- (void)setSpellCheckingType:(UITextSpellCheckingType)v {
	tgc_traits t = self.traits;
	t.spell = (int)v;
	self.traits = t;
}

- (UITextAutocapitalizationType)autocapitalizationType {
	return (UITextAutocapitalizationType)self.traits.capitalize;
}

- (void)setAutocapitalizationType:(UITextAutocapitalizationType)v {
	tgc_traits t = self.traits;
	t.capitalize = (int)v;
	self.traits = t;
}

// ---- keyboard inset ----

- (void)tgKeyboardWillChange:(NSNotification *)n {
	NSValue *end = n.userInfo[UIKeyboardFrameEndUserInfoKey];
	self.kbScreen = end ? end.CGRectValue : CGRectNull;
	[self updateKeyboardInset];
}

- (void)tgKeyboardWillHide:(NSNotification *)n {
	self.kbScreen = CGRectNull;
	[self updateKeyboardInset];
}

// updateKeyboardInset reports the part of the view the keyboard covers
// at the bottom. The end frame is in screen coordinates; the view need
// not fill the window (a sheet, split view). A floating or undocked
// keyboard that does not reach the view's bottom covers nothing the
// content could move away from, so it counts as zero.
- (void)updateKeyboardInset {
	double in = 0;
	UIWindow *w = self.window;
	if (w && !CGRectIsNull(self.kbScreen) && !CGRectIsEmpty(self.kbScreen)) {
		UIScreen *screen = w.windowScene.screen ?: UIScreen.mainScreen;
		CGRect kb = [self convertRect:self.kbScreen fromCoordinateSpace:screen.coordinateSpace];
		CGRect b = self.bounds;
		if (CGRectIntersectsRect(kb, b) && CGRectGetMaxY(kb) >= CGRectGetMaxY(b) - 1) {
			in = MAX(0, CGRectGetMaxY(b) - CGRectGetMinY(kb));
		}
	}
	if (in == self.kbInset) return;
	self.kbInset = in;
	tangoIOSCanvasKeyboard(self.handle, in);
}

@end

// ---------------------------------------------------------------------------
// C interface

static TGCanvasView *textView(uintptr_t h);

void tgc_text_on(uintptr_t h, int on, tgc_traits t) {
	TGCanvasView *v = textView(h);
	if (!v) return;
	v.traits = t;
	if (v.textOn == (on != 0)) return;
	v.textOn = on != 0;
	if (!v.window) return; // didMoveToWindow makes it first responder
	if (!v.isFirstResponder) {
		[v becomeFirstResponder];
	} else {
		[v reloadInputViews];
	}
}

void tgc_text_caret(uintptr_t h, double x, double y, double w, double hh) {
	textView(h).caret = CGRectMake(x, y, w, hh);
}

void tgc_text_traits(uintptr_t h, tgc_traits t) {
	TGCanvasView *v = textView(h);
	if (!v) return;
	tgc_traits old = v.traits;
	if (memcmp(&old, &t, sizeof t) == 0) return;
	v.traits = t;
	if (v.textOn && v.isFirstResponder) [v reloadInputViews];
}

void tgc_text_mirror(uintptr_t h, const char *text, int n, int selStart, int selLen,
	int markStart, int markLen, int notifyText, int notifySel) {
	TGCanvasView *v = textView(h);
	if (!v) return;
	NSString *s = [[NSString alloc] initWithBytes:text length:(NSUInteger)MAX(n, 0) encoding:NSUTF8StringEncoding] ?: @"";
	NSUInteger len = s.length;
	NSUInteger ss = MIN((NSUInteger)MAX(selStart, 0), len);
	NSRange sel = NSMakeRange(ss, MIN((NSUInteger)MAX(selLen, 0), len - ss));
	NSRange mark = NSMakeRange(NSNotFound, 0);
	if (markStart >= 0) {
		NSUInteger ms = MIN((NSUInteger)markStart, len);
		mark = NSMakeRange(ms, MIN((NSUInteger)MAX(markLen, 0), len - ms));
	}
	id<UITextInputDelegate> d = v.tgInputDelegate;
	if (notifySel) [d selectionWillChange:v];
	if (notifyText) [d textWillChange:v];
	v.mText = s;
	v.mSel = sel;
	v.mMark = mark;
	if (notifyText) [d textDidChange:v];
	if (notifySel) [d selectionDidChange:v];
}

void tgc_text_selftest(uintptr_t h, int op, const char *text, int a, int b) {
	TGCanvasView *v = textView(h);
	if (!v) return;
	NSString *s = [NSString stringWithUTF8String:text ?: ""] ?: @"";
	NSRange r = NSMakeRange((NSUInteger)MAX(a, 0), (NSUInteger)MAX(b, 0));
	switch (op) {
	case TGC_TEXT_MARK: [v setMarkedText:s selectedRange:r]; break;
	case TGC_TEXT_INSERT: [v insertText:s]; break;
	case TGC_TEXT_UNMARK: [v unmarkText]; break;
	case TGC_TEXT_DELETE: [v deleteBackward]; break;
	case TGC_TEXT_REPLACE: [v replaceRange:[TGTextRange with:r] withText:s]; break;
	case TGC_TEXT_SELECT: [v setSelectedTextRange:[TGTextRange with:r]]; break;
	}
}

int tgc_text_active(uintptr_t h) {
	TGCanvasView *v = textView(h);
	return v && v.textOn && v.isFirstResponder && v.inputView == nil;
}

void tgc_after(double seconds, uintptr_t h) {
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(seconds * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
		@autoreleasepool {
			tangoIOSCanvasSelftest(h);
		}
	});
}

// textView finds the view through canvas.m's registry.
static TGCanvasView *textView(uintptr_t h) {
	return tgc_view(h);
}
