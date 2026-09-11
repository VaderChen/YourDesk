//go:build winpe

package peertransport

// 在受限映像相容性完成驗證前，不編入 Tailcat 及其相依；
// UI、CLI 與 P2P 自動協商共用空的虛擬後端清單，原生 P2P 保持可用。
const RestrictedBuild = true

func initialBackends() map[Mode]Backend { return map[Mode]Backend{} }
func UnavailableReason(mode Mode) string {
	if Supports(mode) {
		return ""
	}
	return "WinPE 尚未確認 Tailcat 相容性，此實驗版本未包含虛擬傳輸後端。"
}
