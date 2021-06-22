package handler

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"aws.airsence/datasync/config"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack"
)

var (
	DATE = "2021-04-01T00:00:00+00:00" //This date is for checking whether the machine is sychronized or not
)

type Handler struct {
	config               config.Config
	MainLogger           *logrus.Logger
	done                 chan bool
	RemoteMqttClient     mqtt.Client
	LocalMqttClient      mqtt.Client
	DB                   *sql.DB
	remoteMqttConnected  bool
	RawTopic             string
	PollutantTopic       string
	ResendRawTopic       string
	ResendPollutantTopic string
}

type RawDataMsgPack struct {
	DeviceID  string
	Timestamp int64
	RawData   map[string]float64
}

type PollutantDataMsgPack struct {
	DeviceID      string
	Timestamp     int64
	GPS           map[string]float64
	PollutantData map[string]float64
}

type ResendRequest struct {
	StartDate int64
	EndDate   int64
}

func InitHandler(
	config config.Config,
	done chan bool,
	mainLogger *logrus.Logger,
) (handler *Handler) {
	willPayload := strings.Replace(config.Mqtt.WillPayload, "+", config.Mqtt.ClientID, 1)
	willTopic := strings.Replace(config.Mqtt.WillTopic, "+", config.Mqtt.ClientID, 1)
	rawTopic := strings.Replace(config.Mqtt.RawTopic, "+", config.Mqtt.ClientID, 1)
	pollutantTopic := strings.Replace(config.Mqtt.PollutantTopic, "+", config.Mqtt.ClientID, 1)
	resendRawTopic := strings.Replace(config.Mqtt.ResendRawTopic, "+", config.Mqtt.ClientID, 1)
	resendPollutantTopic := strings.Replace(config.Mqtt.ResendPollutantTopic, "+", config.Mqtt.ClientID, 1)
	handler = &Handler{
		config:               config,
		done:                 done,
		remoteMqttConnected:  false,
		MainLogger:           mainLogger,
		RawTopic:             rawTopic,
		PollutantTopic:       pollutantTopic,
		ResendRawTopic:       resendRawTopic,
		ResendPollutantTopic: resendPollutantTopic,
	}
	//Initilize Local MQTT Client
	optionsLocal := mqtt.NewClientOptions()
	optionsLocal.AddBroker("127.0.0.1:1883")
	optionsLocal.SetClientID(config.Mqtt.ClientID + "_DataSync")
	optionsLocal.SetConnectionLostHandler(handler.lostConnectionHandlerLo)
	optionsLocal.SetOnConnectHandler(handler.onConnectionHandlerLo)
	clientLocal := mqtt.NewClient(optionsLocal)
	handler.LocalMqttClient = clientLocal
	token := clientLocal.Connect()
	for !token.WaitTimeout(5 * time.Second) {
	}
	if token.Error() != nil {
		err := token.Error()
		mainLogger.Errorf("Error when connect to local MQTT broker:%v", err)
	}

	//Initilize Remote MQTT Client
	cer, err := tls.LoadX509KeyPair(config.Mqtt.CertFile, config.Mqtt.KeyFile)
	if err != nil {
		mainLogger.Errorf("Error when try to get MQTT credential file:%v", err)
	}
	optionsRemote := mqtt.NewClientOptions()
	optionsRemote.AddBroker(config.Mqtt.Servers)
	optionsRemote.SetClientID(config.Mqtt.ClientID + "_DataSync")
	optionsRemote.SetWill(willTopic, willPayload, config.Mqtt.Qos, false)
	optionsRemote.SetKeepAlive(60 * time.Second)
	optionsRemote.SetWriteTimeout(5 * time.Second)
	optionsRemote.SetPingTimeout(3 * time.Second)
	optionsRemote.SetConnectionLostHandler(handler.lostConnectionHandler)
	optionsRemote.SetOnConnectHandler(handler.ontConnectionHandler)
	optionsRemote.SetAutoReconnect(false)
	optionsRemote.SetTLSConfig(&tls.Config{Certificates: []tls.Certificate{cer}})
	clientRemote := mqtt.NewClient(optionsRemote)
	handler.RemoteMqttClient = clientRemote
	token = clientRemote.Connect()
	for !token.WaitTimeout(5 * time.Second) {
	}
	if token.Error() != nil {
		err := token.Error()
		mainLogger.Errorf("Error when client connect to remote MQTT broker:%v", err)
	}
	go handler.InitDB()
	return
}

