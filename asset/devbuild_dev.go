//go:build tango_dev

package asset

// devBuild is true in development builds (-tags tango_dev, as tan dev
// builds): FromEnv honours DevURLEnv.
const devBuild = true
