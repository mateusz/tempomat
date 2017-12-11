package api

import (
	"github.com/mateusz/tempomat/bucket"
	"time"
)

type TempomatAPI struct {
	buckets []bucket.Bucketable
}

func NewTempomatAPI(b []bucket.Bucketable) *TempomatAPI {
	return &TempomatAPI{
		buckets: b,
	}
}

type DumpArgs struct {
	BucketName string
}

type DumpEntry struct {
	Hash   string
	Title  string
	LastUsed time.Time
	AvgWait time.Duration
}

type DumpList []DumpEntry
type AvgWaitSortDumpList []DumpEntry

func (l AvgWaitSortDumpList) Len() int           { return len(l) }
func (l AvgWaitSortDumpList) Less(i, j int) bool { return l[i].AvgWait>l[j].AvgWait }
func (l AvgWaitSortDumpList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }

func (a *TempomatAPI) Dump(args *DumpArgs, reply *DumpList) error {
	for _, b := range a.buckets {
		if b.String() == args.BucketName {
			*reply = repack(b)
		}
	}
	return nil
}

func repack(b bucket.Bucketable) DumpList {
	e := b.Entries()
	l := make(DumpList, len(e))
	for i := 0; i < len(e); i++ {
		l[i] = DumpEntry{
			Hash:   e[i].Hash(),
			Title:  e[i].Title(),
			LastUsed: e[i].LastUsed(),
			AvgWait: e[i].AvgWait(),
		}
	}
	return l
}
