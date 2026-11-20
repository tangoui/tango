//go:build !darwin

package main

import (
	"context"
	"os"
)

func publishDevOutput(ctx context.Context, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Rename(src, dst)
}
