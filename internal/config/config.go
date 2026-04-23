package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
	rest.RestConf

	DB struct {
		DSN         string
		MaxOpenConn int `json:",default=100"`
		MaxIdleConn int `json:",default=20"`
	}

	Redis struct {
		Addr     string
		Password string `json:",optional"`
		DB       int    `json:",default=0"`
	}

	JWT struct {
		Secret     string
		ExpireHour int `json:",default=24"`
	}

	DingTalk struct {
		AppKey    string
		AppSecret string
		AgentId   int64  `json:",optional"`
		CorpId    string `json:",optional"`
		Sheet     struct {
			BaseId     string
			OperatorId string
			Sheets     struct {
				Hotels           string `json:",default=Ca5Pz1k"`     // 酒店基础信息表（提供竞对关联）
				Venues           string `json:",default=4zrnGvw"`     // 酒店会议室信息表
				DailyData        string `json:",default=smfseyx"`     // Daily Data Input
				DailyDataRevenue string `json:",default=2GiUIZq"`     // Daily Data Input (Revenue)
				CityEvents       string `json:",default=Cn6Evow"`     // City Event
				HotelFacilities  string `json:",default=hAi1ytw"`     // 酒店设施表（完整版）
				HotelEvents      string `json:",default=zJSnWZm"`     // Hotel Event（活动明细）
				Summary          string `json:",default=QDf9rWh"`     // 会议室类型出租率汇总表（仅用于读"酒店对接人员"字段，公式字段读不到）
			}
			DefaultCity string `json:",default=苏州"`
		}
	}

	Sync struct {
		CronExpr  string `json:",default=0 6 * * *"`
		AutoStart bool   `json:",default=true"`
	}

	UpdateCheck struct {
		CronExpr  string `json:",default=0 20 * * *"` // 每晚 20:00 检测
		AutoStart bool   `json:",default=true"`
	}

	Mail struct {
		Host     string `json:",default=smtp.qiye.aliyun.com"`
		Port     int    `json:",default=465"`
		Username string
		Password string
		FromName string `json:",default=会议室运营平台"`
	}

	FrontendURL string `json:",default=http://localhost:5173"`
}
