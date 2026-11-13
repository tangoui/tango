//go:build darwin && !ios && !tango_dev

package darwin

// devWorker is false in production builds: they ignore TANGO_DEV_ROLE.
const devWorker = false
