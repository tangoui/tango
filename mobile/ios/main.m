// Native host for a Tango app on iOS. The app's Go code is built as a
// c-archive (go build -buildmode=c-archive) and linked with this file.
//
// The host owns the main thread: it runs UIApplicationMain and, once UIKit
// has finished launching, calls TangoStart, which installs Tango's UIKit
// side and runs the Go program's main() on a goroutine. Pages are shown in
// the window that TangoSceneDelegate (named in Info.plist) creates.
//
// Keep this file small. Add your own UIApplicationDelegate methods (push
// notifications, URL handling, ...) here as needed; just call TangoStart
// from application:didFinishLaunchingWithOptions:.

#import <UIKit/UIKit.h>
#include <stdio.h>

extern void TangoStart(void); // exported by the Go archive
extern void tg_trace(const char *label); // startup marks (TANGO_TRACE_STARTUP)

@interface AppDelegate : UIResponder <UIApplicationDelegate>
@end

@implementation AppDelegate

- (BOOL)application:(UIApplication *)application didFinishLaunchingWithOptions:(NSDictionary *)launchOptions {
	tg_trace("didFinishLaunching");
	TangoStart();
	return YES;
}

@end

int main(int argc, char *argv[]) {
	tg_trace("main");
	@autoreleasepool {
		// Go's log package writes to stderr: keep it unbuffered so it shows
		// up immediately in `simctl launch --console-pty` and Xcode.
		setvbuf(stdout, NULL, _IONBF, 0);
		setvbuf(stderr, NULL, _IONBF, 0);
		return UIApplicationMain(argc, argv, nil, NSStringFromClass(AppDelegate.class));
	}
}
