package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"

	"aws.airsence/datasync/config"
	"aws.airsence/datasync/handler"
)

var (
	//Version is the version number for this software
	Version = "v1.0.0"
	//BuildDate is the Build Date for this software
	BuildDate = ""
	//CONFIGPATH is the default path of the config file
	CONFIGPATH = "config.tomlz"
	//USERCONFIGPATH is the default path of user config file
	USERCONFIGPATH = "config_user.toml"
	//CONFIG is the config for the software
	CONFIG config.Config
	//USERCONFIG is the user controlled config for the software
	USERCONFIG config.UserConfig
)

func printHelpInfo() {
	fmt.Println("")
	fmt.Println("#######################################################################")
	fmt.Println(`#    ___     _            _____    ______    _   __   ______    ______#`)
	fmt.Println(`#   /   |   (_)   _____  / ___/   / ____/   / | / /  / ____/   / ____/#`)
	fmt.Println(`#  / /| |  / /   / ___/  \__ \   / __/     /  |/ /  / /       / __/   #`)
	fmt.Println(`# / ___ | / /   / /     ___/ /  / /___    / /|  /  / /___    / /___   #`)
	fmt.Println(`#/_/  |_|/_/   /_/     /____/  /_____/   /_/ |_/   \____/   /_____/   #`)
	fmt.Println(`#              BREATHE SAFE            BREATHE EASY                   #`)
	fmt.Println("#######################################################################")
	fmt.Println("")
	fmt.Printf("This is AirSENCE data synchronization service. Current Version:%v,Build Date:%v\n", Version, BuildDate)
	fmt.Println("Command Line Option:")
	fmt.Println("-c --config		User config file")
	fmt.Println("-d --default		Default config file")
	fmt.Println("-h --help		Print this help information")
	fmt.Println("-v --version		Print firmware version")
	os.Exit(0)
}

func argParse() {
	//Read config file path from argument
	argsWithouProg := os.Args[1:]
	if len(argsWithouProg) > 0 {
		for i := 0; i < len(argsWithouProg); i += 2 {
			key := argsWithouProg[i]
			switch key {
			case "-d", "--default":
				value := argsWithouProg[i+1]
				CONFIGPATH = value
				fmt.Printf("Find config file:%v\n", CONFIGPATH)
			case "-c", "--config":
				value := argsWithouProg[i+1]
				USERCONFIGPATH = value
				fmt.Printf("Find user config file:%v\n", USERCONFIGPATH)
			case "-h", "--help":
				printHelpInfo()
			case "-v", "--version":
				fmt.Printf("Current Version:%v,Build Date:%v\n", Version, BuildDate)
				os.Exit(0)
			default:
				fmt.Println("Not acceptable key.")
				printHelpInfo()
			}
		}
	}

}

func loadConfig() {
	//Parse input arguments
	argParse()
	//Read config file
	var err error
	CONFIG, err = config.ReadConf(CONFIGPATH)
	if err != nil {
		log.Fatalf("ERROR when reading %v:%v", CONFIGPATH, err)
		fmt.Printf("ERROR when reading %v:%v", CONFIGPATH, err)
	}
	//Read user config file
	USERCONFIG, err = config.ReadUserConf(USERCONFIGPATH)
	if err != nil {
		log.Printf("ERROR when reading %v:%v", USERCONFIGPATH, err)
		fmt.Printf("ERROR when reading %v:%v", USERCONFIGPATH, err)
	} else {
		CONFIG.MergeUserConfig(USERCONFIG)
	}
}

// gracefullShutdown is the function that help the servre to shutdown without cutting down the
// running request
func gracefullShutdown(quit <-chan os.Signal, stopSignal chan<- bool, mainLogger *logrus.Logger) {
	<-quit
	mainLogger.Infoln("Service is shutting down...")
	close(stopSignal)
	return
}

func main() {
	var err error
	/** Load config file **/
	loadConfig()

	if _, err = os.Stat(CONFIG.Server.MainFolder); err != nil {
		panic(fmt.Sprintf("Main folder not found.The given folder is %v", CONFIG.Server.MainFolder))
	}

	/** Initialize log **/
	// LogInit(ioutil.Discard, os.Stdout, os.Stdout, os.Stderr, logFile)
	rollingLog := &lumberjack.Logger{
		Filename:   CONFIG.Log.Filename,
		MaxSize:    CONFIG.Log.MaxSize, // megabytes
		MaxBackups: CONFIG.Log.MaxBackups,
		MaxAge:     CONFIG.Log.MaxAge,   //days
		Compress:   CONFIG.Log.Compress, // disabled by default
	}
	defer rollingLog.Close()

	// RollingLogInit(ioutil.Discard, os.Stdout, os.Stdout, os.Stderr, rollingLog)
	mainLogger := logrus.New()
	mainLogger.SetFormatter(&logrus.JSONFormatter{})
	mainLogger.SetOutput(rollingLog)
	mainLogger.Infoln("Service initialization start...")

	/** Gracefull shutdow setup **/
	// Initialize two channel for gracefully shutdown
	stopSignal := make(chan bool, 1)
	quit := make(chan os.Signal)
	// Notify quit if os send a close signal
	signal.Notify(quit, os.Interrupt, os.Kill, syscall.SIGKILL, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT)
	//Setup gracefull shutdown routine
	go gracefullShutdown(quit, stopSignal, mainLogger)
	myHandler := handler.InitHandler(CONFIG, stopSignal, mainLogger)
	myHandler.Run()
	mainLogger.Infoln("Service shut down.")
	if CONFIG.Server.GoDebug {
		fmt.Println("All resource released.")
	}
	os.Exit(0)
}
