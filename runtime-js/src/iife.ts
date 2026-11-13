// IIFE entry injected by Go (App.AddBridgeScript(runtimejs.Script)); exposes
// the runtime as window.tango.
import * as tango from "./index.js";

(globalThis as any).tango = tango;
