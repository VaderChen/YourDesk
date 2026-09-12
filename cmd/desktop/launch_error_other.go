//go:build !windows

package main

import (
	"fmt"
	"os"
)

func reportLaunchFailure(err error) { fmt.Fprintln(os.Stderr, err) }
