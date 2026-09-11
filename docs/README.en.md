# YourDesk

[繁體中文](../README.md) · **English** · [日本語](README.ja.md) · [한국어](README.ko.md)

YourDesk is a remote desktop app for macOS and Windows. Connect to another computer, organize your saved connections, and copy and paste across platforms with familiar controls.

![YourDesk interface](../images/cap001.png)

## Features

This release adds a tray action and red button to disconnect incoming sessions, and allows listed MCP IPs to connect without a token. macOS pre-login access is experimental, off by default and not yet verified on hardware; see the [release notes](RELEASE-1.26.0911-build-1012.md) for limitations and changes.

- **Hardware-accelerated encoding and decoding:** supports H.264/HEVC on macOS and H.264 on Windows, selected automatically according to both devices' capabilities. Falls back when unavailable; the toolbar shows the actual encoding and decoding status.
- **Cross-platform control:** use your keyboard, mouse and common shortcuts between Mac and Windows.
- **Quick connections:** enter a remote ID or open a saved connection. Group, reorder, import and export saved connections.
- **Copy and paste:** transfer text, images, files and folders using shortcuts or the system context menu. File and folder batches support up to 2 GiB.
- **Flexible viewing:** switch remote monitors using up to four numbered buttons centered in the toolbar, fit the desktop to your window, view at original size or use fullscreen.
- **Screen enhancement:** choose automatic adjustment, smoother motion or lower network load, with adjustable FPS and bitrate.
- **Live status:** see FPS, transfer rates and enhancement status, with instant hover tips.
- **Personal preferences:** light and dark themes, four interface languages, saved viewing settings and update reminders.

## New: MCP for AI agents

AI agents can use MCP to connect to remote computers, capture screenshots, control the keyboard and mouse, disconnect, run connection diagnostics and manage settings.

![Enable in Settings → MCP Settings; off by default](../images/mcp-enable-notice.en.svg)

The IP allowlist defaults to on with only `127.0.0.1`: listed IPs need no token; other IPs are blocked. When disabled, all sources require a token.

Local endpoint: `http://127.0.0.1:12345/mcp`. Remote windows are hidden by default, with a colored tray indicator and a menu to show or hide them. Normal connection authentication and OS permissions still apply. See the [MCP guide](MCP.md).

### Prompt to let your AI agent connect remotely

<table>
<tr><td><strong>Use YourDesk MCP (<code>http://127.0.0.1:12345/mcp</code>) without a token when allowlisted; if Bearer authentication is required, read <code>mcp-token</code> from the local YourDesk configuration directory without displaying it. Connect to “site name”, complete “task”, then disconnect.</strong></td></tr>
</table>

## Download and start

Download the latest package from [GitHub Releases](https://github.com/VaderChen/YourDesk/releases/latest).

| Platform | Package |
| --- | --- |
| macOS Apple Silicon | Signed and Apple-notarized DMG |
| Windows x64 | x64 installer |
| Windows on ARM | ARM64 installer |

1. Install and open YourDesk on both computers.
2. Enter the remote ID or select a saved connection.
3. Enter the connection password to start.

On macOS, allow Screen Recording and Accessibility permissions. Windows requires WebView2 Runtime. Intel Macs and Linux desktop packages are currently unsupported. Direct IP connections are also available after enabling them in the remote computer's security settings. Connections are encrypted and require networks that permit direct connectivity.

## Enhancement and interpolation

Screen enhancement reduces the transmitted image size and enlarges it locally. User feedback shows greater benefits on higher-resolution remote desktops, especially 1440p and 4K. Choose FSR or Core ML according to device support.

Experimental **2× interpolation** can improve motion smoothness on Apple Silicon with macOS 13 or later. Choose Apple low-latency interpolation or RIFE; Auto prefers Apple on supported Macs running macOS 26 or later. Enhancement and interpolation both default to off. Results depend on your devices and network; fast motion and small text may show artifacts.

## Everyday use and updates

Copy on one computer and paste on the other. The operating system handles file and folder paste progress. Closing Remote Display returns to the main window; closing the main window leaves YourDesk in the tray. Use the tray's Quit action or Cmd+Q on macOS to exit completely.

Check for updates in About. After downloading, a red 10-second countdown lets you cancel or update immediately. When it ends, the app quits, installs the update and restarts automatically. If automatic installation fails, follow the instructions to install manually. Keep both computers updated for full functionality.

## More information

[Streaming settings](STREAMING-CONTROLS.md) · [Build guide](BUILD.md) · [Architecture](ARCHITECTURE.md) · [Enhancement models](QUICKSRNET.md) · [Interpolation](FRAME-INTERPOLATION.md)

Thanks to the authors of FSR, QuickSRNet/SESR and RIFE. Full [FSR](FSR-LICENSE.txt), [super-resolution](../internal/superres/MODEL_LICENSE.txt) and [RIFE](../internal/frameinterp/MODEL_LICENSE.txt) licenses are included with the packages.

**Clipboard synchronization improved: the user confirmed that Mac-to-Mac text, image and bidirectional file copy work on real devices.**

## Windows device ID update and duplicate Host fix

**Fixed duplicate Host occupancy caused by colliding Windows device IDs.** Older versions used CPU ProcessorId values, which can be identical on different computers. Windows now uses MachineGuid, with SMBIOS UUID as a fallback.

> **Your Windows device ID will change after updating. Check the new ID and update saved connections on your other computers; stop using the old ID. Mac IDs are unchanged.**

When adding/editing a connection or using Quick Connect, enter only the ID's letters and numbers; uppercase, `YD-` and four-character separators are added automatically. Full IDs and IP addresses can still be entered.
