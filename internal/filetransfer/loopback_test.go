package filetransfer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"yourdesk/internal/p2p"
)

// This exercises the xfer JSON contract over real local SCTP DataChannels. It is
// not a signaling/authentication or native-window end-to-end test. No STUN, real
// remote device, production home directory, or application setting is touched.
func TestLoopbackDataChannelTransferContract(t *testing.T) {
	s, home := testSession(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	settings := webrtc.SettingEngine{}
	settings.SetIncludeLoopbackCandidate(true)
	settings.SetIPFilter(func(ip net.IP) bool { return ip.IsLoopback() })
	settings.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	api := webrtc.NewAPI(webrtc.WithSettingEngine(settings))
	host, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	viewer, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	wireErrors := make(chan error, 8)
	report := func(err error) {
		select {
		case wireErrors <- err:
		default:
		}
	}
	host.OnDataChannel(func(dc *webrtc.DataChannel) {
		dc.OnMessage(func(m webrtc.DataChannelMessage) {
			var control p2p.Control
			if err := json.Unmarshal(m.Data, &control); err != nil {
				report(err)
				return
			}
			request := control.CommandRequest
			if request == nil || request.Version != p2p.CommandVersion || len(request.Params) > 8192 || !strings.HasPrefix(request.Method, "xfer.") {
				report(fmt.Errorf("invalid test command"))
				return
			}
			result, err := s.command(ctx, strings.TrimPrefix(request.Method, "xfer."), request.Params)
			response := p2p.CommandResponse{Version: p2p.CommandVersion, ID: request.ID}
			if err != nil {
				response.Code = "transfer_error"
				response.Error = err.Error()
			} else {
				response.Result, err = json.Marshal(result)
				if err != nil || len(response.Result) > 16384 {
					report(fmt.Errorf("oversized result"))
					return
				}
			}
			data, err := json.Marshal(p2p.Control{Type: "command-response", CommandResponse: &response})
			if err == nil {
				err = dc.Send(data)
			}
			if err != nil {
				report(err)
			}
		})
	})
	dc, err := viewer.CreateDataChannel(p2p.ControlChannel, nil)
	if err != nil {
		t.Fatal(err)
	}
	opened := make(chan struct{})
	dc.OnOpen(func() { close(opened) })
	responses := make(chan p2p.CommandResponse, 4)
	dc.OnMessage(func(m webrtc.DataChannelMessage) {
		var control p2p.Control
		if err := json.Unmarshal(m.Data, &control); err != nil {
			report(err)
			return
		}
		if control.CommandResponse == nil {
			report(fmt.Errorf("missing response"))
			return
		}
		select {
		case responses <- *control.CommandResponse:
		case <-ctx.Done():
		}
	})
	offer, err := viewer.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	gather := webrtc.GatheringCompletePromise(viewer)
	if err = viewer.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gather:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err = host.SetRemoteDescription(*viewer.LocalDescription()); err != nil {
		t.Fatal(err)
	}
	answer, err := host.CreateAnswer(nil)
	if err != nil {
		t.Fatal(err)
	}
	gather = webrtc.GatheringCompletePromise(host)
	if err = host.SetLocalDescription(answer); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gather:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err = viewer.SetRemoteDescription(*host.LocalDescription()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-opened:
	case err := <-wireErrors:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var sequence uint64
	invoke := func(method string, in any, out any, wantError bool) {
		t.Helper()
		params, err := json.Marshal(in)
		if err != nil || len(params) > 8192 {
			t.Fatalf("bad test params: %v", err)
		}
		sequence++
		request := p2p.CommandRequest{Version: p2p.CommandVersion, ID: sequence, Method: "xfer." + method, Expires: time.Now().Add(5 * time.Second).UnixMilli(), Params: params}
		wire, err := json.Marshal(p2p.Control{Type: "command-request", CommandRequest: &request})
		if err != nil {
			t.Fatal(err)
		}
		if err = dc.Send(wire); err != nil {
			t.Fatal(err)
		}
		select {
		case response := <-responses:
			if response.ID != sequence || (response.Code != "") != wantError {
				t.Fatalf("unexpected %s response: %+v", method, response)
			}
			if out != nil && !wantError {
				if err := json.Unmarshal(response.Result, out); err != nil {
					t.Fatal(err)
				}
			}
		case err := <-wireErrors:
			t.Fatal(err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	name := "本機測試.bin"
	payload := bytes.Repeat([]byte{0, 127, 128, 255}, 16*1024)
	var begun struct {
		ID          string `json:"id"`
		ResumeToken string `json:"resumeToken"`
	}
	invoke("begin", map[string]any{"path": name, "size": len(payload)}, &begun, false)
	for offset := 0; offset < len(payload); offset += ChunkSize {
		var response struct {
			NextOffset int `json:"nextOffset"`
		}
		invoke("write", map[string]any{"id": begun.ID, "offset": offset, "data": base64.StdEncoding.EncodeToString(payload[offset : offset+ChunkSize])}, &response, false)
		if response.NextOffset != offset+ChunkSize {
			t.Fatal("wrong wire offset")
		}
	}
	var resumed struct {
		State      string `json:"state"`
		NextOffset int64  `json:"nextOffset"`
	}
	invoke("resume", resumeArgs(begun.ID, begun.ResumeToken, name, int64(len(payload))), &resumed, false)
	if resumed.State != "uploading" || resumed.NextOffset != int64(len(payload)) {
		t.Fatal("wrong resumed wire offset")
	}
	invoke("commit", map[string]string{"id": begun.ID}, nil, false)
	invoke("resume", resumeArgs(begun.ID, begun.ResumeToken, name, int64(len(payload))), &resumed, false)
	if resumed.State != "complete" {
		t.Fatal("missing completion receipt over DataChannel")
	}
	var info Entry
	invoke("stat", map[string]string{"path": name}, &info, false)
	var downloaded []byte
	for offset := int64(0); offset < info.Size; {
		var result struct {
			Data       string `json:"data"`
			NextOffset int64  `json:"nextOffset"`
		}
		invoke("read", map[string]any{"path": name, "offset": offset, "length": ChunkSize, "modified": info.Modified, "size": info.Size}, &result, false)
		data, err := base64.StdEncoding.DecodeString(result.Data)
		if err != nil {
			t.Fatal(err)
		}
		if result.NextOffset <= offset {
			t.Fatal("read did not advance")
		}
		downloaded = append(downloaded, data...)
		offset = result.NextOffset
	}
	if !bytes.Equal(downloaded, payload) {
		t.Fatal("DataChannel transfer corrupted data")
	}
	got, err := os.ReadFile(filepath.Join(home, name))
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("committed data differs: %v", err)
	}
	invoke("begin", map[string]any{"path": name, "size": 0}, nil, true)
	invoke("list", map[string]string{"path": "../"}, nil, true)
	var removed removeResult
	invoke("remove", map[string]any{"path": name, "directory": false, "size": info.Size, "modified": info.Modified, "confirm": name, "recursive": false}, &removed, false)
	if !removed.OK || removed.RemovedCount != 1 {
		t.Fatal("invalid remove wire result")
	}
	if _, err := os.Stat(filepath.Join(home, name)); !os.IsNotExist(err) {
		t.Fatal("loopback removal did not finish")
	}
}