//InitDB initialize DB connection
func (handler *Handler) InitDB() {
	var date, mileStoneTime time.Time
	var db *sql.DB
	var err error
	mileStoneTime, _ = time.Parse(
		time.RFC3339,
		DATE,
	)
	for {
		//Checking whether the time is synchonized or not
		date = time.Now().UTC()
		if date.Before(mileStoneTime) {
			time.Sleep(15 * time.Second)
			continue
		}
		break
	}
	//Get the path to create or open the database file
	dbPath := fmt.Sprintf(
		"%v/%v_%v.db?cache=shared&mode=rwc&_journal_mode=WAL",
		handler.config.Server.MainFolder,
		handler.config.Mqtt.ClientID,
		date.Format("200601"),
	)
	//Try to open connection to the database
	for {
		db, err = sql.Open("sqlite3", dbPath)
		if err != nil {
			handler.MainLogger.Errorf("Unable to open database for storing data:%v", err)
			handler.isReadOnlyError(err)
			time.Sleep(time.Second * 30)
			continue
		}
		break
	}
	handler.DB = db
	sqlStmt := `
	create table if not exists pollutant (id integer not null primary key, ts timestamp,data json,sent bool default false);
	`
	_, err = db.Exec(sqlStmt)
	if err != nil {
		handler.MainLogger.Errorf("Unable to create pollutant table:%v", err)
	}
	sqlStmt = `
	create table if not exists raw (id integer not null primary key, ts timestamp,data json,sent bool default false);
	`
	_, err = db.Exec(sqlStmt)
	if err != nil {
		handler.MainLogger.Errorf("Unable to create raw table:%v", err)
	}
}

//startTx start a transaction with DB
func (handler *Handler) startTx() (*sql.Tx, error) {
	if handler.DB == nil {
		go handler.InitDB()
		return nil, fmt.Errorf("Database not connected")
	}
	tx, err := handler.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("Unable to setup connection to database")
	}
	return tx, nil
}

//fixFileSystem will try to fix the file system for SD card when read-only file system problem
//happens on the SD card
func (handler *Handler) fixFileSystem() {
	if _, err := exec.Command("fsck.fat", "-a", "/dev/mmcblk0p1").Output(); err != nil {
		log.Printf("File System repair Fail:%v\n", err)
	}
}

//isReadOnlyError will detect whether the error is a read-only file system error or not
func (handler *Handler) isReadOnlyError(err error) {
	if errString := fmt.Sprintf("%v", err); strings.Contains(errString, "read-only file system") {
		log.Println("We got read-only file system error, will try to fix the file system for Onion")
		handler.fixFileSystem()
	}
}

//onConnectionHandlerLo will subscribe to pollutant/raw topic with local MQTT broker
//It will also subscribe to resend pollutant/raw topic with local MQTT broker (For local UI to send resend request)
func (handler *Handler) onConnectionHandlerLo(c mqtt.Client) {
	handler.MainLogger.Info("MQTT client get connection with local MQTT broker")
	var token mqtt.Token
	token = handler.LocalMqttClient.Subscribe(handler.PollutantTopic, 0, handler.pollutantHandler)
	handler.subTokenHandler(token, handler.PollutantTopic)

	token = handler.LocalMqttClient.Subscribe(handler.RawTopic, 0, handler.rawHandler)
	handler.subTokenHandler(token, handler.RawTopic)

	token = handler.LocalMqttClient.Subscribe(handler.ResendPollutantTopic, 0, handler.resendPollutantHandler)
	handler.subTokenHandler(token, handler.ResendPollutantTopic)

	token = handler.LocalMqttClient.Subscribe(handler.ResendRawTopic, 0, handler.resendRawHandler)
	handler.subTokenHandler(token, handler.ResendRawTopic)
}

//ontConnectionHandler will subscribe to resend pollutant/raw topic with remote MQTT broker
//It will also set the remoteMqttConnected flag to true
func (handler *Handler) ontConnectionHandler(c mqtt.Client) {
	handler.MainLogger.Info("MQTT client get connection with remote MQTT broker")
	handler.remoteMqttConnected = true
	var token mqtt.Token
	token = handler.RemoteMqttClient.Subscribe(handler.ResendRawTopic, 0, handler.resendRawHandler)
	handler.subTokenHandler(token, handler.ResendRawTopic)

	token = handler.RemoteMqttClient.Subscribe(handler.ResendPollutantTopic, 0, handler.resendPollutantHandler)
	handler.subTokenHandler(token, handler.ResendPollutantTopic)
}

