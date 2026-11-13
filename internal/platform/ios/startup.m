//go:build ios

// Startup tracing for the iOS backend. Measuring the cold start (process
// creation → first page loaded) showed where the time goes; the marks stay
// in the code and print only when TANGO_TRACE_STARTUP is set.

#import <Foundation/Foundation.h>
#include <sys/sysctl.h>
#include <sys/time.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include "startup.h"

static int traceOn = -1;
static double processStart; // Unix seconds
static NSMutableSet<NSString *> *seen;

static double nowSeconds(void) {
	struct timeval tv;
	gettimeofday(&tv, NULL);
	return tv.tv_sec + tv.tv_usec / 1e6;
}

static void traceInit(void) {
	const char *e = getenv("TANGO_TRACE_STARTUP");
	traceOn = e && *e && strcmp(e, "0") != 0;
	if (!traceOn) return;
	seen = [NSMutableSet set];
	// The kernel's process start time, so the marks include dyld and the
	// Go runtime's initialisation before any of our code runs.
	struct kinfo_proc kp;
	size_t len = sizeof kp;
	int mib[4] = {CTL_KERN, KERN_PROC, KERN_PROC_PID, getpid()};
	if (sysctl(mib, 4, &kp, &len, NULL, 0) == 0) {
		processStart = kp.kp_proc.p_starttime.tv_sec + kp.kp_proc.p_starttime.tv_usec / 1e6;
	} else {
		processStart = nowSeconds();
	}
}

__attribute__((constructor)) static void traceLoad(void) {
	tg_trace("image loaded (static initializers)");
}

void tg_trace(const char *label) {
	static dispatch_once_t once;
	dispatch_once(&once, ^{ traceInit(); });
	if (!traceOn) return;
	NSString *l = [NSString stringWithUTF8String:label];
	@synchronized(seen) {
		if ([seen containsObject:l]) return;
		[seen addObject:l];
	}
	fprintf(stderr, "tango/ios: startup %.1f ms %s\n", (nowSeconds() - processStart) * 1000, label);
}

