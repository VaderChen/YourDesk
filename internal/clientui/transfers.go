package clientui

import "yourdesk/internal/clipboard"

type transferView struct {
	clipboard.Progress
	Key  string `json:"key"`
	Site string `json:"site"`
}

// 呼叫端持有 server.mu；一筆慢傳輸只喚回視窗一次。
func (s *server) acceptTransferProgress(p *process, values []clipboard.Progress) {
	if len(values) > 8 {
		return
	}
	previous := map[string]bool{}
	for _, v := range p.transfers {
		previous[v.Direction+v.ID] = true
	}
	notify := false
	for _, v := range values {
		if len(v.ID) != 32 || (v.Direction != "send" && v.Direction != "receive") || v.Bytes < 0 || v.Total < 0 || v.Bytes > v.Total {
			return
		}
		if !previous[v.Direction+v.ID] {
			notify = true
		}
	}
	p.transfers = values
	if notify {
		select {
		case s.transferNotice <- struct{}{}:
		default:
		}
	}
}
func (s *server) transferSnapshot() []transferView {
	result := []transferView{}
	for key, p := range s.children {
		name := p.room
		for _, site := range s.library.Sites {
			if site.ID == p.siteID {
				name = site.Name
				break
			}
		}
		for _, v := range p.transfers {
			result = append(result, transferView{Progress: v, Key: key + ":" + v.Direction + ":" + v.ID, Site: name})
		}
	}
	return result
}
