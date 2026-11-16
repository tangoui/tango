module tango.local/uibench/tango

go 1.27

require (
	github.com/tangoui/tango v0.0.0
	tango.local/uibench/proto v0.0.0
)

require (
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	golang.org/x/image v0.47.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
)

replace (
	github.com/tangoui/tango => ../../..
	tango.local/uibench/proto => ../proto
)
