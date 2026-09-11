//go:build !winpe

package peertransport

const RestrictedBuild = false

func initialBackends() map[Mode]Backend { return map[Mode]Backend{Tailcat: tailcatBackend{}} }
func UnavailableReason(mode Mode) string {
	if Supports(mode) {
		return ""
	}
	return "此版本未提供此傳輸後端"
}
