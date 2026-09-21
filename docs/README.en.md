# YourDesk

[繁體中文](../README.md) · **English** · [日本語](README.ja.md) · [한국어](README.ko.md)

> **Antivirus notice: some antivirus products currently flag YourDesk. Our empty Go test project also triggered detections, and the official Go FAQ describes possible false positives for Go programs. However, not all detections in the release have been confirmed as false positives. The author will continue investigating and improving the project to address these alerts. Assess the risk before downloading; do not disable antivirus protection.**

YourDesk is a remote desktop app for macOS and Windows. Connect to another computer, organize your saved connections, and copy and paste across platforms with familiar controls.

![YourDesk walkthrough: groups, search, saved sites, appearance and quick connect](../images/yourdesk-demo.gif)

## Features

- **Remote desktop and terminal:** choose remote desktop or an interactive terminal for your task. Terminals use independent windows and can connect to Linux CLI Hosts.
- **Dedicated desktop file transfer:** browse remote files, create folders, permanently delete items, pause and resume transfers, and view progress in a separate window. Available for macOS and Windows; this desktop feature does not include Android or iOS and does not start screen capture or a shell.
- **Remote control through MCP AI agents:** AI agents can connect through MCP, operate the desktop or terminal, query site capabilities and disconnect when finished.
- **Hardware-accelerated encoding and decoding:** supports H.264/HEVC on macOS and H.264 on Windows, selected automatically according to both devices' capabilities. Falls back when unavailable; the toolbar shows the actual encoding and decoding status.
- **Cross-platform control:** use your keyboard, mouse and common shortcuts between Mac and Windows.
- **Quick connections:** enter a remote ID or open a saved connection. Group, reorder, import and export saved connections.
- **Copy and paste:** transfer text, images, files and folders using shortcuts or the system context menu. File and folder batches support up to 2 GiB.
- **Flexible viewing:** switch remote monitors using up to four numbered buttons centered in the toolbar, fit the desktop to your window, view at original size or use fullscreen.
- **Screen enhancement:** choose automatic adjustment, smoother motion or lower network load, with adjustable FPS and bitrate.
- **Live status:** see FPS, transfer rates and enhancement status, with instant hover tips.
- **Personal preferences:** light and dark themes, four interface languages, saved viewing settings and update reminders.

## MCP for AI agents

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
| Windows x64 | x64 installer / portable ZIP (extract all files and run YourDesk.exe) |
| Windows on ARM | ARM64 installer |
| Linux x64 / arm64 | CLI Host ZIP (no desktop / REMOTE) |

1. Install and open YourDesk on both computers.
2. Enter the remote ID or select a saved connection.
3. Enter the connection password to start.

For remote desktop control on macOS, allow Screen Recording and Accessibility permissions. Dedicated file transfer does not start screen capture or keyboard/mouse control; normal filesystem permissions still apply. When receiving remote files via the clipboard, allow Network Volumes access if prompted. If previously denied, enable it for YourDesk under System Settings → Privacy & Security → Files and Folders, then copy the files again. Windows requires WebView2 Runtime. Intel Macs and Linux desktop packages are currently unsupported. Direct IP connections are also available after enabling them in the remote computer's security settings. Connections are encrypted and require networks that permit direct connectivity.

This README describes the capabilities of the project source. For the features included in a published package, refer to that Release's notes.

## Enhancement and interpolation

Screen enhancement reduces the transmitted image size and enlarges it locally. User feedback shows greater benefits on higher-resolution remote desktops, especially 1440p and 4K. Choose FSR or Core ML according to device support.

Experimental **2× interpolation** can improve motion smoothness on Apple Silicon with macOS 13 or later. Choose Apple low-latency interpolation or RIFE; Auto prefers Apple on supported Macs running macOS 27 or later. Enhancement and interpolation both default to off. Results depend on your devices and network; fast motion and small text may show artifacts.

## Everyday use and updates

