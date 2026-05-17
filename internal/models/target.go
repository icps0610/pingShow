package models

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
)

type TargetInfo struct {
	ID       string  `json:"id,omitempty"`
	Name     string  `json:"name"`
	IP       string  `json:"ip"`
	Last     int64   `json:"last"`
	Min      int64   `json:"min"`
	Max      int64   `json:"max"`
	Avg      int64   `json:"avg"`
	Jtr      int64   `json:"jtr"`
	P95      int64   `json:"p95"`
	TO       int     `json:"to"`
	History  []int64 `json:"history"`
	Sent     int     `json:"-"`
	Recv     int     `json:"-"`
	TotalRTT int64   `json:"-"`
	RawRTTs  []int64 `json:"-"`
}

type LogEntry struct {
	Timestamp string           `json:"timestamp"`
	Metrics   map[string]int64 `json:"metrics"`
}

type TargetConfig struct {
	Port    int           `json:"port"`
	Targets []*TargetInfo `json:"targets"`
}

var (
	Targets    = []*TargetInfo{}
	SystemPort = 80 // 預設使用 80 埠號
	DataMutex  sync.Mutex
	LogChan    = make(chan LogEntry, 100)
)

func generateIDFromIP(ip string) string {
	var runes []rune
	for _, r := range ip {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			runes = append(runes, r)
		} else {
			runes = append(runes, '_')
		}
	}
	return "t_" + string(runes)
}

func LoadTargets() error {
	filename := "target.json"
	var list []*TargetInfo

	// 如果檔案不存在，則建立包含預設 port 與 targets 的設定檔
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		defaultConfig := TargetConfig{
			Port: 80,
			Targets: []*TargetInfo{
				{Name: "local", IP: "192.168.0.1"},
				{Name: "Taiwan", IP: "168.95.1.1"},
				{Name: "Google", IP: "8.8.8.8"},
			},
		}
		data, err := json.MarshalIndent(defaultConfig, "", "    ")
		if err != nil {
			return err
		}
		if err := ioutil.WriteFile(filename, data, 0644); err != nil {
			return err
		}
		list = defaultConfig.Targets
		SystemPort = defaultConfig.Port
	} else {
		// 讀取 target.json
		data, err := ioutil.ReadFile(filename)
		if err != nil {
			return err
		}

		// 優先嘗試解析為新的物件格式（包含 port 與 targets）
		var config TargetConfig
		if err := json.Unmarshal(data, &config); err == nil && len(config.Targets) > 0 {
			if config.Port <= 0 {
				config.Port = 80
			}
			SystemPort = config.Port
			list = config.Targets
		} else {
			// 相容舊格式：若解析為物件失敗，則解析為原本的單純 targets 陣列
			var rawList []*TargetInfo
			if err := json.Unmarshal(data, &rawList); err != nil {
				return err
			}
			SystemPort = 80 // 預設使用 80 埠號
			list = rawList
		}
	}

	// 後處理：
	// 1. 自動產生穩定且唯一的 ID（若 json 中指定了 id 則使用指定的，否則由 索引+IP 動態產生，不寫回 json）
	// 2. 內建初始化 Min 為 9999
	for idx, t := range list {
		if t.ID == "" {
			t.ID = fmt.Sprintf("t_%d_%s", idx+1, generateIDFromIP(t.IP))
		}
		if t.Min == 0 {
			t.Min = 9999
		}
	}

	DataMutex.Lock()
	Targets = list
	DataMutex.Unlock()

	return nil
}

func (t *TargetInfo) AppendHistory(val int64) {
	t.History = append(t.History, val)
	if len(t.History) > 60 {
		t.History = t.History[1:]
	}
}

func (t *TargetInfo) ResetStats() {
	t.Min = 9999
	t.Max = 0
	t.Avg = 0
	t.Jtr = 0
	t.P95 = 0
	t.TO = 0
	t.Sent = 0
	t.Recv = 0
	t.TotalRTT = 0
	t.RawRTTs = nil
}
