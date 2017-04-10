package api

import "github.com/mateusz/tempomat/bucket"

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
	Credit float64
}

type DumpList []DumpEntry
type CreditSortDumpList []DumpEntry

func (l CreditSortDumpList) Len() int           { return len(l) }
func (l CreditSortDumpList) Less(i, j int) bool { return l[i].Credit < l[j].Credit }
func (l CreditSortDumpList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }

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
			Credit: e[i].Credit(),
		}
	}
	return l
}
