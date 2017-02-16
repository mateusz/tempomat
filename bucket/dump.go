package bucket

type DumpEntry struct {
	Title  string
	Credit float64
}

type DumpList []DumpEntry
type CreditSortList []DumpEntry

func (l CreditSortList) Len() int           { return len(l) }
func (l CreditSortList) Less(i, j int) bool { return l[i].Credit < l[j].Credit }
func (l CreditSortList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
