//go:build !windows

package hostsession

func keepSessionAwake() func() { return func() {} }
