package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jwc20/stopwatch-tui/stopwatch"
)

func exportGWSCommands(splits []stopwatch.SplitEntry, names []string) (string, error) {
	if len(splits) == 0 {
		return "", fmt.Errorf("no splits to export")
	}

	appStart := splits[0].RecordedAt.Add(-splits[0].Elapsed)

	var sb strings.Builder
	for i, split := range splits {
		var start time.Time
		if i == 0 {
			start = appStart
		} else {
			start = splits[i-1].RecordedAt
		}
		end := split.RecordedAt

		name := fmt.Sprintf("Split %d", i+1)
		if i < len(names) && names[i] != "" {
			name = names[i]
		}

		lap := end.Sub(start)
		description := fmt.Sprintf(
			"Split %d of %d | Elapsed: %s | Lap: %s",
			i+1, len(splits),
			formatSplitDuration(split.Elapsed),
			formatSplitDuration(lap),
		)

		sb.WriteString(fmt.Sprintf(
			"gws calendar +insert --summary %q --start %q --end %q --description %q\n",
			name,
			start.Format(time.RFC3339),
			end.Format(time.RFC3339),
			description,
		))
	}

	filename := fmt.Sprintf("splits_%s.sh", time.Now().Format("2006-01-02_15-04-05"))
	if err := os.WriteFile(filename, []byte(sb.String()), 0755); err != nil {
		return "", err
	}
	return filename, nil
}
