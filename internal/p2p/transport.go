package p2p

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
	"yourdesk/internal/peertransport"
	"yourdesk/internal/signaling"
)

type transportDescription struct {
	webrtc.SessionDescription
	NegotiationVersion int                  `json:"transportNegotiation,omitempty"`
	Capabilities       []peertransport.Mode `json:"transportCapabilities,omitempty"`
	Transport          *transportOffer      `json:"transport,omitempty"`
}
type transportOffer struct {
	Version int                `json:"version"`
	Mode    peertransport.Mode `json:"mode"`
	Secret  string             `json:"secret,omitempty"`
}

func newTransportPC(ctx context.Context, signal *signaling.Client, mode peertransport.Mode, host bool, offer string) (*webrtc.PeerConnection, peertransport.Link, error) {
	link, err := peertransport.Open(ctx, mode, host, offer)
	if err != nil {
		return nil, nil, err
	}
	if link == nil {
		pc, err := webrtc.NewPeerConnection(connectionConfig(signal))
		return pc, nil, err
	}
	settings := webrtc.SettingEngine{}
	settings.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	settings.SetIncludeLoopbackCandidate(true)
	settings.SetIPFilter(func(ip net.IP) bool { return ip.IsLoopback() })
	settings.SetReceiveMTU(65535)
	mux := ice.NewUDPMuxDefault(ice.UDPMuxParams{UDPConn: link})
	settings.SetICEUDPMux(mux)
	pc, err := webrtc.NewAPI(webrtc.WithSettingEngine(settings)).NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		mux.Close()
		return nil, nil, err
	}
	return pc, link, nil
}
func (p *Peer) bindTransport(link peertransport.Link, mode peertransport.Mode) {
	p.transportMode = mode
	if link != nil {
		go func() { <-p.Done(); link.Close() }()
	}
}
func validateTransport(description transportDescription, mode peertransport.Mode) error {
	if mode == peertransport.Native && description.Transport == nil {
		return nil
	}
	if description.Transport == nil || description.Transport.Version != 1 || description.Transport.Mode != mode || mode == peertransport.Native {
		return errors.New("兩端傳輸模式不同或版本不支援，請確認兩端的 Tailcat 模式設定")
	}
	return nil
}

// TransportMode 回報本次工作階段實際選用的虛擬傳輸，與後續偏好變更分開。
func (p *Peer) TransportMode() string {
	if p.transportMode == peertransport.Native {
		return "native"
	}
	return string(p.transportMode)
}

// 使用既有可轉送的 error envelope 承載升級要求，避免舊 Server 把它當成
// 最終 answer 清除待機 offer。Receive 已驗證密碼簽章、角色、room 與時效。
type transportRequestEnvelope struct {
	Request *transportRequest `json:"transportRequest"`
}
type transportRequest struct {
	Version   int                `json:"version"`
	Mode      peertransport.Mode `json:"mode"`
	OfferHash string             `json:"offerHash"`
}

func offerHash(sdp string) string {
	hash := sha256.Sum256([]byte(sdp))
	return hex.EncodeToString(hash[:])
}
func requestedTransport(e signaling.Envelope, description transportDescription) (peertransport.Mode, bool) {
	if e.Kind != signaling.KindError || description.Transport != nil {
		return peertransport.Native, false
	}
	var message transportRequestEnvelope
	if e.DecodePayload(&message) != nil || message.Request == nil {
		return peertransport.Native, false
	}
	request := message.Request
	if request.Version != 1 || request.Mode == peertransport.Native || !peertransport.Supports(request.Mode) || request.OfferHash != offerHash(description.SDP) {
		return peertransport.Native, false
	}
	return request.Mode, true
}
func receiveTransportDescription(ctx context.Context, signal *signaling.Client, preferred peertransport.Mode) (transportDescription, peertransport.Mode, error) {
	var description transportDescription
	if !peertransport.Supports(preferred) {
		return description, preferred, errors.New("不支援的傳輸模式")
	}
	e, err := signal.Receive(ctx)
	if err != nil {
		return description, preferred, err
	}
	if e.Kind != signaling.KindOffer {
		return description, preferred, errors.New("未收到有效的傳輸配對資料")
	}
	if err = e.DecodePayload(&description); err != nil {
		return description, preferred, err
	}
	// Host 已選用虛擬通道時，未開啟偏好的 Viewer 自動配合。
	if description.Transport != nil {
		selected := description.Transport.Mode
		if !peertransport.Supports(selected) || (preferred != peertransport.Native && preferred != selected) {
			return description, preferred, errors.New("對端不支援所需的傳輸模式，請更新兩端程式")
		}
		return description, selected, validateTransport(description, selected)
	}
	if preferred == peertransport.Native {
		return description, preferred, nil
	}
	supported := false
	for _, mode := range description.Capabilities {
		if mode == preferred {
			supported = true
		}
	}
	if description.NegotiationVersion != 1 || !supported {
		return description, preferred, errors.New("對端不支援單端傳輸協商，請更新遠端程式")
	}
	previous := offerHash(description.SDP)
	request := transportRequestEnvelope{Request: &transportRequest{Version: 1, Mode: preferred, OfferHash: previous}}
	waiting, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	if err = signal.Send(waiting, signaling.KindError, request); err != nil {
		return description, preferred, err
	}
	// 忽略可能已排入佇列的舊待機 offer，只接受新的已驗證 offer；不重複提出要求。
	for {
		e, err = signal.Receive(waiting)
		if err != nil {
			return description, preferred, err
		}
		if e.Kind != signaling.KindOffer {
			return description, preferred, errors.New("傳輸協商失敗")
		}
		if err = e.DecodePayload(&description); err != nil {
			return description, preferred, err
		}
		if offerHash(description.SDP) == previous {
			continue
		}
		return description, preferred, validateTransport(description, preferred)
	}
}
