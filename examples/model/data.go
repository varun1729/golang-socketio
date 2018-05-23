package model

// Data is an example model
type Data struct {
	EventName         string      `json:"eventName"`
	BroadcastRoomName string      `json:"bcRoomName"`
	Payload           interface{} `json:"payload"`
}
