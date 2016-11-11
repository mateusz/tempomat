package main

import (
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
)

type StatsFormatter struct {
}

func (f *StatsFormatter) Format(entry *log.Entry) ([]byte, error) {
	return []byte(fmt.Sprintf("%s,%s\n", entry.Time.Format(time.RFC3339), entry.Message)), nil
}
