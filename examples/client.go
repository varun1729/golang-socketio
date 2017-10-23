package main

import (
	"log"
	"runtime"
	"time"

	"github.com/geneva-lake/golang-socketio"
	"github.com/geneva-lake/golang-socketio/transport"
)

type Channel struct {
	Channel string `json:"channel"`
}

type Message struct {
	Id      int    `json:"id"`
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

func sendJoin(c *gosocketio.Client) {
	log.Println("Acking /join")
	result, err := c.Ack("/join", Channel{"main"}, time.Second*5)
	if err != nil {
		log.Fatal(err)
	} else {
		log.Println("Ack result to /join: ", result)
	}
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	c, err := gosocketio.Dial(
		gosocketio.GetUrlPolling("localhost", 3811, false),
		transport.GetDefaultPollingClientTransport())
	if err != nil {
		log.Fatal(err)
	}

	err = c.On("/message", func(h *gosocketio.Channel, args Message) {
		log.Println("--- Got chat message: ", args)
	})
	if err != nil {
		log.Fatal(err)
	}



	err = c.On(gosocketio.OnDisconnection, func(h *gosocketio.Channel) {
		//log.Fatal("Disconnected")
		log.Println("Disconnected")
	})
	if err != nil {
		log.Fatal(err)
	}

	err = c.On(gosocketio.OnConnection, func(h *gosocketio.Channel) {
		log.Println("Connected")
	})
	if err != nil {
		log.Fatal(err)
	}

	time.Sleep(20 * time.Second)
	c.Close()
	//c.Emit("41", "")
	log.Println("client closed")
	time.Sleep(30 * time.Second)

	log.Println(" [x] Complete")
}
