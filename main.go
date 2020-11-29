package main

import (
	static "github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"gopkg.in/olahol/melody.v1"
	"encoding/json"
	"context"
	"fmt"
	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/linkedin/goavro"
	"log"
	"time"
)

type PowerEvent struct {
    Timestamp int64 `json:"timestamp"`
    Watts float32 `json:"watts"`
}
func (p PowerEvent) String() string {
	return fmt.Sprintf("%d, %f", p.Timestamp, p.Watts)
}

func readingCodec() (*goavro.Codec, error) {
	codec, err := goavro.NewCodec(`
    {
      "name": "Reading",
      "type": "record",
      "fields": [
        {
          "name": "value",
          "type": [
            "null",
            "float"
          ]
        }
      ]
    }`)
	return codec, err
}

// Start listening to pulsar, and send new events to the provided listener
func pulsarListen(m *melody.Melody) {
	codec, err := readingCodec()

	client, err := pulsar.NewClient(pulsar.ClientOptions{
		URL:               "pulsar://ektar.wellorder.net:6650",
		OperationTimeout:  30 * time.Second,
		ConnectionTimeout: 30 * time.Second,
	})
	if err != nil {
		log.Fatalf("Could not instantiate Pulsar client: %v", err)
	}
	reader, err := client.CreateReader(pulsar.ReaderOptions{
		Topic:            "homeiot/prod/ektar-zw095-power",
		StartMessageID: pulsar.EarliestMessageID(),
	})
	if err != nil {
		log.Fatal(err)
	}
	defer reader.Close()
	for  {
		msg, err := reader.Next(context.Background())
		if err != nil {
			log.Fatal(err)
		}
		native, _, err := codec.NativeFromBinary(msg.Payload())
		if err != nil {
			fmt.Println(err)
		}
		record := native.(map[string]interface{})["value"]
		floatval := record.(map[string]interface{})["float"].(float32)
		publishTime := msg.PublishTime()
		publishEpoch := publishTime.Unix()
		var p = PowerEvent {
			Timestamp: publishEpoch,
			Watts: floatval}
		pj, err := json.Marshal(p)
		if err != nil {
			log.Fatal(err)
		}
		m.Broadcast(pj)
		fmt.Println(string(pj))
	}
	defer client.Close()
}


func main() {
	r := gin.Default()
	var m *melody.Melody = melody.New()

	// Serve static content
	r.Use(static.Serve("/", static.LocalFile("./public", true)))


	// Websockets listener for power updates
	r.GET("/power-ws", func(c *gin.Context) {
		m.HandleRequest(c.Writer, c.Request)
	})

	go pulsarListen(m)
	
	r.Run(":5000")
}
