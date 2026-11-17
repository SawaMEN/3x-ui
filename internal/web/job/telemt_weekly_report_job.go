package job

import "github.com/SawaMEN/3x-ui/v3/internal/web/service/tgbot"

// TelemtWeeklyReportJob snapshots cumulative MTProto client traffic once a
// week, without requiring Xray or Sing-box to be running.
type TelemtWeeklyReportJob struct {
	bot tgbot.Tgbot
}

func NewTelemtWeeklyReportJob() *TelemtWeeklyReportJob {
	return &TelemtWeeklyReportJob{}
}

func (j *TelemtWeeklyReportJob) Run() {
	j.bot.SendWeeklyTelemtReport()
}
