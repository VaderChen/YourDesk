package p2p

import "encoding/json"

func encodeControl(c Control) ([]byte, error)  { return json.Marshal(c) }
func decodeControl(b []byte, c *Control) error { return json.Unmarshal(b, c) }
