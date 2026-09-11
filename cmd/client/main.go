package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
	"yourdesk/internal/authlog"
	"yourdesk/internal/clientui"
	"yourdesk/internal/deviceid"
	"yourdesk/internal/hostguard"
	"yourdesk/internal/hostsession"
	"yourdesk/internal/peertransport"
	"yourdesk/internal/prelogin"
	"yourdesk/internal/security"
	"yourdesk/internal/signaling"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--prelogin" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		_, err := prelogin.Handle(ctx, os.Args[1:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "YourDesk 未登入服務：", err)
			os.Exit(1)
		}
		return
	}

	signalURL := flag.String("signal", "wss://127.0.0.1:8080/ws", "rendezvous WebSocket URL")
	room := flag.String("room", "", "pairing room; empty means use hardware UID")
	serviceHost := flag.Bool("prelogin-host", false, "由系統服務授權的 Host")
	parentStdin := flag.Bool("parent-stdin", false, "管理 APP 關閉密碼管線時一併結束 Host")
	secretStdin := flag.Bool("secret-stdin", false, "從標準輸入讀取連線密碼 JSON；供自動化使用")
	display := flag.Int("display", 0, "display index")
	fps := flag.Int("fps", 12, "maximum capture FPS")
	quality := flag.Int("quality", 70, "JPEG quality 1-100")
	printSecret := flag.Bool("print-secret", false, "讀取或首次建立本機連線密碼後輸出並結束")
	printUID := flag.Bool("print-uid", false, "print hardware-derived device UID and exit")
	codec := flag.String("codec", "auto", "codec mode: auto, hardware-h264, hardware-hevc, software-jpeg")
	ui := flag.Bool("ui", false, "開啟內嵌 HTML 的 Client 桌面視窗")
	directListen := flag.String("direct-listen", "", "IP 直連 TLS 監聽位址；必須明確啟用")
	checkSignal := flag.Bool("check-signal", false, "驗證 TLS 伺服器健康狀態後結束")
	transport := flag.String("transport", "", "虛擬傳輸模式：空白為原生 UDP，tailcat 為實驗性 Tailcat")
	flag.Parse()
	mode := "host"
	if *ui {
		mode = "ui"
	}
	authlog.Start(clientui.ApplicationVersion(), mode)
	if *checkSignal {
		if err := security.CheckSignalServer(*signalURL); err != nil {
			fatal(err)
		}
		fmt.Println("TLS 伺服器驗證成功")
		return
	}

	if *printSecret {
		value, _, err := security.LocalSecret("")
		if err != nil {
			fatal(err)
		}
		fmt.Println(value)
		return
	}
	if *ui {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		err := clientui.Run(ctx, clientui.Options{Signal: *signalURL, Room: *room, DirectListen: *directListen,
			HostArgs: []string{"-signal", *signalURL, "-room", *room,
				"-display", fmt.Sprint(*display), "-fps", fmt.Sprint(*fps), "-quality", fmt.Sprint(*quality), "-codec", *codec}})
		if err != nil {
			fatal(err)
		}
		return
	}

	if *printUID {
		identity, err := deviceid.Current()
		if err != nil {
			fatal(err)
		}
		fmt.Println(identity.UID)
		return
	}
	if *room == "" {
		identity, err := deviceid.Current()
		if err != nil {
			fatal(err)
		}
		*room = identity.UID
		fmt.Printf("DEVICE UID: %s (%s)\n", identity.UID, identity.Source)
	}
	secretText := ""
	challenge := ""
	var parentScanner *bufio.Scanner
	if *parentStdin && !*secretStdin {
		fatal(errors.New("parent-stdin 必須搭配 secret-stdin"))
	}
	if *secretStdin {
		var input struct {
			Secret    string `json:"secret"`
			Challenge string `json:"challenge"`
		}
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(make([]byte, 1024), 4096)
		if !scanner.Scan() || json.Unmarshal(scanner.Bytes(), &input) != nil {
			fatal(fmt.Errorf("無法從標準輸入讀取連線密碼"))
		}
		parentScanner = scanner
		secretText = input.Secret
		challenge = input.Challenge
	} else {
		var err error
		secretText, _, err = security.LocalSecret("")
		if err != nil {
			fatal(err)
		}
	}
	authlog.Event("password-loaded", map[string]any{"fromPrivatePipe": *secretStdin})
	secret, err := security.DecodeSecret(secretText)
	if err != nil {
		fatal(err)
	}
	authlog.Event("password-decoded", map[string]any{"valid": err == nil})
	if challenge != "" {
		proof := security.Sign(secret, []byte(challenge))
		data, _ := json.Marshal(map[string]string{"event": "host-password-proof", "proof": proof})
		fmt.Println("YOURDESK_UI_EVENT " + string(data))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *parentStdin {
		// 此管線由 UI 持有；正常退出或崩潰都會關閉，不輪詢可重用的 PID。
		go func() {
			for parentScanner.Scan() {
				var request struct {
					Disconnect bool `json:"disconnect"`
				}
				if json.Unmarshal(parentScanner.Bytes(), &request) == nil && request.Disconnect {
					hostsession.Disconnect()
				}
			}
			authlog.Event("parent-pipe-closed", nil)
			stop()
			// 擷取驅動若無法配合取消，也不允許 Host 永久殘留。
			time.Sleep(5 * time.Second)
			os.Exit(0)
		}()
	}
	if *serviceHost {
		if !*parentStdin || !*secretStdin {
			fatal(errors.New("服務 Host 必須由私有父管線啟動"))
		}
		release, err := prelogin.AcquireHost(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			fatal(err)
		}
		defer release()
	}
	releaseHost, err := hostguard.Acquire(ctx, *room, func() {
		fmt.Println(`YOURDESK_UI_EVENT {"event":"host-conflict","local":true}`)
		authlog.Event("local-host-occupied", nil)
	})
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		fatal(err)
	}
	defer releaseHost()
	fmt.Println(`YOURDESK_UI_EVENT {"event":"host-ready"}`)
	options := hostsession.Options{Version: clientui.ApplicationVersion(), Transport: peertransport.Mode(*transport), Display: *display, FPS: *fps, Quality: *quality, Codec: *codec}
	if *directListen != "" {
		go func() {
			err := signaling.ListenDirect(ctx, *directListen, secret, func(session context.Context, sig *signaling.Client) {
				if err := hostsession.Stream(session, sig, options); err != nil {
					slog.Warn("IP 直連結束", "error", err)
				}
			})
			if err != nil {
				slog.Error("IP 直連無法啟動", "error", err)
			}
		}()
	}
	// 中央服務失聯時持續重試；不影響獨立的 IP 直連入口。
	for ctx.Err() == nil {
		sig, err := signaling.Dial(ctx, *signalURL, *room, signaling.RoleHost, secret)
		if err == nil {
			err = hostsession.Stream(ctx, sig, options)
			sig.Close()
		}
		if err != nil {
			if errors.Is(err, signaling.ErrHostOccupied) {
				fmt.Println(`YOURDESK_UI_EVENT {"event":"host-conflict"}`)
			}
			authlog.Event("host-session-ended", map[string]any{"class": authlog.ErrorClass(err)})
			slog.Warn("中央連線結束，稍後重試", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func fatal(err error) { fmt.Fprintln(os.Stderr, "yourdesk-client:", err); os.Exit(1) }
