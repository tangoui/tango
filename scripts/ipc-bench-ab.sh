#!/usr/bin/env bash
set -euo pipefail

if [[ $# != 2 || ! $1 =~ ^[1-9][0-9]*$ ]]; then
  echo "Usage: $0 <rounds> <out.jsonl>" >&2
  exit 2
fi

suffix=
case "${RUNNER_OS:-$(uname -s)}" in
  Linux) os=linux; lane_flag=TANGO_IPC_LANES_LINUX ;;
  Windows|MINGW*|MSYS*) os=windows; lane_flag=TANGO_IPC_LANES_WINDOWS; suffix=.exe ;;
  macOS|Darwin) os=darwin; lane_flag=TANGO_IPC_LANES_DARWIN ;;
  *) echo "Unsupported benchmark platform: ${RUNNER_OS:-$(uname -s)}" >&2; exit 2 ;;
esac

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
duplex="$tmp/duplex$suffix"
runner="$tmp/runner$suffix"

# Use Go for process timeouts and JSON on all runners, including Git Bash
# and macOS, where GNU timeout or jq may be missing.
cat > "$tmp/runner.go" <<'GO'
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"time"
)

const runTimeout = 3 * time.Minute

func benchmark(binary, laneSetting string) (map[string]json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "-bench")
	cmd.Env = append(cmd.Environ(), laneSetting)
	cmd.Stderr = os.Stderr
	// Bound stdout collection even if a WebView subprocess inherits the pipe.
	cmd.WaitDelay = time.Second
	stdout, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("benchmark timed out after %s", runTimeout)
	}
	if err != nil {
		return nil, err
	}

	var result map[string]json.RawMessage
	for _, line := range bytes.Split(stdout, []byte{'\n'}) {
		var object map[string]json.RawMessage
		if json.Unmarshal(line, &object) != nil || object == nil {
			continue
		}
		if result != nil {
			return nil, errors.New("benchmark printed multiple JSON objects on stdout")
		}
		result = object
	}
	if result == nil {
		return nil, errors.New("benchmark printed no JSON object on stdout")
	}
	return result, nil
}

func main() {
	log.SetFlags(0)
	binary, platform, laneFlag, output := os.Args[1], os.Args[2], os.Args[3], os.Args[5]
	rounds, err := strconv.Atoi(os.Args[4])
	if err != nil || rounds < 1 {
		log.Fatal("rounds must be a positive integer")
	}

	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		log.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	arms := []struct{ name, setting string }{
		{"A", "TANGO_IPC_LANES=0"},
		{"B", laneFlag + "=1"},
	}
	for index := range rounds {
		for _, arm := range arms {
			result, err := benchmark(binary, arm.setting)
			row := make(map[string]any, len(result)+3)
			if err != nil {
				row["error"] = err.Error()
			} else {
				for key, value := range result {
					row[key] = value
				}
				row["os"] = platform
			}
			row["arm"], row["round"] = arm.name, index+1
			if err := encoder.Encode(row); err != nil {
				log.Fatal(err)
			}
		}
	}
	if err := file.Close(); err != nil {
		log.Fatal(err)
	}
}
GO

go build -p 1 -o "$runner" "$tmp/runner.go"
go build -p 1 -o "$duplex" ./examples/duplex

unset TANGO_IPC_LANES
"$runner" "$duplex" "$os" "$lane_flag" "$1" "$2"
