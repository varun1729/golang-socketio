package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/geneva-lake/golang-socketio"
	"github.com/geneva-lake/golang-socketio/transport"
	"github.com/mtfelian/utils"
)

type MessageInner struct {
	Id      int    `json:"id"`
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

var currentRoot string
var assetsDir http.FileSystem

func main() {
	currentRoot, _ := os.Getwd()
	d := filepath.Join(currentRoot, "assets")
	if !utils.FileExists(d) {
		d = filepath.Join(currentRoot, "examples", "assets")
	}
	assetsDir = http.Dir(d)

	fmt.Println("assetsDir:", assetsDir)

	server := gosocketio.NewServer(transport.GetDefaultPollingTransport())

	server.On(gosocketio.OnConnection, func(c *gosocketio.Channel) {
		log.Println("Connected")

		//c.Emit("/message", MessageInner{10, "main", "using emit"})

		//c.Join("test")
		//c.BroadcastTo("test", "/message", MessageInner{10, "main", "using broadcast"})
	})
	server.On(gosocketio.OnDisconnection, func(c *gosocketio.Channel) {
		log.Println("Disconnected")
	})

	server.On("send", func(c *gosocketio.Channel, param interface{}) string {
		log.Println("came send ")
		j, err := json.Marshal(param)
		if err != nil {
			log.Println("marhsalling err:", err)
		}
		fmt.Println("json:", string(j))
		return "send received"
	})

	server.On("another", func(c *gosocketio.Channel, param string) string {
		log.Println("came another", param)
		return "another param:" + param
	})

	serveMux := http.NewServeMux()
	serveMux.Handle("/socket.io/", server)

	serveMux.HandleFunc("/", AssetsFileHandler)

	log.Println("Starting server...")
	log.Panic(http.ListenAndServe(":3811", serveMux))
}

func AssetsFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return
	}
	var file string = r.URL.Path
	f, err := assetsDir.Open(file)
	if err != nil {
		log.Println("can not open file ", file, " ", err)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		log.Fatalln("can not open file ", file, " ", err)
	}
	http.ServeContent(w, r, file, fi.ModTime(), f)
}
