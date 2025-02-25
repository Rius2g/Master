package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	
	// Import your backend package
	contract "github.com/rius2g/Master/backend/pkg/ContractInteractionInterface"
    msgProcessor "github.com/rius2g/Master/backend/pkg/messageProcessor"
	t "github.com/rius2g/Master/backend/pkg/types"
)


//this is the demo to show the package with the message processor abstraction
func main(){
    if err := godotenv.Load(); err != nil {
        log.Fatal("No .env file found")
    }

    fmt.Println("==== Smart Contract Message Processor Demo ====")
    fmt.Println("Initializing Smart Contract interface...") 

    contractAddress := os.Getenv("CONTRACT_ADDRESS") 
    if contractAddress == "" {
        log.Fatal("CONTRACT_ADDRESS not found in .env")
    }

    privateKey := os.Getenv("PRIVATE_KEY") 
    if privateKey == "" {
        log.Fatal("PRIVATE_KEY not found in .env")
    }

    c, err := contract.Init(contractAddress, privateKey, 1) //security level 1
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Smart Contract interface initialized") 

    mp, err := msgProcessor.NewMessageProcessor(c)
    if err != nil { 
        log.Fatal(err)
    }

    fmt.Println("Message Processor initialized")



}