func (handler *Handler) lostConnectionHandlerLo(c mqtt.Client, err error) {
	handler.MainLogger.Errorf("MQTT client lost connection with local MQTT broker:%v", err)
}

//lostConnectionHandler will set remoteMqttConnected flag to false
func (handler *Handler) lostConnectionHandler(c mqtt.Client, err error) {
	handler.MainLogger.Errorf("MQTT client lost connection with remote MQTT broker:%v", err)
	handler.remoteMqttConnected = false
	token := handler.RemoteMqttClient.Connect()
	for !token.WaitTimeout(5 * time.Second) {
	}
	if token.Error() != nil {
		err := token.Error()
		handler.MainLogger.Errorf("Error when client connect to remote MQTT broker:%v", err)
	}
}

//rawHandler is the handler for local MQTT client when it receive raw data from local MQTT broker.
//It will try to send data to remote MQTT broker and save data to local database
func (handler *Handler) rawHandler(client mqtt.Client, msg mqtt.Message) {
	var sendSuccessful bool = false
	if handler.config.Server.SendRawData {
		if err := handler.sendRaw(msg.Payload()); err != nil {
			handler.MainLogger.Errorf("Error when send raw data to remote server:%v", err)
		} else {
			sendSuccessful = true
		}
	}
	if handler.config.Server.LogRaw {
		if err := handler.saveRawDatabase(msg.Payload(), sendSuccessful); err != nil {
			handler.MainLogger.Errorf("Error when save raw data to database:%v", err)
		}
	}
}

//pollutantHandler is the handler for local MQTT client when it receive pollutant data from local MQTT broker.
//It will try to send data to remote MQTT broker and save data to local database
func (handler *Handler) pollutantHandler(client mqtt.Client, msg mqtt.Message) {
	var sendSuccessful bool = false
	if handler.config.Server.SendPollutantData {
		if err := handler.sendPollutant(msg.Payload()); err != nil {
			handler.MainLogger.Errorf("Error when send pollutant data to remote server:%v", err)
		} else {
			sendSuccessful = true
		}
	}
	if handler.config.Server.LogPollutant {
		if err := handler.savePollutantDatabase(msg.Payload(), sendSuccessful); err != nil {
			handler.MainLogger.Errorf("Error when save pollutant data to database:%v", err)
		}
	}
}

/*
TODO: Currently, resend handler cannot handler the situation when the month change.
For example, if you are now in April 2021, you cannot request the this software to resend data before April 1st, 2021,
even if the database file is there. The resend logic should be update to handler this kind of request.
*/

/*resendRawHandler is the handler for resend request for raw data.
The request format should be (in json)

{
	"StartDate":(Unix time in int64),
	"EndDate":(Unix time in int64)
}
*/
func (handler *Handler) resendRawHandler(client mqtt.Client, msg mqtt.Message) {
	handler.MainLogger.Info("Get resend raw data request")
	resTopic := fmt.Sprintf("%v/response", msg.Topic())
	var resStr string
	var err error
	var request ResendRequest
	if err = json.Unmarshal(msg.Payload(), &request); err != nil {
		resStr = fmt.Sprintf("Unable to parse resend request:%v", err)
		handler.MainLogger.Errorf(resStr)
		handler.mqttResponse(resTopic, resStr, false)
		return
	}
	if err = handler.resendRaw(request.StartDate, request.EndDate); err != nil {
		resStr = fmt.Sprintf("Fail to resend:%v", err)
		handler.MainLogger.Errorf(resStr)
		handler.mqttResponse(resTopic, resStr, false)
	} else {
		startdate := time.Unix(request.StartDate, 0)
		enddate := time.Unix(request.EndDate, 0)
		resStr = fmt.Sprintf(
			"Successfully resend raw data between %v and %v",
			startdate.Format(time.RFC3339),
			enddate.Format(time.RFC3339),
		)
		handler.mqttResponse(resTopic, resStr, true)
	}
}

