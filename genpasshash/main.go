package main

import (
	"flag"
	"fmt"
	"log"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	cost := 0
	flag.IntVar(&cost, "c", 0, "cost")
	flag.Parse()
	pass := flag.Arg(0)

	hash, err := bcrypt.GenerateFromPassword([]byte(pass), cost)

	if err != nil {
		log.Fatal("failed to generate hash: ", err)
		return
	}

	fmt.Println(string(hash))
}
