//go:build !darwin || !cgo

package prelogin

import (
	"context"
	"errors"
)

func Status() State                                 { return State{Message: "此平台尚未提供未登入連線服務。"} }
func Configure(context.Context, bool, Config) error { return errors.New(Status().Message) }
func runDaemon(context.Context) error               { return errors.New(Status().Message) }
func runAgent(context.Context) error                { return errors.New(Status().Message) }
func install(string) error                          { return errors.New(Status().Message) }
func remove() error                                 { return errors.New(Status().Message) }

func AcquireHost(context.Context) (func(), error) { return nil, errors.New(Status().Message) }

func Incoming(context.Context, bool) (bool, error)        { return false, errors.New(Status().Message) }
func brokerRequest(context.Context, string) (bool, error) { return false, errors.New(Status().Message) }
