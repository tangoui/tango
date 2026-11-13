// Objective-C declarations shared by bridge.m (pages, scene delegate) and
// mobile.m (mobile capabilities). Not visible to Go.

#ifndef TANGO_IOS_BRIDGE_PRIVATE_H
#define TANGO_IOS_BRIDGE_PRIVATE_H

#import <UIKit/UIKit.h>
#import <WebKit/WebKit.h>

@interface TGPage : UIViewController <WKNavigationDelegate, WKScriptMessageHandler, WKUIDelegate>
@property(nonatomic) uint64_t sid;
@property(nonatomic, strong) WKWebView *web;
@property(nonatomic) CGSize reported;
@end

@interface TangoSceneDelegate : UIResponder <UIWindowSceneDelegate>
@property(nonatomic, strong) UIWindow *window;
@end

// bridge.m, for mobile.m.
UIViewController *tg_top_controller(void); // where to present UI from; nil without a window
UIWindow *tg_window(void);
void tg_presented_ended(void);             // a presented sheet went away: let page transitions resume
TGPage *tg_page(uint64_t sid);             // the open page with sid, or nil (also for webview_ext.m)
void tg_page_register(TGPage *p);          // adds a page made elsewhere (canvas.m) under p.sid

// mobile.m, for bridge.m.
UIViewController *tg_new_host(void);       // the window's root view controller
void tg_mobile_install(void);              // from tg_install
void tg_scene_connected(UISceneConnectionOptions *options); // cold-start links

#endif
