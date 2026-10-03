package p2p

import (
	"sync"
	"testing"
)

func TestSupportsCommandSnapshotAndUpdates(t *testing.T) {
	p := &Peer{}
	p.commandsInit()
	p.commands.remote = CommandCapabilities{Version: CommandVersion, Methods: []string{"xfer.read", "xfer.write"}}
	snapshot := p.RemoteCommands()
	snapshot.Methods[0] = "xfer.unknown"
	if !p.SupportsCommand("xfer.read") || p.SupportsCommand("xfer.unknown") {
		t.Fatal("caller mutation of a capability snapshot changed the peer")
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			p.commands.mu.Lock()
			p.commands.remote = CommandCapabilities{Version: CommandVersion, Methods: []string{"xfer.read"}}
			p.commands.mu.Unlock()
		}
	}()
	for i := 0; i < 1000; i++ {
		if !p.SupportsCommand("xfer.read") || p.SupportsCommand("xfer.unknown") {
			t.Fatal("support check lost valid capabilities during update")
		}
	}
	wg.Wait()
	p.commands.remote.Version++
	if p.SupportsCommand("xfer.read") {
		t.Fatal("unsupported command version accepted")
	}
}

func BenchmarkSupportsFileCommand(b *testing.B) {
	p := &Peer{}
	p.commandsInit()
	p.commands.remote = CommandCapabilities{Version: CommandVersion, Methods: []string{
		"capabilities.get", "ping", "session.status", "session.disconnect", "network.probe",
		"xfer.location", "xfer.list", "xfer.stat", "xfer.read", "xfer.begin", "xfer.resume", "xfer.write", "xfer.commit", "xfer.cancel", "xfer.mkdir", "xfer.remove",
	}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !p.SupportsCommand("xfer.write") {
			b.Fatal("file write command unavailable")
		}
	}
}
