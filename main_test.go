package main

import (
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	_ "github.com/mattn/go-sqlite3"
	"github.com/vmihailenco/msgpack"
)

func TestSqlite(t *testing.T) {
	// os.Remove("./foo.db")
	var sqlStmt string
	db, err := sql.Open("sqlite3", "./AirSENCE-Dummy_202104.db?cache=shared&mode=rwc&_journal_mode=WAL")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	// sqlStmt = `
	// create table if not exists pollutant (id integer not null primary key, ts timestamp,data json,sent bool default false);
	// `
	// _, err = db.Exec(sqlStmt)
	// if err != nil {
	// 	log.Printf("%q: %s\n", err, sqlStmt)
	// 	return
	// }
	// tx, err := db.Begin()
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// sqlStmt = `
	// insert into pollutant(ts,data,sent) values( ?, ?,?)
	// `
	// stmt, err := tx.Prepare(sqlStmt)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// // defer stmt.Close()
	// for index := 0; index < 100; index++ {
	// 	ts := time.Now().UTC().Unix() - 2000 + int64(index)*60
	// 	data := make(map[string]interface{})
	// 	pdata := make(map[string]float64)
	// 	pdata["NO2"] = float64(index)
	// 	pdata["CO"] = float64(index * 2)
	// 	data["GPS"] = map[string]float64{
	// 		"Latitude":  111.1,
	// 		"Longitude": 111.1,
	// 	}
	// 	data["PollutantData"] = pdata
	// 	data["Timestamp"] = ts
	// 	data["DeviceID"] = "AirSENCE-Dummy"
	// 	jdata, err := msgpack.Marshal(data)
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}
	// 	_, err = stmt.Exec(ts, jdata, false)
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}
	// }
	// stmt.Close()
	// tx.Commit()
	sqlStmt = `
	select id, ts, data,sent from pollutant where ts between ? and ?
	`
	st := time.Now().Unix() - 6000
	et := time.Now().Unix()
	rows, err := db.Query(sqlStmt, st, et)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var ts time.Time
		var jdata []byte
		var data map[string]interface{}
		var sent bool
		err = rows.Scan(&id, &ts, &jdata, &sent)
		if err != nil {
			log.Fatal(err)
		}
		err = msgpack.Unmarshal(jdata, &data)
		fmt.Println(id, ts, data, sent)
	}
	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}
}

func TestSending(t *testing.T) {
	optionsLocal := mqtt.NewClientOptions()
	optionsLocal.AddBroker("127.0.0.1:1883")
	optionsLocal.SetClientID("Dummy" + "_DataSync")
	// optionsLocal.SetConnectionLostHandler(handler.lostConnectionHandlerLo)
	// optionsLocal.SetOnConnectHandler(handler.onConnectionHandlerLo)
	clientLocal := mqtt.NewClient(optionsLocal)
	token := clientLocal.Connect()
	for !token.WaitTimeout(5 * time.Second) {
	}
	if token.Error() != nil {
		err := token.Error()
		fmt.Printf("Error when connect to local MQTT broker:%v", err)
	}
	for {
		data := make(map[string]interface{})
		data["DeviceID"] = "AirSENCE-Dummy"
		data["Timestamp"] = time.Now().Unix()
		gps := make(map[string]float64)
		gps["Latitude"] = -79.0
		gps["Longitude"] = 41.0
		data["GPS"] = gps
		pollutantData := make(map[string]float64)
		pollutantData["NO"] = rand.Float64() * 20
		pollutantData["NO2"] = rand.Float64() * 30
		data["PollutantData"] = pollutantData
		payload, _ := msgpack.Marshal(data)
		clientLocal.Publish(
			"airsence/AUG/AirSENCE-Dummy/pollutant",
			0,
			false,
			payload,
		)
		fmt.Println("Data sent")
		fmt.Printf("Data:%v\n", data)
		time.Sleep(15 * time.Second)
	}
}
