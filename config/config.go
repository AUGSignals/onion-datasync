package config

import (
	"io/ioutil"

	"github.com/BurntSushi/toml"
	"github.com/yeka/zip"
)

type MainConfig struct {
	DataSync Config
}

//Config is the config for initialize the service
type Config struct {
	Server ServerConfig
	Log    LogConfig
	Mqtt   MqttConfig
}

//UserConfig is the config for user
type UserConfig struct {
	Server UserServerConfig
	Mqtt   UserMqttConfig
}

//UserServerConfig is the config for user to control basic raw data and pollutant data sending
type UserServerConfig struct {
	LogRaw            bool //LogRaw decide whether log raw data to local or not
	LogPollutant      bool //LogPollutant decide whether log pollutant data to local or not
	SendRawData       bool //SendRawData decide whether send raw data to server or not
	SendPollutantData bool //SendPollutantData device whether send pollutant data to server or not
	WaitTime          int  //WaitTime before the main process start running
}

//UserMqttConfig is the config for user to control MQTT related configuration
type UserMqttConfig struct {
	ClientID             string //ClientID for this device, which is also the DeviceID
	Qos                  byte   //Qos for communication
	WillTopic            string
	WillPayload          string
	RawTopic             string //Topic for sending raw data
	PollutantTopic       string //Topic for sending pollutant data
	ResendPollutantTopic string
	ResendRawTopic       string
	ResendingInterval    int //Sending Interval in second
}

//ServerConfig is the config for cloud server
type ServerConfig struct {
	GoDebug           bool   //GoDebug turn on debug mode for software so it print out more debug information
	LogRaw            bool   //LogRaw decide whether log raw data to local or not
	LogPollutant      bool   //LogPollutant decide whether log pollutant data to local or not
	SendRawData       bool   //SendRawData decide whether send raw data to server or not
	SendPollutantData bool   //SendPollutantData device whether send pollutant data to server or not
	MainFolder        string //MainFolder is where database file is located
	WaitTime          int
}

type LogConfig struct {
	Filename   string
	MaxSize    int  //File maximum size in megabytes
	MaxBackups int  //File maximum backup number
	MaxAge     int  //File maximum ages in days
	Compress   bool //Whether compress the backup or not (disabled by default)
}

//MqttConfig is the config for mqtt communication
type MqttConfig struct {
	Servers              string //MQTT server address with port number
	ClientID             string //ClientID for this device, which is also the DeviceID
	Qos                  byte   //Qos for communication
	WillTopic            string
	WillPayload          string
	RawTopic             string //Topic for sending raw data
	PollutantTopic       string //Topic for sending pollutant data
	ResendRawTopic       string //Topic for resending raw data
	ResendPollutantTopic string //Topic for resending pollutant data
	ResendingInterval    int    //Sending Interval in second
	KeyFile              string
	CertFile             string
}

func ReadConf(configPath string) (Config, error) {
	var conf MainConfig
	r, err := zip.OpenReader(configPath)
	if err != nil {
		return conf.DataSync, err
	}
	defer r.Close()
	for _, f := range r.File {
		if f.IsEncrypted() {
			f.SetPassword("AUGSignals-2020")
		}
		data, err := f.Open()
		if err != nil {
			return conf.DataSync, err
		}
		defer data.Close()
		buf, err := ioutil.ReadAll(data)
		if err != nil {
			return conf.DataSync, err
		}
		err = toml.Unmarshal(buf, &conf)
		if err != nil {
			return conf.DataSync, err
		}
	}
	return conf.DataSync, nil
}

//ReadUserConf is the function for unzip and parse internal config file
func ReadUserConf(configPath string) (UserConfig, error) {
	var conf UserConfig
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return conf, err
	}
	err = toml.Unmarshal(data, &conf)
	if err != nil {
		return conf, err
	}
	return conf, nil
}

func (config *Config) MergeUserConfig(userconfig UserConfig) {
	//Merge user config (Server part)
	config.Server.LogPollutant = userconfig.Server.LogPollutant
	config.Server.LogRaw = userconfig.Server.LogRaw
	config.Server.SendPollutantData = userconfig.Server.SendPollutantData
	config.Server.SendRawData = userconfig.Server.SendRawData

	//Merge user config (Mqtt part)
	config.Mqtt.ClientID = userconfig.Mqtt.ClientID
	config.Mqtt.Qos = userconfig.Mqtt.Qos
	config.Mqtt.PollutantTopic = userconfig.Mqtt.PollutantTopic
	config.Mqtt.RawTopic = userconfig.Mqtt.RawTopic
	config.Mqtt.ResendPollutantTopic = userconfig.Mqtt.ResendPollutantTopic
	config.Mqtt.ResendRawTopic = userconfig.Mqtt.ResendRawTopic
	config.Mqtt.WillTopic = userconfig.Mqtt.WillTopic
	config.Mqtt.WillPayload = userconfig.Mqtt.WillPayload
	config.Mqtt.ResendingInterval = userconfig.Mqtt.ResendingInterval
}
