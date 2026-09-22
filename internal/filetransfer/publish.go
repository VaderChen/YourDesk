package filetransfer

import "os"

// PublishFile 將已核對的暫存檔提交至同一個目錄，不覆寫既有檔案。
// 上傳及本機下載共用各平台的安全更名實作。
func PublishFile(root *os.Root, temp, name string, expected os.FileInfo) error {
	return commitNoReplace(root, temp, name, expected)
}
