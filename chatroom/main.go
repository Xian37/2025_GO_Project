package main

import (
	"chatroom/models"
	"chatroom/service"
	"chatroom/transport"
	"fmt"
	"log"
	"net/http"
)

func main() {
	broadcastChan := make(chan models.Message)

	stateService := service.NewStateService(broadcastChan)

	go stateService.HandleMessageLoop()

	wsHandler := transport.NewWebsocketHandler(stateService)

	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)
	http.HandleFunc("/ws", wsHandler.HandleConnections)

	fmt.Println("Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
