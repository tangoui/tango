//go:build darwin && !ios && tango_dev

package darwin

import "os"

// devWorker: TANGO_DEV_ROLE=worker (see internal/devhost), development
// builds only.
var devWorker = os.Getenv("TANGO_DEV_ROLE") == "worker"
