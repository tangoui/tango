module tango.local/uibench/nsgo

go 1.27.3

require (
	github.com/nsgo-dev/nsgo v0.0.0
	tango.local/uibench/proto v0.0.0
)

require (
	github.com/ebitengine/purego v0.11.3 // indirect
	github.com/yuin/goldmark v1.8.7 // indirect
	go.yaml.in/yaml/v3 v3.0.6 // indirect
	golang.org/x/image v0.48.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/text v0.44.0 // indirect
)

replace (
	github.com/nsgo-dev/nsgo => ../third_party/nsgo
	tango.local/uibench/proto => ../proto
)
