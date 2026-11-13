//go:build !tango_dev

package asset

// devBuild is false in production builds, the default: FromEnv ignores
// DevURLEnv, so another process cannot point a shipped app at a page of
// its choosing by setting the variable, and the dev proxy is not linked.
const devBuild = false
