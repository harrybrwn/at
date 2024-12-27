package main

import (
	"log"
	"net/http"

	"github.com/harrybrwn/at/xrpc"
)

func main() {
	srv := xrpc.NewServer()
	err := http.ListenAndServe(":8080", srv)
	if err != nil {
		log.Fatal(err)
	}
}
