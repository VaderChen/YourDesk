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
	"runtime"
	"syscall"
	"time"
	"yourdesk/internal/authlog"
	"yourdesk/internal/autostart"
	"yourdesk/internal/clientui"
	"yourdesk/internal/deviceid"
	"yourdesk/internal/hardwareprobe"
	"yourdesk/internal/hostguard"
	"yourdesk/internal/hostsession"
	"yourdesk/internal/optimization"
	"yourdesk/internal/peertransport"
	"yourdesk/internal/prelogin"
	"yourdesk/internal/runtimeenv"
	"yourdesk/internal/security"
	"yourdesk/internal/signaling"
	"yourdesk/internal/terminal"
)

func main() {
	if hardwareprobe.HandleHelper(os.Args[1:]) {
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--terminal-window" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := clientui.RunTerminalWindow(ctx); err != nil {
			fatal(err)
		}
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "--prelogin" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		_, err := prelogin.Handle(ctx, os.Args[1:])
		if err != nil {
			fmt.Fprintln(os.Stderr, cliText("YourDesk pre-login service:"), err)
			os.Exit(1)
		}
		return
	}

	signalURL := flag.String("signal", "wss://127.0.0.1:8080/ws", cliText("rendezvous WebSocket URL"))
	room := flag.String("room", "", cliText("pairing room; empty means use hardware UID"))
	serviceHost := flag.Bool("prelogin-host", false, cliText("Host authorized by the system service"))
	parentStdin := flag.Bool("parent-stdin", false, cliText("Stop the Host when the parent app closes its input pipe"))
	explicitSecret := flag.String("secret", "", cliText("Connection password for this CLI Host run; does not change the saved password"))
	secretStdin := flag.Bool("secret-stdin", false, cliText("Read connection password JSON from standard input for automation"))
	display := flag.Int("display", 0, cliText("display index"))
	fps := flag.Int("fps", 12, cliText("maximum capture FPS"))
	quality := flag.Int("quality", 70, cliText("JPEG quality 1-100"))
	printSecret := flag.Bool("print-secret", false, cliText("Print the saved connection password (generate on first use) and exit"))
	printUID := flag.Bool("print-uid", false, cliText("print hardware-derived device UID and exit"))
	codecGoal := flag.String("codec-goal", "balanced", "串流偏好：balanced、low-latency、bandwidth")
	codec := flag.String("codec", "auto", cliText("codec mode: auto, hardware-h264, hardware-hevc, software-jpeg"))
	ui := flag.Bool("ui", false, cliText("Open the Client desktop window"))
	directListen := flag.String("direct-listen", "", cliText("Listen address for direct IP connections over TLS; opt-in"))
	checkSignal := flag.Bool("check-signal", false, cliText("Check the signaling server over TLS and exit"))
	transport := flag.String("transport", "", cliText("Transport mode: empty for native UDP, tailcat for experimental Tailcat"))
	autostartMode := flag.String("autostart", "", cliText("Login startup: on, off or status; uses the saved password"))
	flag.Parse()
	secretProvided := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "secret" {
			secretProvided = true
		}
	})
	if flag.NArg() != 0 {
		fatal(errors.New(cliText("Unexpected positional arguments; use -secret PASSWORD to set the connection password")))
	}
	if secretProvided {
		if *secretStdin || *parentStdin || *serviceHost {
			fatal(errors.New(cliText("-secret cannot be combined with -secret-stdin, -parent-stdin or -prelogin-host")))
		}
		if _, err := security.DecodeSecret(*explicitSecret); err != nil {
			fatal(err)
		}
	}
	if *autostartMode != "" {
		configureAutostart(*autostartMode, *signalURL, *room, *ui, secretProvided || *secretStdin || *parentStdin || *serviceHost)
		return
	}
	headless := runtimeenv.Headless()
	if headless {
		*ui = false
		slog.Info(cliText("No desktop session detected; terminal connections only"))
	}
	if secretProvided && *ui {
		fatal(errors.New(cliText("-secret is only supported for CLI Hosts; change the password in Settings for desktop mode")))
	}
	mode := "host"
	if *ui {
		mode = "ui"
	}
	authlog.Start(clientui.ApplicationVersion(), mode)
	if *checkSignal {
		if err := security.CheckSignalServer(*signalURL); err != nil {
			fatal(err)
		}
		fmt.Println(cliText("TLS server check passed"))
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
		fatal(errors.New(cliText("parent-stdin requires secret-stdin")))
	}
	if *secretStdin {
		if !*parentStdin {
			fmt.Fprintln(os.Stderr, cliText("Waiting for password: enter one JSON line and press Enter, e.g. {\"secret\":\"YOUR_PASSWORD\"}."))
		}
		var input struct {
			Secret    string `json:"secret"`
			Challenge string `json:"challenge"`
		}
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(make([]byte, 1024), 4096)
		if !scanner.Scan() || json.Unmarshal(scanner.Bytes(), &input) != nil {
			fatal(fmt.Errorf("%s", cliText("Could not read connection password JSON from standard input")))
		}
		parentScanner = scanner
		secretText = input.Secret
		challenge = input.Challenge
	} else if secretProvided {
		secretText = *explicitSecret
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
	if !*parentStdin {
		fmt.Fprintln(os.Stderr, cliText("Connection password loaded and validated. Starting Host."))
		if headless && !terminal.Available() {
			fmt.Fprintln(os.Stderr, cliText("Interactive terminal is unavailable for this account. Run as a regular user without root or sudo."))
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *parentStdin {
		// 此管線由 UI 持有；正常退出或崩潰都會關閉，不輪詢可重用的 PID。
		go func() {
			for parentScanner.Scan() {
				var request struct {
					Disconnect   bool                 `json:"disconnect"`
					Optimization *optimization.Policy `json:"optimization"`
				}
				if json.Unmarshal(parentScanner.Bytes(), &request) == nil {
					if request.Disconnect {
						hostsession.Disconnect()
					}
					if request.Optimization != nil {
						optimization.Apply(*request.Optimization)
					}
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
			fatal(errors.New(cliText("Service Host must be started through a private parent pipe")))
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
		if !*parentStdin {
			fmt.Fprintln(os.Stderr, cliText("A Host is already running for this device. Waiting for it to exit."))
		}
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
	ctx = signaling.WithHostCapabilities(ctx, signaling.HostCapabilities{Schema: 1, OS: runtime.GOOS, Arch: runtime.GOARCH, Version: clientui.ApplicationVersion(), Desktop: !headless, Terminal: terminal.Available(), Clipboard: !headless})
	options := hostsession.Options{Headless: headless, Version: clientui.ApplicationVersion(), Transport: peertransport.Mode(*transport), Display: *display, FPS: *fps, Quality: *quality, Codec: *codec, CodecGoal: optimization.Goal(*codecGoal)}
	if *directListen != "" {
		go func() {
			err := signaling.ListenDirect(ctx, *directListen, secret, func(session context.Context, sig *signaling.Client) {
				if err := hostsession.Stream(session, sig, options); err != nil {
					slog.Warn(cliText("Direct IP connection ended"), "error", err)
				}
			})
			if err != nil {
				slog.Error(cliText("Could not start direct IP listener"), "error", err)
			}
		}()
	}
	// 中央服務失聯時持續重試；不影響獨立的 IP 直連入口。
	for ctx.Err() == nil {
		if !*parentStdin {
			fmt.Fprintln(os.Stderr, cliText("Connecting to the signaling server..."))
		}
		sig, err := signaling.Dial(ctx, *signalURL, *room, signaling.RoleHost, secret)
		if err == nil {
			if !*parentStdin {
				fmt.Fprintln(os.Stderr, cliText("Signaling channel established. Waiting for a remote connection."))
			}
			err = hostsession.Stream(ctx, sig, options)
			sig.Close()
		}
		if err != nil {
			if errors.Is(err, signaling.ErrHostOccupied) {
				fmt.Println(`YOURDESK_UI_EVENT {"event":"host-conflict"}`)
			}
			authlog.Event("host-session-ended", map[string]any{"class": authlog.ErrorClass(err)})
			slog.Warn(cliText("Signaling connection ended; retrying shortly"), "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func fatal(err error) { fmt.Fprintln(os.Stderr, "yourdesk-client:", err); os.Exit(1) }

// 登入啟動設定不啟動 Host，也不把單次密碼寫入啟動項目。
func configureAutostart(mode, signalURL, room string, desktop, privateInput bool) {
	if mode != "on" && mode != "off" && mode != "status" {
		fatal(errors.New(cliText("Use -autostart on, off or status")))
	}
	desktop = desktop && !runtimeenv.Headless()
	if mode == "status" {
		s := autostart.Status(desktop)
		if s.Message == autostart.Failed {
			fatal(errors.New(cliText("Could not change login startup; check the installation path, account permissions and user session")))
		}
		if !s.Supported {
			fmt.Println(cliText("Login startup is unavailable"))
		} else if s.Enabled {
			fmt.Println(cliText("Login startup is enabled"))
		} else {
			fmt.Println(cliText("Login startup is disabled"))
		}
		return
	}
	if mode == "on" && privateInput {
		fatal(errors.New(cliText("Login startup uses the saved password; omit temporary password and parent-pipe options")))
	}
	var c autostart.Config
	if mode == "on" {
		var err error
		c, err = autostart.ClientConfig(signalURL, room, desktop)
		if err != nil {
			fatal(err)
		}
		if _, _, err = security.LocalSecret(""); err != nil {
			fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := autostart.Configure(ctx, mode == "on", c); err != nil {
		fatal(errors.New(cliText("Could not change login startup; check the installation path, account permissions and user session")))
	}
	if mode == "on" {
		fmt.Println(cliText("Login startup is enabled"))
	} else {
		fmt.Println(cliText("Login startup is disabled"))
	}
}
