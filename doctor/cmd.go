package main

import (
	"fmt"
	"log"
	"net/rpc"

	"github.com/mateusz/tempomat/api"
)

func main() {
	client, err := rpc.DialHTTP("tcp", "localhost:29999")
	if err != nil {
		log.Fatal("Failed to dial server:", err)
	}

	var reply string
	args := api.EmptyArgs{}
	err = client.Call("BucketDumper.Slash32", &args, &reply)
	if err != nil {
		log.Fatal("Call error:", err)
	}
	fmt.Printf("Reply: %s", reply)
}
