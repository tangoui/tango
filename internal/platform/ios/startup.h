// Startup tracing and WebKit prewarming of the iOS backend (startup.m).

#ifndef TANGO_IOS_STARTUP_H
#define TANGO_IOS_STARTUP_H

// tg_trace prints "tango/ios: startup <ms> <label>" to stderr, the time
// since the process was created, when TANGO_TRACE_STARTUP is set (simctl:
// SIMCTL_CHILD_TANGO_TRACE_STARTUP=1). Any thread; each label once.
void tg_trace(const char *label);

// tg_prewarm_webkit (bridge.m) creates the first page's web view ahead of
// time and starts its web content process. Main thread, from TangoStart.
void tg_prewarm_webkit(void);

#endif