Copy on one computer and paste on the other. The operating system handles file and folder paste progress. Closing Remote Display returns to the main window; closing the main window leaves YourDesk in the tray. Use the tray's Quit action or Cmd+Q on macOS to exit completely.

Check for updates in About. After downloading, a red 10-second countdown lets you cancel or update immediately. When it ends, the app quits, installs the update and restarts automatically. If automatic installation fails, follow the instructions to install manually. Keep both computers updated for full functionality.

## Dedicated file transfer

1. Close the site's active CMD or GUI connection, then choose the file-transfer icon between CMD and GUI and enter the connection password. Both computers must support independent file-transfer sessions; older Hosts are rejected rather than opened as desktop sessions.
2. Browse the remote user's home folder. Drag local files or folders from Finder / File Explorer into the upload area, or choose local files. Existing files are not overwritten and existing folders are not merged.
3. Create folders or select an item to delete it. Deletion is permanent, not a move to Trash / Recycle Bin; deleting a folder also requires typing its name and confirming removal of its contents. For downloads, select one remote file, wait until it finishes downloading, then drag it from the native area at the bottom into Finder / File Explorer. Completed downloads remain in the system Downloads folder under `YourDesk`.
4. Pause or resume uploads and downloads while monitoring the percentage, transferred / total bytes, speed and estimated time remaining. After a disconnection, keep the transfer window open, reconnect to the same site from the main window, then choose Resume manually. Resume requires both apps and this window to remain open; it does not survive an app or system restart. If cancellation cannot be confirmed when closing, the window stays open so you can retry or reconnect.

This workflow is for the macOS / Windows desktop clients, not Android / iOS. See the [file-transfer guide](FILE-TRANSFER.md) for size and queue limits, cancellation behavior, and platform validation status. Windows native operation and actual Finder / File Explorer drag-and-drop still require platform-specific verification; browser smoke tests do not replace it.

## More information

[File transfer](FILE-TRANSFER.md) · [Streaming settings](STREAMING-CONTROLS.md) · [Build guide](BUILD.md) · [Architecture](ARCHITECTURE.md) · [Enhancement models](QUICKSRNET.md) · [Interpolation](FRAME-INTERPOLATION.md)

Thanks to the authors of FSR, QuickSRNet/SESR and RIFE. Full [FSR](FSR-LICENSE.txt), [super-resolution](../internal/superres/MODEL_LICENSE.txt) and [RIFE](../internal/frameinterp/MODEL_LICENSE.txt) licenses are included with the packages.

**This release fixes remote-file paste on a local Mac; the user confirmed that pasting works again.**

## Remote terminal

Choose GUI (Desktop) or CMD (Terminal), or double-click a site to choose between those two modes. Dedicated file transfer uses its own icon between CMD and GUI, not this double-click menu. Independent terminal windows support multiple sites; unsupported older peers show an update prompt. Run Linux as a regular user. CLI supports `-secret "password"`; `LC_ALL → LC_MESSAGES → LANG` selects Traditional Chinese, English, Japanese or Korean, with English fallback. See the ZIP README for usage.

## Support development

If YourDesk is useful to you, consider buying me a coffee to support continued development.

[☕ Buy Me a Coffee](https://buymeacoffee.com/vaderchen)

## License

Copyright (C) 2026 VaderChen.

This project uses a custom [source-available, no-commercial-sales license](../LICENSE.en.md). Non-sale use, modification and free sharing, including internal business use, are permitted. Selling, paid hosting/SaaS, paid services and integration into paid products are prohibited; a separate license may be discussed for sales or integration into paid products or services, but this license governs until a separate written agreement is reached. Third-party components retain their respective licenses. This is not an OSI open-source license. The Traditional Chinese LICENSE is authoritative. Ordinary employee wages are not sales. Advertising, affiliate commissions and data monetization through provision of the Software are also restricted. Free and internal integrations must comply with the complete license.
