package main

import (
	"fmt"
	"log"
	"net/rpc"

	"github.com/mateusz/tempomat/api"
	"github.com/mateusz/tempomat/bucket"
)

func main() {
	client, err := rpc.DialHTTP("tcp", "localhost:29999")
	if err != nil {
		log.Fatal("Failed to dial server:", err)
	}

	var reply map[string]bucket.EntrySlash32
	args := api.EmptyArgs{}
	err = client.Call("BucketDumper.Slash32", &args, &reply)
	if err != nil {
		log.Fatal("Call error:", err)
	}

	fmt.Printf("%#v\n", len(reply))
	for k, c := range reply {
		fmt.Printf("Slash32,%s,%s,%.3f", k, c.Netmask, c.Credit)
	}
}
