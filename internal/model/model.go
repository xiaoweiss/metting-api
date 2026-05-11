package model

import "time"

type User struct {
	Id              int64  `gorm:"primaryKey;autoIncrement"`
	DingTalkUnionId string `gorm:"column:dingtalk_union_id;uniqueIndex;size:64;not null"`
	Name            string `gorm:"size:64"`
	Email           string `gorm:"size:128"`
	Phone           string `gorm:"size:32"` // 手机号，用于 SMS 通知
	Status          string `gorm:"type:enum('pending','active','disabled');default:pending"`
	IsAdmin         bool   `gorm:"default:false"`
	AdminPassword   string `gorm:"column:admin_password;size:128"`
	RoleId          *int64 `gorm:"column:role_id"`
	PrimaryHotelId  *int64 `gorm:"column:primary_hotel_id"` // 该用户"所属酒店"（日报渲染默认对标）
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (User) TableName() string { return "users" }

type Role struct {
	Id    int64  `gorm:"primaryKey;autoIncrement"`
	Name  string `gorm:"size:64;uniqueIndex;not null"`
	Label string `gorm:"size:64;not null"`
	Menus string `gorm:"type:json;not null"`
	Apis  string `gorm:"type:json;not null"`
}

func (Role) TableName() string { return "roles" }

type Hotel struct {
	Id               int64  `gorm:"primaryKey;autoIncrement"`
	Name             string `gorm:"size:128;not null"`
	City             string `gorm:"size:64"`
	MarketAreaId     int64
	StarRating       string `gorm:"column:star_rating;size:16"`        // 5-Star
	Brand            string `gorm:"size:64"`                           // 希尔顿酒店及度假村
	HotelGroup       string `gorm:"column:hotel_group;size:64"`        // 希尔顿集团
	Region           string `gorm:"size:64"`                           // 华东地区
	Province         string `gorm:"size:64"`                           // 江苏省
	MetropolitanArea string `gorm:"column:metropolitan_area;size:128"` // 长三角城市群
	CoreCity         string `gorm:"column:core_city;size:64"`          // 上海
	HotelType        string `gorm:"column:hotel_type;size:64"`         // Convention
	BusinessDistrict string `gorm:"column:business_district;size:128"` // 苏州新区/寒山寺
	CreatedAt        time.Time
}

func (Hotel) TableName() string { return "hotels" }

type MarketArea struct {
	Id   int64  `gorm:"primaryKey;autoIncrement"`
	Name string `gorm:"size:64;not null"`
	City string `gorm:"size:64;not null"`
}

func (MarketArea) TableName() string { return "market_areas" }

type Venue struct {
	Id               int64   `gorm:"primaryKey;autoIncrement"`
	HotelId          int64   `gorm:"index"`
	Name             string  `gorm:"size:128;not null"`
	Type             string  `gorm:"size:64"`
	AvailablePeriods string  `gorm:"size:64"`
	AreaSqm          float64 `gorm:"column:area_sqm;type:decimal(10,2)"`            // 面积
	TheaterCapacity  int     `gorm:"column:theater_capacity"`                       // 剧院式
	HasPillar        *bool   `gorm:"column:has_pillar"`                             // 是否有柱
	DingtalkRecordId string  `gorm:"column:dingtalk_record_id;size:32;uniqueIndex"` // 钉钉行 id；同名不同 type 靠这个区分
}

func (Venue) TableName() string { return "venues" }

// HotelEvent 来自 Hotel Event 表，记录具体活动（而非每场次）
type HotelEvent struct {
	Id             int64      `gorm:"primaryKey;autoIncrement"`
	HotelId        int64      `gorm:"index:idx_hotel_date"`
	VenueId        int64      `gorm:"index"`
	EventName      string     `gorm:"column:event_name;size:256"`                     // 活动名称
	EventType      string     `gorm:"column:event_type;size:64"`                       // 活动类型 "Wedding(婚宴)"
	EventDate      time.Time  `gorm:"column:event_date;type:date;index:idx_hotel_date"` // 活动日期
	TargetDate     *time.Time `gorm:"column:target_date;type:date"`                    // 预定日期
	EndDate        *time.Time `gorm:"column:end_date;type:date"`                       // 结束日期
	Period         string     `gorm:"size:16"`                                         // 上午/下午/全天 (多选可能多个，逗号分隔)
	BookingStatus  string     `gorm:"column:booking_status;size:32"`                   // 已出租 / 预订中 ...
	DingtalkRecord string     `gorm:"column:dingtalk_record_id;size:32;uniqueIndex"`   // 去重
	CreatedAt      time.Time
}

func (HotelEvent) TableName() string { return "hotel_events" }

type UserHotelPerm struct {
	UserId  int64 `gorm:"primaryKey"`
	HotelId int64 `gorm:"primaryKey"`
}

func (UserHotelPerm) TableName() string { return "user_hotel_perms" }

type MeetingRecord struct {
	Id                  int64      `gorm:"primaryKey;autoIncrement"`
	HotelId             int64      `gorm:"index:idx_date_hotel"`
	VenueId             int64
	RecordDate          time.Time  `gorm:"type:date;index:idx_date_hotel"`
	Period              string     `gorm:"type:enum('AM','PM','EV');not null"`
	IsBooked            bool       `gorm:"default:false"`
	ActivityType        string     `gorm:"size:64"`
	BanquetFoodRevenue  float64    `gorm:"type:decimal(12,2);default:0"`
	BanquetVenueRevenue float64    `gorm:"type:decimal(12,2);default:0"`
	EntryDate           *time.Time `gorm:"type:date"`
	CreatedAt           time.Time
}

func (MeetingRecord) TableName() string { return "meeting_records" }

type CompetitorGroup struct {
	Id          int64  `gorm:"primaryKey;autoIncrement"`
	Name        string `gorm:"size:128;not null"`
	BaseHotelId int64
}

func (CompetitorGroup) TableName() string { return "competitor_groups" }

type CompetitorGroupHotel struct {
	GroupId int64 `gorm:"primaryKey"`
	HotelId int64 `gorm:"primaryKey"`
}

func (CompetitorGroupHotel) TableName() string { return "competitor_group_hotels" }

type CityEvent struct {
	Id        int64     `gorm:"primaryKey;autoIncrement"`
	City      string    `gorm:"size:64;index:idx_city_date"`
	VenueName string    `gorm:"size:128"`
	EventName string    `gorm:"size:256;not null"`
	EventType string    `gorm:"size:64"`
	EventDate time.Time `gorm:"type:date;index:idx_city_date"`
}

func (CityEvent) TableName() string { return "city_events" }

type ColorThreshold struct {
	Id         int64   `gorm:"primaryKey;autoIncrement"`
	HotelId    *int64  // NULL 表示全局默认
	MetricType string  `gorm:"type:enum('occupancy','activity');not null"`
	Level      string  `gorm:"type:enum('low','medium','high');not null"`
	MinValue   float64 `gorm:"type:decimal(5,2)"`
	MaxValue   float64 `gorm:"type:decimal(5,2)"`
	Color      string  `gorm:"size:16;not null"`
}

func (ColorThreshold) TableName() string { return "color_thresholds" }

type EmailGroup struct {
	Id        int64     `gorm:"primaryKey;autoIncrement"`
	Name      string    `gorm:"size:128;not null"`
	HotelId   int64
	Scene     string    `gorm:"size:64"`
	CreatedAt time.Time
}

func (EmailGroup) TableName() string { return "email_groups" }

type EmailGroupMember struct {
	GroupId int64  `gorm:"primaryKey"`
	UserId  int64  `gorm:"primaryKey"`
	Email   string `gorm:"size:128;not null"`
}

func (EmailGroupMember) TableName() string { return "email_group_members" }

type EmailSchedule struct {
	Id        int64     `gorm:"primaryKey;autoIncrement"`
	GroupId   int64
	CronExpr  string    `gorm:"size:64;not null"`
	Enabled   bool      `gorm:"default:true"`
	CreatedAt time.Time
}

func (EmailSchedule) TableName() string { return "email_schedules" }

type EmailLog struct {
	Id         int64     `gorm:"primaryKey;autoIncrement"`
	ScheduleId int64     `gorm:"index"`
	TemplateId int64     `gorm:"column:template_id;index"`         // 模板 id，重发时用
	Source     string    `gorm:"size:64"`                          // 'blast' / 'group:<id>' / 'manual' / 'legacy'
	Status     string    `gorm:"type:enum('success','partial','failed');not null"`
	Total      int
	FailCount  int
	FailList   string    `gorm:"type:json"` // 兼容字段，仍保留方便查；权威数据看 email_log_recipients
	RetryCount int
	SentAt     time.Time
	CreatedAt  time.Time `gorm:"index"`
}

// EmailLogRecipient 每封邮件一行，包含状态 + 错误原因 + 单独的 retry_count
type EmailLogRecipient struct {
	Id         int64     `gorm:"primaryKey;autoIncrement"`
	LogId      int64     `gorm:"column:log_id;index"`
	Email      string    `gorm:"size:128;not null"`
	Status     string    `gorm:"type:enum('sent','failed');not null"`
	Error      string    `gorm:"type:text"`
	RetryCount int       `gorm:"column:retry_count;default:0"`
	SentAt     time.Time `gorm:"column:sent_at"`
	CreatedAt  time.Time
}

func (EmailLogRecipient) TableName() string { return "email_log_recipients" }

func (EmailLog) TableName() string { return "email_logs" }

type SyncLog struct {
	Id          int64     `gorm:"primaryKey;autoIncrement"`
	Source      string    `gorm:"size:64;not null"`
	Status      string    `gorm:"type:enum('success','failed');not null"`
	RecordCount int
	Message     string    `gorm:"type:text"`
	SyncedAt    time.Time `gorm:"index"`
}

func (SyncLog) TableName() string { return "sync_logs" }

type MailTemplate struct {
	Id          int64     `gorm:"primaryKey;autoIncrement"`
	Name        string    `gorm:"size:128;uniqueIndex;not null"`
	Subject     string    `gorm:"size:256;not null"`
	Body        string    `gorm:"type:text;not null"`
	Description string    `gorm:"size:256"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (MailTemplate) TableName() string { return "mail_templates" }

type MailSetting struct {
	Id        int64     `gorm:"primaryKey"`
	SmtpHost  string    `gorm:"column:smtp_host;size:128;not null"`
	SmtpPort  int       `gorm:"column:smtp_port;not null;default:465"`
	Username  string    `gorm:"size:128;not null"`
	Password  string    `gorm:"size:256;not null"`
	FromName  string    `gorm:"column:from_name;size:64;not null"`
	UpdatedAt time.Time
}

func (MailSetting) TableName() string { return "mail_settings" }

type SyncSchedule struct {
	Id        int64     `gorm:"primaryKey;autoIncrement"`
	CronExpr  string    `gorm:"size:64;not null"`
	Enabled   bool      `gorm:"default:true"`
	UpdatedAt time.Time
}

func (SyncSchedule) TableName() string { return "sync_schedules" }

// UpdateCheckSchedule 数据未录入提醒的 cron（跟 sync_schedules 同款结构）
type UpdateCheckSchedule struct {
	Id        int64  `gorm:"primaryKey;autoIncrement"`
	CronExpr  string `gorm:"size:64;not null"`
	Enabled   bool   `gorm:"default:true"`
	UpdatedAt time.Time
}

func (UpdateCheckSchedule) TableName() string { return "update_check_schedules" }

// MailBlastSchedule 全员群发邮件的调度配置(多行支持,每行一个独立 cron + 模板)
type MailBlastSchedule struct {
	Id         int64      `gorm:"primaryKey;autoIncrement"`
	Name       string     `gorm:"size:64;not null"` // 任务名,如「9am 看板图」
	CronExpr   string     `gorm:"column:cron_expr;size:64;not null"`
	TemplateId int64      `gorm:"column:template_id;not null"`
	Enabled    bool       `gorm:"default:false"`
	LastRunAt  *time.Time `gorm:"column:last_run_at"`
	UpdatedAt  time.Time
}

func (MailBlastSchedule) TableName() string { return "mail_blast_schedules" }

// NotificationSetting 通知渠道配置（sms / dingtalk_robot / dingtalk_ding）
type NotificationSetting struct {
	Id        int64     `gorm:"primaryKey;autoIncrement"`
	Channel   string    `gorm:"size:32;uniqueIndex;not null"` // sms / dingtalk_robot / dingtalk_ding
	Config    string    `gorm:"type:json"`                    // 渠道配置 JSON
	Enabled   bool      `gorm:"default:false"`
	UpdatedAt time.Time
}

func (NotificationSetting) TableName() string { return "notification_settings" }

// HotelUpdateCheck 每日酒店更新检测日志
type HotelUpdateCheck struct {
	Id                int64      `gorm:"primaryKey;autoIncrement"`
	CheckDate         time.Time  `gorm:"column:check_date;type:date;uniqueIndex:uk_date_hotel,priority:1"`
	HotelId           int64      `gorm:"uniqueIndex:uk_date_hotel,priority:2"`
	IsUpdated         bool       `gorm:"column:is_updated"`
	RecordCount       int        `gorm:"column:record_count"`
	NotifiedChannels  string     `gorm:"column:notified_channels;size:128"`
	NotifiedAt        *time.Time `gorm:"column:notified_at"`
	CreatedAt         time.Time
}

func (HotelUpdateCheck) TableName() string { return "hotel_update_checks" }

// DashboardSnapshot PC 端「保存」时上传的看板截图,按 (hotel_id, snapshot_date, mode, format) 唯一,UPSERT 覆盖最新
type DashboardSnapshot struct {
	Id           int64     `gorm:"primaryKey;autoIncrement"`
	HotelId      int64     `gorm:"column:hotel_id;not null;uniqueIndex:uk_hotel_date_mode_format,priority:1"`
	SnapshotDate time.Time `gorm:"column:snapshot_date;type:date;not null;uniqueIndex:uk_hotel_date_mode_format,priority:2"`
	Mode         string    `gorm:"size:16;not null;uniqueIndex:uk_hotel_date_mode_format,priority:3"` // 'occupancy' | 'bookings'
	Format       string    `gorm:"size:8;not null;uniqueIndex:uk_hotel_date_mode_format,priority:4"`  // 'png' | 'pdf'
	FilePath     string    `gorm:"column:file_path;size:255;not null"`
	FileSize     int64     `gorm:"column:file_size"`
	UploadedBy   *int64    `gorm:"column:uploaded_by"`
	UploadedAt   time.Time `gorm:"column:uploaded_at;autoCreateTime"`
}

func (DashboardSnapshot) TableName() string { return "dashboard_snapshots" }

// MailTemplateAttachment 邮件模板的静态附件,群发时全员收到同一份
type MailTemplateAttachment struct {
	Id           int64     `gorm:"primaryKey;autoIncrement"`
	TemplateId   int64     `gorm:"column:template_id;not null;index"`
	OriginalName string    `gorm:"column:original_name;size:255;not null"`
	FilePath     string    `gorm:"column:file_path;size:255;not null"`
	FileSize     int64     `gorm:"column:file_size"`
	MimeType     string    `gorm:"column:mime_type;size:64;not null"`
	Cid          string    `gorm:"size:128;not null"`
	SortOrder    int       `gorm:"column:sort_order;default:0"`
	UploadedAt   time.Time `gorm:"column:uploaded_at;autoCreateTime"`
}

func (MailTemplateAttachment) TableName() string { return "mail_template_attachments" }
