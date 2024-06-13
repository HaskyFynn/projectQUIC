package my_logging

import (
	"fmt"
	"log"
	"os"
	"path"
)

var iLog *log.Logger // Global variable to store the logger object

func init() {
	LOGFILE := path.Join(os.TempDir(), "server.log")
	fmt.Println(LOGFILE)
	f, err := os.OpenFile(LOGFILE, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer f.Close()
	iLog = log.New(f, "iLog: ", log.LstdFlags) // Create the logger object
	iLog.Println("Server started!")
}
