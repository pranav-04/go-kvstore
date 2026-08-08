package main

import (
	"log"
	"net/http"

	"kvstore/internal/api"
	"kvstore/internal/store"
)

const DB_FILE = "/opt/kvstore/kvstore.db"

func main() {
	kvstore := store.NewStore(DB_FILE)
	err := kvstore.Open()
	if err != nil {
		log.Fatal(err)
	}

	handler := api.NewHandler(kvstore)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	log.Println("Listening on 127.0.0.1:8080")
	log.Fatal(http.ListenAndServe("127.0.0.1:8080", mux))
}