module yourdeskandroid/core

go 1.27

require (
	github.com/coder/websocket v1.8.14
	github.com/pion/ice/v4 v4.0.10
	github.com/pion/webrtc/v4 v4.1.6
)

replace github.com/wlynxg/anet => ./third_party/anet

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/pion/datachannel v1.5.10 // indirect
	github.com/pion/dtls/v3 v3.0.7 // indirect
	github.com/pion/interceptor v0.1.41 // indirect
	github.com/pion/logging v0.2.4 // indirect
	github.com/pion/mdns/v2 v2.0.7 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.15 // indirect
	github.com/pion/rtp v1.8.23 // indirect
	github.com/pion/sctp v1.8.40 // indirect
	github.com/pion/sdp/v3 v3.0.16 // indirect
	github.com/pion/srtp/v3 v3.0.8 // indirect
	github.com/pion/stun/v3 v3.0.0 // indirect
	github.com/pion/transport/v3 v3.0.8 // indirect
	github.com/pion/turn/v4 v4.1.1 // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/mobile v0.0.0-20260908204917-8b95e45f8d3e // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/tools v0.50.0 // indirect
)

tool golang.org/x/mobile/cmd/gobind
