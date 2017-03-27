package bucket

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
