package main

import (
	"fmt"
	"log"
	"net/rpc"
	"os"
	"sort"

	"time"

	"github.com/containous/flaeg"
	"github.com/mateusz/tempomat/api"
	"github.com/olekukonko/tablewriter"
)

type configuration struct {
	Bucket string `description:"Name of the bucket to dump"`
}

var conf configuration

func main() {
	conf = configuration{Bucket: "Slash32"}
	confPtr := &configuration{}
	rootCmd := &flaeg.Command{
		Name:                  "tempomat-doctor",
		Description:           "Connects to tempomat server and dumps current hash values",
		Config:                &conf,
		DefaultPointersConfig: confPtr,
		Run: func() error {
			// We are just setting globals.
			return nil
		},
	}
	flaeg := flaeg.New(rootCmd, os.Args[1:])

	if err := flaeg.Run(); err != nil {
		log.Fatal("Error reading flags: %s", err)
	}

	client, err := rpc.DialHTTP("tcp", "127.0.0.1:29999")
	if err != nil {
		log.Fatal("Failed to dial server:", err)
	}

	dump := make(api.DumpList, 0)
	args := api.DumpArgs{
		BucketName: conf.Bucket,
	}
	err = client.Call("TempomatAPI.Dump", &args, &dump)
	if err != nil {
		log.Fatal("Call error:", err)
	}

	sort.Sort(sortDumpList(dump))

	fmt.Printf("Bucket: %s\n", conf.Bucket)

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Wait[s]", "Cpu[s]", "Req/s", "Last[s]", "Title"})
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(false)

	for _, d := range dump {
		rps := 0.0
		if d.AvgSincePrev.Seconds() > 0 {
			rps = 1.0 / d.AvgSincePrev.Seconds()
		}

		table.Append([]string{
			fmt.Sprintf("%.2f", d.AvgWait.Seconds()),
			fmt.Sprintf("%.2f", d.AvgCpuSecs),
			fmt.Sprintf("%.2f", rps),
			fmt.Sprintf("%.0f", time.Now().Sub(d.LastUsed).Seconds()),
			d.Title,
		})
	}

	table.Render()
}

func truncateString(str string, num int) string {
	out := str
	if len(str) > num {
		if num > 3 {
			num -= 3
		}
		out = str[0:num] + "~"
	}
	return out
}

type sortDumpList []api.DumpEntry

func (l sortDumpList) Len() int { return len(l) }
func (l sortDumpList) Less(i, j int) bool {
	if l[i].AvgCpuSecs != l[j].AvgCpuSecs {
		return l[i].AvgCpuSecs > l[j].AvgCpuSecs
	}
	if l[i].LastUsed != l[j].LastUsed {
		return l[i].LastUsed.After(l[j].LastUsed)
	}
	return true

}
func (l sortDumpList) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
