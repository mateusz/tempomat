package main

import (
	"fmt"
	"log"
	"net/rpc"
	"os"
	"sort"

	"github.com/containous/flaeg"
	"github.com/mateusz/tempomat/api"
	"time"
)

type Configuration struct {
	Bucket string `description:"Name of the bucket to dump"`
}

var conf Configuration

func init() {
	conf = Configuration{Bucket: "Slash32"}
	confPtr := &Configuration{}
	rootCmd := &flaeg.Command{
		Name:                  "Tempomat doctor",
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
}

func main() {
	client, err := rpc.DialHTTP("tcp", "localhost:29999")
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
		fmt.Printf("|%s\t\t\t", b)
	}
	fmt.Print("\n")
	for i:=0; ; i++ {
		has := false
		for _, d := range dumps {
			if i<len(d) {
				has = true
				fmt.Printf("|%.2f\t%.0f\t%s\t", d[i].AvgWait.Seconds(), time.Now().Sub(d[i].LastUsed).Seconds(), d[i].Title)
			}
		}
		fmt.Print("\n")

		if !has {
			break
		}
	}

}
