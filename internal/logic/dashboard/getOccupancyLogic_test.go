package dashboard

import (
	"sort"
	"testing"
)

// buildOccupancyList 四种数据组合用例
// 验证日期并集 + HasHotelRecord 正确性 + Hotel 字段值
func TestBuildOccupancyList(t *testing.T) {
	t.Run("case1_三源齐全_本酒店当天有数据", func(t *testing.T) {
		hotel := []periodRow{
			{RecordDate: "2026-05-01", Period: "AM", Booked: 3, Total: 10},
		}
		comp := []periodRow{
			{RecordDate: "2026-05-01", Period: "AM", Booked: 5, Total: 20},
		}
		mkt := []periodRow{
			{RecordDate: "2026-05-01", Period: "AM", Booked: 8, Total: 40},
		}

		got := buildOccupancyList(hotel, comp, mkt, nil)
		if len(got) != 1 {
			t.Fatalf("期望 1 天, 实际 %d 天", len(got))
		}
		if got[0].Date != "2026-05-01" {
			t.Errorf("日期错: %s", got[0].Date)
		}
		if !got[0].HasHotelRecord {
			t.Errorf("HasHotelRecord 应为 true")
		}
		if got[0].Hotel.M != 30 {
			t.Errorf("Hotel.M 期望 30, 实际 %v", got[0].Hotel.M)
		}
	})

	t.Run("case2_本酒店漏录_竞对商圈有数据", func(t *testing.T) {
		hotel := []periodRow{} // 本酒店无录入
		comp := []periodRow{
			{RecordDate: "2026-05-02", Period: "AM", Booked: 5, Total: 20},
		}
		mkt := []periodRow{
			{RecordDate: "2026-05-02", Period: "PM", Booked: 10, Total: 40},
		}

		got := buildOccupancyList(hotel, comp, mkt, nil)
		if len(got) != 1 {
			t.Fatalf("期望 1 天 (来自竞对/商圈), 实际 %d 天", len(got))
		}
		if got[0].Date != "2026-05-02" {
			t.Errorf("日期错: %s", got[0].Date)
		}
		if got[0].HasHotelRecord {
			t.Errorf("HasHotelRecord 应为 false")
		}
		if got[0].Hotel.M != 0 || got[0].Hotel.A != 0 || got[0].Hotel.E != 0 {
			t.Errorf("本酒店无数据时 Hotel 各时段应为 0, 实际 M=%v A=%v E=%v",
				got[0].Hotel.M, got[0].Hotel.A, got[0].Hotel.E)
		}
		if got[0].CompetitorAvg.M != 25 {
			t.Errorf("CompetitorAvg.M 期望 25, 实际 %v", got[0].CompetitorAvg.M)
		}
	})

	t.Run("case3_本酒店有商圈有_竞对无", func(t *testing.T) {
		hotel := []periodRow{
			{RecordDate: "2026-05-03", Period: "PM", Booked: 4, Total: 10},
		}
		comp := []periodRow{}
		mkt := []periodRow{
			{RecordDate: "2026-05-03", Period: "PM", Booked: 12, Total: 40},
		}

		got := buildOccupancyList(hotel, comp, mkt, nil)
		if len(got) != 1 {
			t.Fatalf("期望 1 天, 实际 %d 天", len(got))
		}
		if !got[0].HasHotelRecord {
			t.Errorf("HasHotelRecord 应为 true")
		}
		if got[0].Hotel.A != 40 {
			t.Errorf("Hotel.A 期望 40, 实际 %v", got[0].Hotel.A)
		}
		if got[0].CompetitorAvg.M != 0 || got[0].CompetitorAvg.A != 0 {
			t.Errorf("竞对无数据时 CompetitorAvg 应为 0, 实际 M=%v A=%v",
				got[0].CompetitorAvg.M, got[0].CompetitorAvg.A)
		}
	})

	t.Run("case4_三源都无_该天不出现", func(t *testing.T) {
		got := buildOccupancyList(nil, nil, nil, nil)
		if len(got) != 0 {
			t.Errorf("三源全空时不应有任何日期, 实际 %d 天", len(got))
		}
	})

	t.Run("case5_多日混合_并集排序后字段正确", func(t *testing.T) {
		hotel := []periodRow{
			{RecordDate: "2026-05-01", Period: "AM", Booked: 1, Total: 10}, // 1日 本酒店有
		}
		comp := []periodRow{
			{RecordDate: "2026-05-02", Period: "AM", Booked: 5, Total: 20}, // 2日 竞对有
		}
		mkt := []periodRow{
			{RecordDate: "2026-05-03", Period: "PM", Booked: 8, Total: 40}, // 3日 商圈有
		}
		events := []struct {
			EventDate string
			Count     int
		}{
			{EventDate: "2026-05-02", Count: 2},
		}

		got := buildOccupancyList(hotel, comp, mkt, events)
		if len(got) != 3 {
			t.Fatalf("期望 3 天并集, 实际 %d 天", len(got))
		}

		// 排序方便断言
		sort.Slice(got, func(i, j int) bool { return got[i].Date < got[j].Date })

		// 1 日: 只有本酒店
		if got[0].Date != "2026-05-01" || !got[0].HasHotelRecord {
			t.Errorf("1日: %+v", got[0])
		}
		// 2 日: 本酒店无 (竞对有)
		if got[1].Date != "2026-05-02" || got[1].HasHotelRecord {
			t.Errorf("2日 HasHotelRecord 应为 false: %+v", got[1])
		}
		if got[1].CityEventCount != 2 {
			t.Errorf("2日 CityEventCount 期望 2, 实际 %d", got[1].CityEventCount)
		}
		// 3 日: 本酒店无 (商圈有)
		if got[2].Date != "2026-05-03" || got[2].HasHotelRecord {
			t.Errorf("3日 HasHotelRecord 应为 false: %+v", got[2])
		}
	})
}
