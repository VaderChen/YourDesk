//go:build windows

package clientui

func fileDownloadsDirectory() (string, error) {
	return `C:\`, nil
}
