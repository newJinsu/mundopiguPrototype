package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	mainLobby := newLobby()
	go mainLobby.Run()

	fs := http.FileServer(http.Dir("./public"))
	http.Handle("/", fs)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(mainLobby, w, r)
	})

	fmt.Println("Server started on :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
