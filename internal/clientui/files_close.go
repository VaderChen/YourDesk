package clientui

// All methods run on the window's UI thread. Native close gestures request the
// page's bounded cleanup; process cancellation always has a direct exit path.
type filesCloseGate struct {
	ready, closed bool
	request       func()
	terminate     func()
}

func (g *filesCloseGate) markReady() { g.ready = true }

func (g *filesCloseGate) requestClose() {
	if g.closed {
		return
	}
	if !g.ready {
		// No file queue can exist before its page listeners are ready. A failed
		// navigation must not leave an uncloseable blank native window.
		g.forceClose()
		return
	}
	g.request()
}

func (g *filesCloseGate) forceClose() {
	if !g.closed {
		g.closed = true
		g.terminate()
	}
}
