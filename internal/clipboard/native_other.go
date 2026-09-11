//go:build (!darwin && !windows) || (darwin && !cgo)

package clipboard

import "fmt"

func nativeRevision() int64         { return 0 }
func readContent() (content, error) { return content{}, nil }
func writeContent(content) error    { return fmt.Errorf("此平台尚未支援剪貼簿同步") }

func readLegacyText() (content, error) { return content{}, nil }
