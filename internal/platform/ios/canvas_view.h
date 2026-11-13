// Objective-C declarations shared by canvas.m (the canvas view: frames,
// touches, hardware keys) and canvas_text.m (its UITextInput and the soft
// keyboard). Not visible to Go.

#ifndef TANGO_IOS_CANVAS_VIEW_H
#define TANGO_IOS_CANVAS_VIEW_H

#import <UIKit/UIKit.h>
#import <QuartzCore/QuartzCore.h>

#include "canvas_text.h"

@interface TGCanvasView : UIView
@property(nonatomic) uintptr_t handle;
@property(nonatomic, strong) CAMetalLayer *metalLayer;
@property(nonatomic, strong) CALayer *cpuLayer;
@property(nonatomic, strong) CADisplayLink *link;
@property(nonatomic) CFRunLoopTimerRef timer;
@property(nonatomic) UIEdgeInsets insets;
@property(nonatomic) BOOL tgFocused; // as last reported to Go
- (CGFloat)scale;

// Text input state (canvas_text.m). The mirror holds the document Go
// last reported (canvas_textdoc.go), in UTF-16 units; UIKit's queries
// are answered from it without calling Go.
@property(nonatomic) BOOL textOn;
@property(nonatomic, copy) NSString *mText;
@property(nonatomic) NSRange mSel;
@property(nonatomic) NSRange mMark; // location NSNotFound: no marked text
@property(nonatomic) CGRect caret;  // view coordinates (points)
@property(nonatomic) tgc_traits traits;
@property(nonatomic, weak) id<UITextInputDelegate> tgInputDelegate;
@property(nonatomic, strong) id<UITextInputTokenizer> tgTokenizer;
@property(nonatomic, copy) NSDictionary<NSAttributedStringKey, id> *tgMarkedStyle;
@property(nonatomic, strong) UIView *noKeyboard; // inputView while text input is off
@property(nonatomic) CGRect kbScreen;            // keyboard end frame (screen); CGRectNull: hidden
@property(nonatomic) double kbInset;             // as last reported to Go

// Accessibility state (canvas_a11y.m): the elements of the semantics
// tree; nil until the first update.
@property(nonatomic, strong) id tgAX;
@end

// canvas_text.m: UITextInput, the soft keyboard and its inset.
@interface TGCanvasView (TextInput) <UITextInput>
- (void)setupText;           // once, after init
- (void)updateKeyboardInset; // after a layout or keyboard change
@end

// canvas_a11y.m: drops the accessibility elements and stops the
// deactivation timer (the view is closing).
void tga_teardown(TGCanvasView *v);

// canvas.m: the open canvas view with handle h, or nil.
TGCanvasView *tgc_view(uintptr_t h);

#endif