/*resendPollutantHandler is the handler for resend request for pollutant data.
The request format should be (in json)

{
	"StartDate":(Unix time in int64),
	"EndDate":(Unix time in int64)
}
*/
func (handler *Handler) resendPollutantHandler(client mqtt.Client, msg mqtt.Message) {
	handler.MainLogger.Info("Get resend pollutant data request")
	resTopic := fmt.Sprintf("%v/response", msg.Topic())
	var resStr string
	var err error
	var request ResendRequest
	if err = json.Unmarshal(msg.Payload(), &request); err != nil {
		resStr := fmt.Sprintf("Unable to parse resend request:%v", err)
		handler.MainLogger.Errorf(resStr)
		handler.mqttResponse(resTopic, resStr, false)
		return
	}
	if err = handler.resendPollutant(request.StartDate, request.EndDate); err != nil {
		resStr = fmt.Sprintf("Fail to resend:%v", err)
		handler.MainLogger.Errorf(resStr)
		handler.mqttResponse(resTopic, resStr, false)
	} else {
		startdate := time.Unix(request.StartDate, 0)
		enddate := time.Unix(request.EndDate, 0)
		resStr = fmt.Sprintf(
			"Successfully resend pollutant data between %v and %v",
			startdate.Format(time.RFC3339),
			enddate.Format(time.RFC3339),
		)
		handler.mqttResponse(resTopic, resStr, true)
	}
}

func (handler *Handler) resendRaw(startdate int64, enddate int64) (err error) {
	handler.MainLogger.Info("Resending raw data triggered.")
	if !handler.remoteMqttConnected {
		return fmt.Errorf("Remote MQTT client not connected")
	}
	if handler.DB == nil {
		go handler.InitDB()
		return fmt.Errorf("Database not connected")
	}
	selectStmt := `
	select id,ts,data from raw where sent = false and ts between ? and ?
	`
	updateStmt := `
	update raw set sent = true where id = ?
	`
	tx, err := handler.startTx()
	if err != nil {
		return fmt.Errorf("Error when start transaction for resend raw:%v", err)
	}
	stmt, err := tx.Prepare(updateStmt)
	if err != nil {
		return fmt.Errorf("Error when start transaction for resend raw:%v", err)
	}
	defer stmt.Close()
	rows, err := handler.DB.Query(selectStmt, startdate, enddate)
	if err != nil {
		return fmt.Errorf("Error when query data for resend raw:%v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var ts time.Time
		var jsonBinary []byte
		err = rows.Scan(&id, &ts, &jsonBinary)
		if err != nil {
			return fmt.Errorf("Unable to fetch raw data from database:%v", err)
		}
		err = handler.sendRaw(jsonBinary)
		if err != nil {
			return fmt.Errorf("Error when update database for resend raw:%v", err)
		} else {
			_, err = stmt.Exec(id)
			if err != nil {
				return fmt.Errorf("Error when update database for resend raw:%v", err)
			}
		}
	}
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("Error when commit database for resend raw:%v", err)
	}
	return nil
}

func (handler *Handler) resendPollutant(startdate int64, enddate int64) (err error) {
	handler.MainLogger.Info("Resending pollutant data triggered.")
	if !handler.remoteMqttConnected {
		return fmt.Errorf("Remote MQTT client not connected")
	}
	if handler.DB == nil {
		go handler.InitDB()
		return fmt.Errorf("Database not connected")
	}
	selectStmt := `
	select id,ts,data from pollutant where sent = false and ts between ? and ?
	`
	updateStmt := `
	update pollutant set sent = true where id = ?
	`
	tx, err := handler.startTx()
	if err != nil {
		return fmt.Errorf("Error when start transaction for resend pollutant:%v", err)
	}
	stmt, err := tx.Prepare(updateStmt)
	if err != nil {
		return fmt.Errorf("Error when start transaction for resend pollutant:%v", err)
	}
	defer stmt.Close()
	rows, err := handler.DB.Query(selectStmt, startdate, enddate)
	if err != nil {
		return fmt.Errorf("Error when query data for resend pollutant:%v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var ts time.Time
		var jsonBinary []byte
		err = rows.Scan(&id, &ts, &jsonBinary)
		if err != nil {
			return fmt.Errorf("Unable to fetch pollutant data from database:%v", err)
		}
		err = handler.sendPollutant(jsonBinary)
		if err != nil {
			return fmt.Errorf("Error when update database for resend pollutant:%v", err)
		} else {
			_, err = stmt.Exec(id)
			if err != nil {
				return fmt.Errorf("Error when update database for resend pollutant:%v", err)
			}
		}
	}
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("Error when commit database for resend pollutant:%v", err)
	}
	return nil
}

