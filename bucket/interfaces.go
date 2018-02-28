package bucket

import (
	"fmt"
	"net/http"

	"github.com/mateusz/tempomat/lib/config"
	"time"
)

type Entries []Entry

type Entry interface {
	fmt.Stringer
	Hash() string
	LastUsed() time.Time
	AvgWait() time.Duration
	AvgSincePrev() time.Duration
	Title() string
}

type Bucketable interface {
	fmt.Stringer
	Entries() Entries
	ReserveN(r *http.Request, start time.Time, qty float64) (delay time.Duration, ok bool)
	SetConfig(config.Config)
	DelayThreshold() time.Duration
}

type LastUsedSortEntries []Entry

func (l LastUsedSortEntries) Len() int           { return len(l) }
func (l LastUsedSortEntries) Less(i, j int) bool { return l[i].LastUsed().After(l[j].LastUsed()) }
func (l LastUsedSortEntries) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }

type AvgWaitSortEntries []Entry

func (l AvgWaitSortEntries) Len() int           { return len(l) }
func (l AvgWaitSortEntries) Less(i, j int) bool { return l[i].AvgWait()>l[j].AvgWait() }
func (l AvgWaitSortEntries) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
