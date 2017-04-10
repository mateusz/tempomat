package bucket

import (
	"fmt"
	"net/http"

	"github.com/mateusz/tempomat/lib/config"
)

type Entries []Entry

type Entry interface {
	fmt.Stringer
	Hash() string
	Credit() float64
	Title() string
}

type Bucketable interface {
	fmt.Stringer
	Entries() Entries
	Register(r *http.Request, cost float64)
	Threshold() float64
	SetConfig(config.Config)
}

type CreditSortEntries []Entry

func (l CreditSortEntries) Len() int           { return len(l) }
func (l CreditSortEntries) Less(i, j int) bool { return l[i].Credit() < l[j].Credit() }
func (l CreditSortEntries) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
