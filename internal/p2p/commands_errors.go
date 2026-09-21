package p2p

import (
	"errors"
	"fmt"
)

// ErrCommandInterrupted classifies transport loss, deadlines, and temporary
// admission failures without inspecting localized messages. Callers must still
// reconcile mutable commands before retrying: completion may be unknown.
var ErrCommandInterrupted = errors.New("P2P 指令暫時中斷")

func interruptedCommand(err error) error {
	return fmt.Errorf("%w：%w", ErrCommandInterrupted, err)
}

func commandResponseError(out CommandResponse) error {
	if out.Code == "" {
		return nil
	}
	err := fmt.Errorf("%s：%s", out.Code, out.Error)
	switch out.Code {
	case "busy", "expired", "in_progress":
		return interruptedCommand(err)
	default:
		// In particular, the legacy mixed expired_or_invalid response cannot
		// safely be assumed transient. Invalid/unsupported/file errors stay hard.
		return err
	}
}
