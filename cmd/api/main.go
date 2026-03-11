package main

import (
	"log"

	"github.com/MatMassu/checkout-handler/internal/server"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	if err := server.Start(); err != nil {
		log.Fatalf("server: %v", err)
	}
}
