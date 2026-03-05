package main

import (
	"fmt"
	"os"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/jwc20/stopwatch-tui/stopwatch"
)

func exportICS(splits []stopwatch.SplitEntry) (string, error) {
	if len(splits) == 0 {
		return "", fmt.Errorf("no splits to export")
	}

	appStart := splits[0].RecordedAt.Add(-splits[0].Elapsed)

	cal := ics.NewCalendar()
	cal.SetMethod(ics.MethodPublish)

	for i, split := range splits {
		var start time.Time
		if i == 0 {
			start = appStart
		} else {
			start = splits[i-1].RecordedAt
		}
		end := split.RecordedAt

		event := cal.AddEvent(fmt.Sprintf("split-%d-%d@stopwatch-tui", i+1, split.RecordedAt.UnixNano()))
		event.SetCreatedTime(time.Now())
		event.SetDtStampTime(time.Now())
		event.SetStartAt(start)
		event.SetEndAt(end)
		event.SetSummary(fmt.Sprintf("Split %d", i+1))
	}

	filename := fmt.Sprintf("splits_%s.ics", time.Now().Format("2006-01-02_15-04-05"))
	f, err := os.Create(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if err := cal.SerializeTo(f); err != nil {
		return "", err
	}
	return filename, nil
}
