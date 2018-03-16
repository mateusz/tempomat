package main

import (
	"fmt"
	"log"
	"net/rpc"
	"sort"

	"github.com/mateusz/tempomat/api"
	"time"
)

func main() {
	client, err := rpc.DialHTTP("tcp", "127.0.0.1:29999")
	if err != nil {
		log.Fatal("Failed to dial server:", err)
	}

	bucketNames := []string{"Slash32", "Slash24", "Slash16", "UserAgent"}
	dumps := make([]api.DumpList, len(bucketNames))

	for i, b := range bucketNames {
		args := api.DumpArgs{
			BucketName: b,
		}
		err = client.Call("TempomatAPI.Dump", &args, &dumps[i])
		if err != nil {
			log.Fatal("Call error:", err)
		}

		sort.Sort(api.AvgWaitSortDumpList(dumps[i]))
	}

	// Headers
	for _, b := range bucketNames {
		fmt.Printf("|%s\t\t\t\t\t", b)
	}
	fmt.Print("\n")
	for i:=0; ; i++ {
		has := false
		for _, d := range dumps {
			if i<len(d) {
				has = true
				fmt.Printf("|%.2f\t%.2f\t%.2f\t%.0f\t%s\t",
					d[i].AvgWait.Seconds(),
					d[i].AvgCpuSecs,
					1.0/d[i].AvgSincePrev.Seconds(),
					time.Now().Sub(d[i].LastUsed).Seconds(),
					truncateString(d[i].Title, 40),
				)
			} else {
				fmt.Print("|---\t---\t---\t---\t---------\t")
			}
		}
		fmt.Print("\n")

		if !has {
			break
		}
	}

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
