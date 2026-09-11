## Windows 重複 Host 問題修正 / Windows duplicate Host fix

### 繁體中文

**已修正 Windows 因裝置 ID 撞號而出現的重複 Host 占用問題。** 舊版使用 CPU ID（ProcessorId），不同電腦可能取得相同值；新版改用 Windows MachineGuid，無效時改用 SMBIOS UUID。

> **Windows 更新後的裝置 ID（序號）會改變。請在更新後查看新 ID，並修改其他電腦已儲存的連線站台；不要繼續使用舊 ID。Mac ID 不受影響。**

新增／編輯站台與快速連線可只輸入 ID 的英數字，程式自動轉大寫、補上 `YD-` 與每四碼的 `-`；也支援貼上完整 ID 及輸入 IP 位址。

### English

**Fixed duplicate Host occupancy caused by colliding Windows device IDs.** Older versions used CPU ProcessorId values, which can be identical on different computers. Windows now uses MachineGuid, with SMBIOS UUID as a fallback.

> **Your Windows device ID will change after updating. Check the new ID and update saved connections on your other computers; stop using the old ID. Mac IDs are unchanged.**

When adding/editing a connection or using Quick Connect, enter only the ID's letters and numbers; uppercase, `YD-` and four-character separators are added automatically. Full IDs and IP addresses can still be entered.

### 日本語

**Windows の装置 ID 重複による Host 占有問題を修正しました。** 旧版で使用していた CPU の ProcessorId は、別のパソコンでも同じ値になる場合があります。新版は Windows MachineGuid を使用し、無効な場合は SMBIOS UUID を使用します。

> **更新後、Windows の装置 ID（識別番号）が変わります。新しい ID を確認し、他のパソコンに保存した接続先を更新してください。古い ID は使用しないでください。Mac の ID は変わりません。**

接続先の追加・編集とクイック接続では、ID の英数字だけを入力すると、大文字化、`YD-`、4 文字ごとの `-` を自動で適用します。完全な ID の貼り付けや IP アドレスの入力にも対応します。

### 한국어

**Windows 장치 ID 충돌로 발생하던 중복 Host 점유 문제를 수정했습니다.** 이전 버전의 CPU ProcessorId는 서로 다른 컴퓨터에서 같을 수 있습니다. 이제 Windows MachineGuid를 사용하고 유효하지 않으면 SMBIOS UUID를 사용합니다.

> **업데이트 후 Windows 장치 ID가 변경됩니다. 새 ID를 확인하고 다른 컴퓨터에 저장한 연결 정보를 수정하세요. 이전 ID는 사용하지 마세요. Mac ID는 변경되지 않습니다.**

연결 추가·편집과 빠른 연결에서 ID의 영문자와 숫자만 입력하면 대문자, `YD-` 및 4자리마다 `-`가 자동 적용됩니다. 전체 ID 붙여넣기와 IP 주소 입력도 지원합니다.

## MCP 與遠端操作 / MCP and remote control

MCP 可供 AI Agent 連線、取得遠端畫面及操作鍵盤滑鼠，也提供斷線、診斷及設定工具。預設關閉；IP 白名單預設開啟，只允許 `127.0.0.1`，並須使用獨立 Token。啟用方式與可直接交給 Agent 的 prompt 見 [README](https://github.com/VaderChen/YourDesk#新增-mcp讓-ai-agent-操作遠端) 與 [MCP 文件](https://github.com/VaderChen/YourDesk/blob/main/docs/MCP.md)。

AI agents can connect to and operate remote computers through MCP. MCP defaults to off and requires a token; the default IP allowlist permits only localhost. Setup and ready-to-use prompts are included in the multilingual README.

## 更新注意事項 / Update notes

- **Windows 更新後請重新取得裝置 ID，更新其他電腦的已儲存站台。** 既有連線密碼不會因這項改動主動重設，Mac 的 ID 生成方式保持不變。
- **After updating Windows, retrieve the new device ID and update saved connections on other computers.** Existing connection passwords are not reset by this change; Mac ID generation is unchanged.
- 請完整結束舊版並啟動新版，避免舊 Host 繼續使用舊 ID。Fully quit the old version and start the new version so the Host uses its new ID.
- 本次修正 CPU 特徵值造成的跨電腦撞號；複製系統映像若保留相同識別資料，仍需處理映像本身的重複識別。This fixes collisions caused by CPU identifiers; cloned system images retaining identical machine identifiers can still collide.
- Mac 對 Mac 的文字、圖片及雙向檔案複製，已由使用者於前一版實機確認可用。The user confirmed Mac-to-Mac text, image and bidirectional file copy on real devices in the preceding release.

## 套件與驗證 / Packages and validation

提供 macOS Apple Silicon DMG、Windows x64 與 ARM64 Installer。macOS App 與 DMG 已完成簽章及 Apple 公證；三平台編譯、JavaScript／JSON／Python 語法檢查通過。本次未新增實機測試；目標 Windows 電腦更新後的新 ID 與連線結果仍待實機確認。

Includes a signed and notarized macOS Apple Silicon DMG and Windows x64/ARM64 installers. Platform builds and syntax checks passed. No additional hardware tests were run in this release pass; the affected Windows computer's new ID and connection still require confirmation after installation.
