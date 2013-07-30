package main

import (
	"encoding/json"
	"fmt"
	"github.com/siesta/neo4j"
	"github.com/streadway/amqp"
	"io/ioutil"
	"koding/databases/mongo"
	oldNeo "koding/databases/neo4j"
	"koding/tools/amqputil"
	"koding/tools/config"
	"labix.org/v2/mgo/bson"
	"log"
	"net/http"
	"strconv"
	"strings"
)

var GRAPH_URL = config.Current.Neo4j.Write + ":" + strconv.Itoa(config.Current.Neo4j.Port)

type Relationship struct {
	Id         bson.ObjectId `bson:"_id,omitempty"`
	TargetId   bson.ObjectId `bson:"targetId,omitempty"`
	TargetName string        `bson:"targetName"`
	SourceId   bson.ObjectId `bson:"sourceId,omitempty"`
	SourceName string        `bson:"sourceName"`
	As         string
	Data       bson.Binary
}

func main() {
	amqpChannel := connectToRabbitMQ()

	coll := mongo.GetCollection("relationships")
	query := bson.M{
		"targetName": bson.M{"$nin": oldNeo.NotAllowedNames},
		"sourceName": bson.M{"$nin": oldNeo.NotAllowedNames},
	}
	iter := coll.Find(query).Skip(0).Limit(1000).Sort("-timestamp").Iter()

	var result Relationship
	for iter.Next(&result) {
		if processRelationship(result) {
			createRelationship(result, amqpChannel)
		}
	}
}

func connectToRabbitMQ() *amqp.Channel {
	conn := amqputil.CreateConnection("syncWorker")
	amqpChannel, err := conn.Channel()
	if err != nil {
		panic(err)
	}
	return amqpChannel
}

func createRelationship(rel Relationship, amqpChannel *amqp.Channel) {
	data := make([]bson.M, 1)
	data[0] = bson.M{
		"_id":        rel.Id,
		"sourceId":   rel.SourceId,
		"sourceName": rel.SourceName,
		"targetId":   rel.TargetId,
		"targetName": rel.TargetName,
		"as":         rel.As,
	}

	eventData := bson.M{"event": "RelationshipSaved", "payload": data}

	neoMessage, err := json.Marshal(eventData)
	if err != nil {
		fmt.Println("unmarshall error")
		return
	}

	amqpChannel.Publish(
		"graphFeederExchange", // exchange name
		"",    // key
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			Body: neoMessage,
		},
	)
}

func processRelationship(result Relationship) bool {
	exists := true
	targetId, sourceId := "", ""

	exists, sourceId = checkNodeExists(result.SourceId.Hex())
	if exists != true {
		log.Println("SourceNode doesnt exists")
		return true
	}

	exists, targetId = checkNodeExists(result.TargetId.Hex())
	if exists != true {
		log.Println("TargetNode doesnt exists")
		return true
	}

	exists = checkRelationshipExists(sourceId, targetId, result.As)
	if exists != true {
		log.Println("Relationship ERROR")
		return true
	}

	// everything is fine
	return false
}

func getAndParse(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

func checkRelationshipExists(sourceId, targetId, relType string) bool {
	url := fmt.Sprintf("%v/db/data/node/%v/relationships/all/%v", GRAPH_URL, sourceId, relType)

	body, err := getAndParse(url)
	if err != nil {
		return false
	}

	relResponse := make([]neo4j.RelationshipResponse, 1)
	err = json.Unmarshal(body, &relResponse)
	if err != nil {
		return false
	}

	for _, rl := range relResponse {
		id := strings.SplitAfter(rl.End, GRAPH_URL+"/db/data/node/")[1]
		if targetId == id {
			return true
		}
	}

	return false
}

func checkNodeExists(id string) (bool, string) {
	url := fmt.Sprintf("%v/db/data/index/node/koding/id/%v", GRAPH_URL, id)
	body, err := getAndParse(url)
	if err != nil {
		return false, ""
	}

	nodeResponse := make([]neo4j.NodeResponse, 1)
	err = json.Unmarshal(body, &nodeResponse)
	if err != nil {
		return false, ""
	}

	if len(nodeResponse) < 1 {
		return false, ""
	}

	node := nodeResponse[0]
	idd := strings.SplitAfter(node.Self, GRAPH_URL+"/db/data/node/")

	nodeId := string(idd[1])
	if nodeId == "" {
		return false, ""
	}

	return true, nodeId
}
