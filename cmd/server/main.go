package main

import (
	"log"
	"net/http"

	"kvstore/internal/api"
	"kvstore/internal/store"
)

const DB_FILE = "KvStore.db"

func main() {
	kvstore := store.NewStore(DB_FILE)
	err := kvstore.Open()
	if err != nil {
		log.Fatal(err)
	}

	handler := api.NewHandler(kvstore)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	log.Println("Listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}