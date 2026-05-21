package models

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type TargetInfo struct {
	ID       string  `json:"id,omitempty"`
	Name     string  `json:"name"`
	IP       string  `json:"ip"`
	Disabled bool    `json:"disabled,omitempty"`
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
	Port       int           `json:"port"`
	Interval   int           `json:"interval"`
	YMax       int           `json:"ymax"`
	DayRange   int           `json:"day_range"`
	MonthRange int           `json:"month_range"`
	Timezone   string        `json:"timezone"`
	Targets    []*TargetInfo `json:"targets"`
}

var (
	Targets          = []*TargetInfo{}
	SystemPort       = 80 // 預設使用 80 埠號
	SystemInterval   = 1  // 預設 ping 間隔 1 秒
	SystemYMax       = 50 // 預設 y軸 最大值
	SystemDayRange   = 7
	SystemMonthRange = 3
	SystemLocation   = time.Local // 預設使用系統時區
	SystemTimezone   = ""         // 空字串代表使用系統時區
	DataMutex        sync.Mutex
	LogChan          = make(chan LogEntry, 100)
	AppDir           string
)

func init() {
	exe, err := os.Executable()
	if err == nil {
		AppDir = filepath.Dir(exe)
	} else {
		AppDir = "."
	}
}

func LoadTargets() error {
	filename := filepath.Join(AppDir, "target.json")
	var list []*TargetInfo

	// 如果檔案不存在，則建立包含預設 port 與 targets 的設定檔
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		defaultConfig := struct {
			Port       int `json:"port"`
			Interval   int `json:"interval"`
			YMax       int `json:"ymax"`
			DayRange   int `json:"day_range"`
			MonthRange int `json:"month_range"`
			Targets    []struct {
				Name string `json:"name"`
				IP   string `json:"ip"`
			} `json:"targets"`
		}{
			Port:       80,
			Interval:   1,
			YMax:       50,
			DayRange:   7,
			MonthRange: 3,
			Targets: []struct {
				Name string `json:"name"`
				IP   string `json:"ip"`
			}{
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
		
		for _, dt := range defaultConfig.Targets {
			list = append(list, &TargetInfo{Name: dt.Name, IP: dt.IP})
		}
		SystemPort = defaultConfig.Port
		SystemInterval = defaultConfig.Interval
		SystemYMax = defaultConfig.YMax
		SystemDayRange = defaultConfig.DayRange
		SystemMonthRange = defaultConfig.MonthRange
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
			if config.Interval <= 0 {
				config.Interval = 1
			}
			if config.YMax <= 0 {
				config.YMax = 50
			}
			if config.DayRange <= 0 {
				config.DayRange = 7
			}
			if config.MonthRange <= 0 {
				config.MonthRange = 3
			}
			SystemPort = config.Port
			SystemInterval = config.Interval
			SystemYMax = config.YMax
			SystemDayRange = config.DayRange
			SystemMonthRange = config.MonthRange
			if config.Timezone != "" {
				if loc, err := time.LoadLocation(config.Timezone); err == nil {
					SystemLocation = loc
					SystemTimezone = config.Timezone
				}
			}
			list = config.Targets
		} else {
			// 相容舊格式：若解析為物件失敗，則解析為原本的單純 targets 陣列
			var rawList []*TargetInfo
			if err := json.Unmarshal(data, &rawList); err != nil {
				return err
			}
			SystemPort = 80 // 預設使用 80 埠號
			SystemInterval = 1
			SystemYMax = 50
			SystemDayRange = 7
			SystemMonthRange = 3
			list = rawList
		}
	}

	// 後處理與去重：
	// 1. 若發生重複 IP，直接合併去重
	// 2. ID 直接使用 IP 字串本身
	// 3. 內建初始化 Min 為 9999
	var dedupList []*TargetInfo
	seenIP := make(map[string]bool)
	for _, t := range list {
		if seenIP[t.IP] {
			continue
		}
		seenIP[t.IP] = true
		t.ID = t.IP
		if t.Min == 0 {
			t.Min = 9999
		}
		dedupList = append(dedupList, t)
	}

	DataMutex.Lock()
	Targets = dedupList
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

func IsTargetActive(ip string) bool {
	DataMutex.Lock()
	defer DataMutex.Unlock()
	for _, t := range Targets {
		if t.IP == ip {
			return true
		}
	}
	return false
}

func SaveTargets() error {
	filename := filepath.Join(AppDir, "target.json")
	
	DataMutex.Lock()
	cleanTargets := make([]struct {
		Name     string `json:"name"`
		IP       string `json:"ip"`
		Disabled bool   `json:"disabled,omitempty"`
	}, len(Targets))
	for i, t := range Targets {
		cleanTargets[i].Name = t.Name
		cleanTargets[i].IP = t.IP
		cleanTargets[i].Disabled = t.Disabled
	}
	
	cleanConfig := struct {
		Port       int    `json:"port"`
		Interval   int    `json:"interval"`
		YMax       int    `json:"ymax"`
		DayRange   int    `json:"day_range"`
		MonthRange int    `json:"month_range"`
		Timezone   string `json:"timezone"`
		Targets    []struct {
			Name     string `json:"name"`
			IP       string `json:"ip"`
			Disabled bool   `json:"disabled,omitempty"`
		} `json:"targets"`
	}{
		Port:       SystemPort,
		Interval:   SystemInterval,
		YMax:       SystemYMax,
		DayRange:   SystemDayRange,
		MonthRange: SystemMonthRange,
		Timezone:   SystemTimezone,
		Targets:    cleanTargets,
	}
	DataMutex.Unlock()

	data, err := json.MarshalIndent(cleanConfig, "", "    ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filename, data, 0644)
}

