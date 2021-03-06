package gitnotify

import (
	"fmt"
	"log"
	"reflect"
	"sync"
	"time"

	"gopkg.in/robfig/cron.v2"
	// "github.com/robfig/cron"
	// cron "github.com/sairam/cron1"
)

// 1. load all files to get cron schedules, next_scheduled_date
// 2. if next_scheduled_date is in past, schedule a job now - NOTE: this is going to open a flood gate in case of backlog
// 3. find the next run date, save to file
// 4. Once the job runs, get the next date and save to file
// 5. When a user saves the schedule, delete the old schedule reference, add the cron and save the next date to file

var (
	cronLocker   sync.Mutex
	crons        *cron.Cron
	runningCrons = make(map[string]cron.EntryID)
)

func isCronPresentFor(filename string) bool {
	id := runningCrons[filename]
	entry := crons.Entry(id)
	return entry.Valid()
}

func checkCronEntries(filename string) (nextRunTimes []string) {
	nextRunTimes = make([]string, 0, 15)
	id := runningCrons[filename]
	entry := crons.Entry(id)
	if !entry.Valid() {
		return
	}

	conf := new(Setting)
	conf.load(filename)
	tz := conf.User.TimeZoneName
	loc, _ := time.LoadLocation(tz)

	t := entry.Next
	nextRunTimes = append(nextRunTimes, t.In(loc).String())
	for i := 0; i < cap(nextRunTimes)-1; i++ {
		t = entry.Schedule.Next(t)
		nextRunTimes = append(nextRunTimes, t.In(loc).String())
	}
	return
}

func hasUserNotificationSet(s *Setting) bool {
	return isValidEmail(s.usersEmail()) || s.User.isValidWebhook()
}

func upsertCronEntry(s *Setting) {
	cronLocker.Lock()
	defer cronLocker.Unlock()

	tzName := s.User.TimeZoneName
	hour := s.User.Hour
	// weekday is day of week
	weekday := s.User.WeekDay

	a := s.Auth
	filename := a.getConfigFile()

	if weekday == "" || hour == "" || tzName == "" || !hasUserNotificationSet(s) {
		log.Printf("Not starting cron for `%s` since attributes are not set\n", s.Auth.UserName)
		stopCronIfAlreadyRunning(filename)
		return
	}

	if s.User.Disabled == true {
		log.Printf("User `%s` does not want any emails/notifications\n", s.Auth.UserName)
		stopCronIfAlreadyRunning(filename)
		return
	}

	log.Printf("(re)starting cron for %s/%s\n", s.Auth.Provider, s.Auth.UserName)
	cronEntry := fmt.Sprintf("TZ=%s 0 0 %s * * %s", tzName, hour, weekday)

	toStart := true

	id := runningCrons[filename]
	if id != 0 {
		// check if entry was not modified
		var entry cron.Entry
		entry = crons.Entry(id)
		if entry.Valid() {
			s, _ := cron.Parse(cronEntry)
			s2 := s.(cron.Schedule)

			scheduleChanged := !compareSchedules(entry.Schedule, s2)

			if scheduleChanged {
				crons.Remove(id)
				runningCrons[filename] = 0
				toStart = true
			} else {
				toStart = false
			}
		}
	}

	if toStart {
		startCronFor(cronEntry, filename)
	}
}

func stopCronIfAlreadyRunning(filename string) {
	id := runningCrons[filename]
	entry := crons.Entry(id)
	if entry.Valid() {
		crons.Remove(id)
		runningCrons[filename] = 0
	}
}

type cronJob struct {
	filename string
	save     bool
}

func (t cronJob) Run() {
	filename := t.filename
	conf := new(Setting)
	log.Printf("Processing file through cron - %s", filename)
	conf.load(filename)
	processDiffForUser(conf)
	if t.save {
		conf.save(filename)
	}
}

func startCronFor(cronEntry, filename string) {
	id, _ := crons.AddJob(cronEntry, cronJob{filename, true})
	runningCrons[filename] = id
}

// InitCron initialises the cron related methods
func InitCron() {
	crons = cron.New()
	crons.Start()

	if config.Providers[GithubProvider] != "" {
		go getData(GithubProvider)
	}
	if config.Providers[GitlabProvider] != "" {
		go getData(GitlabProvider)
	}
}

// There is no idiomatic way to compare SpecSchedule, put in a sort of adjustment
func compareSchedules(schedule1, schedule2 cron.Schedule) bool {
	v1 := reflect.ValueOf(schedule1).Elem()
	v2 := reflect.ValueOf(schedule2).Elem()
	s1, s2 := fmt.Sprintf("%v", v1), fmt.Sprintf("%v", v2)
	return (s1 == s2)
}
