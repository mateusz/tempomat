package main

import (
	"fmt"
	"log"
	"net/rpc"
	"os"
	"sort"

	"github.com/containous/flaeg"
	"github.com/mateusz/tempomat/api"
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

	var reply api.DumpList
	args := api.DumpArgs{
		BucketName: conf.Bucket,
	}
	err = client.Call("TempomatAPI.Dump", &args, &reply)
	if err != nil {
		log.Fatal("Call error:", err)
	}

	sort.Sort(api.CreditSortDumpList(reply))

	fmt.Printf(conf.Bucket + "\n")
	fmt.Printf("=============\n")
	for _, v := range reply {
		fmt.Printf("%.3f\t%s\n", v.Credit, v.Title)
	}
}