func (handler *Handler) saveRawDatabase(data []byte, sendSuccessful bool) error {
	var err error
	tx, err := handler.startTx()
	if err != nil {
		return err
	}
	sqlStmt := `
	insert into raw(ts,data,sent) values (?,?,?)
	`
	stmt, err := tx.Prepare(sqlStmt)
	if err != nil {
		return err
	}
	defer stmt.Close()
	var rawDataMsgPack RawDataMsgPack
	err = msgpack.Unmarshal(data, &rawDataMsgPack)
	if err != nil {
		return err
	}
	_, err = stmt.Exec(rawDataMsgPack.Timestamp, data, sendSuccessful)
	if err != nil {
		return err
	}
	err = tx.Commit()
	if err != nil {
		return err
	}
	return nil
}

func (handler *Handler) savePollutantDatabase(data []byte, sendSuccessful bool) error {
	var err error
	tx, err := handler.startTx()
	if err != nil {
		return err
	}
	sqlStmt := `
	insert into pollutant(ts,data,sent) values (?,?,?)
	`
	stmt, err := tx.Prepare(sqlStmt)
	if err != nil {
		return err
	}
	defer stmt.Close()
	var pollutantDataMsgPack PollutantDataMsgPack
	err = msgpack.Unmarshal(data, &pollutantDataMsgPack)
	if err != nil {
		return err
	}
	_, err = stmt.Exec(pollutantDataMsgPack.Timestamp, data, sendSuccessful)
	if err != nil {
		return err
	}
	err = tx.Commit()
	if err != nil {
		return err
	}
	return nil
}

//sendRaw sending raw data to remote MQTT broker
func (handler *Handler) sendRaw(data []byte) error {
	token := handler.RemoteMqttClient.Publish(handler.RawTopic, handler.config.Mqtt.Qos, false, data)
	return handler.pubTokenHandler(token)
}

//sendPollutant sending pollutant to remote MQTT broker
func (handler *Handler) sendPollutant(data []byte) error {
	token := handler.RemoteMqttClient.Publish(handler.PollutantTopic, handler.config.Mqtt.Qos, false, data)
	return handler.pubTokenHandler(token)
}

//mqttResponse send respond for resend request
func (handler *Handler) mqttResponse(topic string, msg string, success bool) {
	message := make(map[string]interface{})
	message["suceess"] = success
	message["message"] = msg
	payload, _ := json.Marshal(message)
	token := handler.RemoteMqttClient.Publish(
		topic,
		handler.config.Mqtt.Qos,
		false,
		payload,
	)
	handler.pubTokenHandler(token)
}

//subTokenHandler is the handler for MQTT subscribe token
func (handler *Handler) subTokenHandler(token mqtt.Token, topic string) {
	for !token.WaitTimeout(5 * time.Second) {
	}
	if err := token.Error(); err != nil {
		handler.MainLogger.Errorf("Error when subscribe to %v:%v", topic, err)
	} else {
		handler.MainLogger.Infof("Subscribe to %v", topic)
	}
}

//pubTokenHandler is the handler for MQTT publish token
func (handler *Handler) pubTokenHandler(token mqtt.Token) error {
	for !token.WaitTimeout(5 * time.Second) {
	}
	return token.Error()
}

//Run is main function for Handler to run. It will try to resend with the resending interval
func (handler *Handler) Run() {
	resendTicker := time.NewTicker(time.Minute * time.Duration(handler.config.Mqtt.ResendingInterval))
	for {
		select {
		case <-handler.done:
			err := handler.DB.Close()
			if err != nil {
				handler.MainLogger.Errorf("Unable to close connection with database:%v", err)
			}
			return
		case <-resendTicker.C:
			if handler.remoteMqttConnected {
				if handler.config.Server.SendPollutantData {
					endTime := time.Now().Unix()
					handler.resendPollutant(0, endTime)
				}
				if handler.config.Server.SendRawData {
					endTime := time.Now().Unix()
					handler.resendRaw(0, endTime)
				}
			} else {
				handler.MainLogger.Error("Lost internet connection. Cannot perform any resending.")
			}
		}
	}
}
